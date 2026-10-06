package snapshotting

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
	"github.com/vhive-serverless/vhive/storage"
	"golang.org/x/sys/unix"
)

func (mgr *SnapshotManager) RequiresCompressedWorkingSet(snap *Snapshot) bool {
	return mgr.compression.WorkingSet && !mgr.wsRecording && snap.GetId() != "base"
}

// Uncommitted stream files do not contribute to resident cache accounting.
func hasCommittedStreamCompanion(dir, name string) bool {
	var companion string
	switch name {
	case "working_set_pages_content.zstd.streams", "working_set_pages_content_private.zstd.streams":
		companion = name + ".json"
	case "working_set_pages_content.zstd.streams.json", "working_set_pages_content_private.zstd.streams.json":
		companion = name[:len(name)-len(".json")]
	default:
		return true
	}
	_, err := os.Stat(filepath.Join(dir, companion))
	return err == nil
}

func (mgr *SnapshotManager) streamEpoch(revision string) *atomic.Uint64 {
	value, _ := mgr.wsStreamEpoch.LoadOrStore(revision, &atomic.Uint64{})
	return value.(*atomic.Uint64)
}

func (mgr *SnapshotManager) streamPublishLock(revision string) *sync.Mutex {
	value, _ := mgr.wsStreamPublish.LoadOrStore(revision, &sync.Mutex{})
	return value.(*sync.Mutex)
}

// writeTemp closes a complete file before it can acquire its visible name.
// No fsync is imposed on the existing asynchronous compressed cache.
func writeStreamTemp(path string, write func(io.Writer) error) (name string, err error) {
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".streams-pending-*")
	if err != nil {
		return "", err
	}
	name = f.Name()
	defer func() {
		if err != nil {
			os.Remove(name)
		}
	}()
	err = write(f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	return name, err
}

func (mgr *SnapshotManager) persistWorkingSetStreams(revision, rawPath string, raw []byte) error {
	started := time.Now()
	payloadPath := workingSetZstdPayloadPath(rawPath)
	manifestPath := workingSetZstdManifestPath(rawPath)
	var manifest *zstdstreams.Manifest
	tmp, err := writeStreamTemp(payloadPath, func(dst io.Writer) error {
		var err error
		manifest, err = zstdstreams.EncodeTo(dst, raw, mgr.compression.Level)
		return err
	})
	if err != nil {
		return fmt.Errorf("encode WS streams: %w", err)
	}
	defer os.Remove(tmp)
	data, err := zstdstreams.MarshalManifest(manifest)
	if err != nil {
		return err
	}
	metaTmp, err := writeStreamTemp(manifestPath, func(dst io.Writer) error { _, err := dst.Write(data); return err })
	if err != nil {
		return err
	}
	defer os.Remove(metaTmp)
	if err := os.Rename(tmp, payloadPath); err != nil {
		return err
	}
	if err := os.Rename(metaTmp, manifestPath); err != nil {
		return err
	}
	if err := mgr.uploadFile(revision, payloadPath); err != nil {
		return err
	}
	// Publishing the immutable remote manifest last commits the payload.
	if err := mgr.uploadFile(revision, manifestPath); err != nil {
		return err
	}
	if err := os.Remove(rawPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	log.Infof("ZSTD_WS_ENCODE revision=%s layout=%s raw_bytes=%d compressed_bytes=%d streams=%d level=%d elapsed_us=%d",
		revision, zstdstreams.Layout, manifest.RawSize, manifest.CompressedSize, manifest.StreamCount, manifest.Level, time.Since(started).Microseconds())
	return nil
}

// openCachedStreams pins an fd, not the directory entry: eviction may unlink
// it after we drop the lock, while SectionReaders safely finish through ReadAt.
func (mgr *SnapshotManager) openCachedStreams(revision, rawPath string) (*os.File, []byte, uint64, error) {
	mgr.wsRegistry.deletionLock.RLock()
	defer mgr.wsRegistry.deletionLock.RUnlock()
	lock := mgr.streamPublishLock(revision)
	lock.Lock()
	defer lock.Unlock()
	token := mgr.streamEpoch(revision).Load()
	data, err := os.ReadFile(workingSetZstdManifestPath(rawPath))
	if os.IsNotExist(err) {
		return nil, nil, token, nil
	}
	if err != nil {
		return nil, nil, token, err
	}
	f, err := os.Open(workingSetZstdPayloadPath(rawPath))
	if os.IsNotExist(err) {
		return nil, nil, token, nil
	}
	if err != nil {
		return nil, nil, token, err
	}
	return f, data, token, nil
}

func (mgr *SnapshotManager) readWSRemoteContext(ctx context.Context, snap *Snapshot, path string) ([]byte, error) {
	store, ok := mgr.storage.(storage.ContextObjectStorage)
	if !ok {
		return nil, fmt.Errorf("WS metadata requires context-aware object storage")
	}
	r, err := store.OpenObject(ctx, mgr.getObjectKey(snap.GetId(), filepath.Base(path)))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// The small WS metadata may be read in full, but still honors request deadlines.
func (mgr *SnapshotManager) readWSMetadataContext(ctx context.Context, snap *Snapshot, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	revision := snap.GetId()
	mgr.wsRegistry.deletionLock.RLock()
	token := mgr.streamEpoch(revision).Load()
	data, err := os.ReadFile(path)
	if err == nil {
		mgr.registerWorkingSetAccessForPath(path)
	}
	mgr.wsRegistry.deletionLock.RUnlock()
	if err == nil {
		return data, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	data, err = mgr.readWSRemoteContext(ctx, snap, path)
	if err != nil {
		return nil, err
	}
	if !mgr.cleanChunks {
		mgr.wsStreamWrites.Add(1)
		mgr.wsStreamPending.Add(1)
		go func() {
			defer mgr.wsStreamWrites.Done()
			defer mgr.wsStreamPending.Add(-1)
			err := mgr.publishStreamCache(revision, token, []streamCacheFile{{path, data}})
			if err != nil {
				log.Warnf("WS metadata cache publish failed revision=%s: %v", revision, err)
			}
		}()
	}
	return data, nil
}

type streamCacheFile struct {
	path string
	data []byte
}

// Files are ordered payload then manifest. Publication and cache readers share
// the revision lock; eviction additionally takes the outer deletion write lock.
func (mgr *SnapshotManager) publishStreamCache(revision string, token uint64, files []streamCacheFile) error {
	tmp := make([]string, 0, len(files))
	defer func() {
		for _, path := range tmp {
			os.Remove(path)
		}
	}()
	for _, file := range files {
		path, err := writeStreamTemp(file.path, func(dst io.Writer) error { _, err := dst.Write(file.data); return err })
		if err != nil {
			return err
		}
		tmp = append(tmp, path)
	}
	mgr.wsRegistry.deletionLock.RLock()
	defer mgr.wsRegistry.deletionLock.RUnlock()
	lock := mgr.streamPublishLock(revision)
	lock.Lock()
	defer lock.Unlock()
	if mgr.streamEpoch(revision).Load() != token {
		return nil
	}
	// Remove an old commit marker first; a failed replacement must not expose a
	// previous manifest alongside a new payload. Objects within a revision are immutable.
	if len(files) == 2 {
		if err := os.Remove(files[1].path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	for i, file := range files {
		if err := os.Rename(tmp[i], file.path); err != nil {
			return err
		}
	}
	mgr.registerWorkingSetAccessForPath(files[0].path)
	return nil
}

func (mgr *SnapshotManager) getWorkingSetPathManagedContext(ctx context.Context, snap *Snapshot, rawPath string) ([]byte, func(), error) {
	if !mgr.compression.WorkingSet {
		return mgr.GetSnapshotFileContentManaged(snap, rawPath)
	}
	started := time.Now()
	file, manifestData, token, err := mgr.openCachedStreams(snap.GetId(), rawPath)
	if err != nil {
		return nil, func() {}, err
	}
	local := file != nil
	if local {
		defer file.Close()
	} else {
		if _, ok := mgr.storage.(storage.RangeObjectStorage); !ok {
			return nil, func() {}, fmt.Errorf("compressed WS requires RangeObjectStorage; whole-object fallback is disabled")
		}
		manifestData, err = mgr.readWSRemoteContext(ctx, snap, workingSetZstdManifestPath(rawPath))
		if err != nil {
			return nil, func() {}, fmt.Errorf("compressed working set manifest is required: %w", err)
		}
	}
	manifest, err := zstdstreams.ParseManifest(manifestData)
	if err != nil {
		return nil, func() {}, err
	}
	manifestUS := time.Since(started).Microseconds()
	if local {
		stat, err := file.Stat()
		if err != nil {
			return nil, func() {}, err
		}
		if stat.Size() != manifest.CompressedSize {
			return nil, func() {}, fmt.Errorf("cached streams payload size mismatch")
		}
	}
	if manifest.RawSize > int64(^uint(0)>>1) || manifest.CompressedSize > int64(^uint(0)>>1) {
		return nil, func() {}, fmt.Errorf("working set exceeds addressable memory")
	}
	var destination []byte
	mmapStart := time.Now()
	if manifest.RawSize > 0 {
		destination, err = unix.Mmap(-1, 0, int(manifest.RawSize), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
		if err != nil {
			return nil, func() {}, err
		}
	}
	mmapUS := time.Since(mmapStart).Microseconds()
	var once sync.Once
	release := func() {
		once.Do(func() {
			if len(destination) > 0 {
				if err := unix.Munmap(destination); err != nil {
					log.Warnf("unmap WS streams: %v", err)
				}
			}
		})
	}
	var compressedCache []byte
	if !local && !mgr.cleanChunks {
		compressedCache = make([]byte, int(manifest.CompressedSize))
	}
	key := mgr.getObjectKey(snap.GetId(), filepath.Base(workingSetZstdPayloadPath(rawPath)))
	open := func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
		if local {
			return io.NopCloser(io.NewSectionReader(file, off, n)), nil
		}
		r, err := mgr.storage.(storage.RangeObjectStorage).OpenObjectRange(ctx, key, off, n)
		if err != nil {
			return nil, err
		}
		if mgr.cleanChunks {
			return r, nil
		}
		return &captureReadCloser{Reader: io.TeeReader(r, &fixedSliceWriter{data: compressedCache[off : off+n]}), closer: r}, nil
	}
	stats, err := zstdstreams.DecodeWithStats(ctx, manifest, open, destination)
	if err != nil {
		release()
		return nil, func() {}, fmt.Errorf("stream-decode compressed working set: %w", err)
	}
	if local {
		mgr.wsRegistry.deletionLock.RLock()
		if mgr.streamEpoch(snap.GetId()).Load() == token {
			mgr.registerWorkingSetAccessForPath(workingSetZstdPayloadPath(rawPath))
		}
		mgr.wsRegistry.deletionLock.RUnlock()
	} else if !mgr.cleanChunks {
		mgr.wsStreamWrites.Add(1)
		mgr.wsStreamPending.Add(1)
		go func() {
			defer mgr.wsStreamWrites.Done()
			defer mgr.wsStreamPending.Add(-1)
			err := mgr.publishStreamCache(snap.GetId(), token, []streamCacheFile{
				{workingSetZstdPayloadPath(rawPath), compressedCache}, {workingSetZstdManifestPath(rawPath), manifestData},
			})
			if err != nil {
				log.Warnf("WS streams cache publish failed revision=%s: %v", snap.GetId(), err)
			}
		}()
	}
	data, _ := json.Marshal(stats)
	log.Infof("ZSTD_WS_STREAM_STATS revision=%s layout=%s stats=%s", snap.GetId(), zstdstreams.Layout, data)
	log.Infof("ZSTD_WS_DECODE revision=%s source=%s layout=%s raw_bytes=%d compressed_bytes=%d streams=%d fetchers=8 manifest_us=%d mmap_us=%d elapsed_us=%d",
		snap.GetId(), map[bool]string{true: "local", false: "remote"}[local], zstdstreams.Layout, manifest.RawSize, manifest.CompressedSize, manifest.StreamCount, manifestUS, mmapUS, time.Since(started).Microseconds())
	return destination, release, nil
}

// PendingWorkingSetCacheWrites is diagnostic only; callers must also establish
// that no invocation can schedule another write before declaring a window idle.
func (mgr *SnapshotManager) PendingWorkingSetCacheWrites() int64 {
	return mgr.wsStreamPending.Load()
}
