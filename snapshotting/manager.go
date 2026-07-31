// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package snapshotting

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	// Stored snapshots (identified by the function instance revision, which is provided by the `K_REVISION` environment
	// variable of knative).
	snapshots  map[string]*Snapshot
	baseFolder string
	catalog    Catalog
	remote     *remoteSnapshotTransfer
	// diskCacheLimit bounds local chunk and working-set cache data. A negative
	// value leaves the historical unbounded behaviour in place.
	diskCacheLimit         int64
	diskCacheEvictions     uint64
	diskCacheEvictedBytes  int64
	diskCacheRemoteReloads uint64
}

// EnableRemoteTransfer makes committed snapshots available to other workers
// through store.  It is deliberately opt-in: without this call the manager
// retains the original local-only behaviour.
func (mgr *SnapshotManager) EnableRemoteTransfer(store ArtifactStore, cacheSnaps bool) {
	mgr.Lock()
	defer mgr.Unlock()
	if store == nil {
		mgr.remote = nil
		return
	}
	mgr.remote = newRemoteSnapshotTransfer(store, cacheSnaps)
}

// EnableChunkedMemory opts remote publication into content-addressed chunks.
// It must be called after EnableRemoteTransfer. Passing zero disables it and
// keeps the stage-3 whole-memory-file protocol.
func (mgr *SnapshotManager) EnableChunkedMemory(chunkSize int) error {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return fmt.Errorf("remote transfer is not enabled")
	}
	if chunkSize == 0 {
		remote.mu.Lock()
		remote.chunkSize = 0
		remote.mu.Unlock()
		return nil
	}
	return remote.enableChunkedMemory(chunkSize)
}

// EnableMemoryReconstruction controls whether a chunked remote snapshot is
// materialized into a complete local memory file during download. It is
// disabled by default, so consumers can supply memory directly from the recipe
// (for example, through UFFD). Enable it for consumers that require a local
// memory file.
//
// It must be called after EnableRemoteTransfer.
func (mgr *SnapshotManager) EnableMemoryReconstruction(enabled bool) error {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return fmt.Errorf("remote transfer is not enabled")
	}
	remote.setMemoryReconstruction(enabled)
	return nil
}

// EnableChunkCache uses directory for recipe chunk reuse during remote
// reconstruction. It is independent of snapshot retention and affects only
// chunked remote snapshots. Passing an empty directory disables the cache.
func (mgr *SnapshotManager) EnableChunkCache(directory string) error {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return fmt.Errorf("remote transfer is not enabled")
	}
	if directory == "" {
		remote.setChunkCache(nil)
		return nil
	}
	cache, err := NewFileChunkCache(directory)
	if err != nil {
		return err
	}
	mgr.Lock()
	limit := mgr.diskCacheLimit
	mgr.Unlock()
	if limit >= 0 {
		if err := cache.SetCapacity(limit); err != nil {
			return err
		}
	}
	remote.setChunkCache(cache)
	return nil
}

// SetDiskCacheSize bounds on-disk snapshot cache data in bytes. The budget is
// shared by recipe chunks and downloaded working-set files. A negative value
// disables the limit. Evicted data is deliberately not treated as a snapshot
// failure: AcquireSnapshotContext detects an evicted working set and fetches a
// fresh copy from the configured remote store.
func (mgr *SnapshotManager) SetDiskCacheSize(bytes int64) error {
	if bytes < -1 {
		return fmt.Errorf("snapshot disk cache size must be non-negative or -1")
	}
	mgr.Lock()
	mgr.diskCacheLimit = bytes
	mgr.Unlock()
	return mgr.trimDiskCache(context.Background(), "")
}

// EnableDiskCacheSize is a compatibility-friendly spelling for configuring a
// bounded snapshot cache.
func (mgr *SnapshotManager) EnableDiskCacheSize(bytes int64) error {
	return mgr.SetDiskCacheSize(bytes)
}

// SnapshotDiskCacheMetrics makes cache activity observable to benchmark and
// operational callers. RemoteReloads counts acquires that found an evicted
// working set and therefore had to fetch the snapshot again.
type SnapshotDiskCacheMetrics struct {
	WorkingSetEvictions uint64
	WorkingSetBytes     int64
	RemoteReloads       uint64
}

func (mgr *SnapshotManager) DiskCacheMetrics() SnapshotDiskCacheMetrics {
	mgr.Lock()
	defer mgr.Unlock()
	return SnapshotDiskCacheMetrics{
		WorkingSetEvictions: mgr.diskCacheEvictions,
		WorkingSetBytes:     mgr.diskCacheEvictedBytes,
		RemoteReloads:       mgr.diskCacheRemoteReloads,
	}
}

// EnableProvenanceWorkingSets makes subsequent working-set publication split
// coalesced pages according to policy. It is opt-in; without it the historical
// revision-local coalesced artifact remains the only working-set format.
func (mgr *SnapshotManager) EnableProvenanceWorkingSets(policy ProvenancePolicy, baseIdentity string) error {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return fmt.Errorf("remote transfer is not enabled")
	}
	repository, err := NewWorkingSetRepository(remote.store, policy, baseIdentity)
	if err != nil {
		return err
	}
	remote.setWorkingSetRepository(repository)
	return nil
}

// CleanupChunkCache removes only unpinned chunk files from the configured
// cache. It has no effect when no chunk cache is enabled.
func (mgr *SnapshotManager) CleanupChunkCache(ctx context.Context) error {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return nil
	}
	remote.mu.Lock()
	cache := remote.chunkCache
	remote.mu.Unlock()
	if cache == nil {
		return nil
	}
	return cache.Cleanup(ctx)
}

func (mgr *SnapshotManager) ChunkCacheMetrics() ChunkCacheMetrics {
	mgr.Lock()
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil {
		return ChunkCacheMetrics{}
	}
	remote.mu.Lock()
	cache := remote.chunkCache
	remote.mu.Unlock()
	if cache == nil {
		return ChunkCacheMetrics{}
	}
	return cache.Metrics()
}

// Snapshot identified by VM id

func NewSnapshotManager(baseFolder string) *SnapshotManager {
	manager := new(SnapshotManager)
	manager.snapshots = make(map[string]*Snapshot)
	manager.baseFolder = baseFolder
	manager.diskCacheLimit = -1

	// Clean & init basefolder
	_ = os.RemoveAll(manager.baseFolder)
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)
	catalog, err := NewLocalCatalog(manager.baseFolder)
	manager.catalog = catalog
	if err != nil {
		// Preserve the constructor's historical no-error API. Operations will
		// return the filesystem error instead of panicking if initialization was
		// not possible.
		manager.catalog = &LocalCatalog{baseFolder: manager.baseFolder}
	}

	return manager
}

// AcquireSnapshot returns a snapshot for the specified revision if it is available.
func (mgr *SnapshotManager) AcquireSnapshot(revision string) (*Snapshot, error) {
	return mgr.AcquireSnapshotContext(context.Background(), revision)
}

// AcquireSnapshotContext returns a local snapshot, fetching a committed remote
// copy on a local cache miss when remote transfer has been enabled. When
// snapshot caching is disabled, it removes any previous local copy first so
// every acquire fetches the snapshot from remote storage.
func (mgr *SnapshotManager) AcquireSnapshotContext(ctx context.Context, revision string) (*Snapshot, error) {
	mgr.Lock()
	remote := mgr.remote
	catalog := mgr.catalog
	if remote != nil && !remote.cacheSnaps {
		delete(mgr.snapshots, revision)
		mgr.Unlock()
		if err := catalog.Delete(revision); err != nil && !errors.Is(err, ErrSnapshotNotFound) {
			return nil, fmt.Errorf("remove uncached local snapshot %s: %w", revision, err)
		}
		mgr.Lock()
	}
	descriptor, err := catalog.Get(revision)
	if err == nil {
		if remote != nil && !mgr.hasWorkingSetFiles(descriptor) {
			mgr.Unlock()
			mgr.Lock()
			mgr.diskCacheRemoteReloads++
			mgr.Unlock()
			log.WithField("revision", revision).Info("Snapshot working set was evicted; re-fetching remote snapshot")
			if err := catalog.Delete(revision); err != nil {
				return nil, fmt.Errorf("remove evicted local snapshot %s: %w", revision, err)
			}
			return mgr.AcquireSnapshotContext(ctx, revision)
		}
		// A snapshot created locally predates remote publication. Chunked
		// publication records the recipe in the catalog, but that original
		// in-memory Snapshot cannot see the new recipe marker. Rebuild the
		// filesystem adapter from the descriptor so recipe-backed restores use
		// UFFD instead of attempting to open a non-existent mem_file.
		if snap, ok := mgr.snapshots[revision]; ok && descriptor.MemoryRecipe == "" {
			mgr.Unlock()
			mgr.touchWorkingSet(descriptor)
			_ = mgr.trimDiskCache(ctx, revision)
			return snap, nil
		}
		mgr.Unlock()
		mgr.touchWorkingSet(descriptor)
		_ = mgr.trimDiskCache(ctx, revision)
		return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor), nil
	}
	mgr.Unlock()
	if remote == nil || (!errors.Is(err, ErrSnapshotNotFound) && !(errors.Is(err, ErrSnapshotNotReady) && remote.hasDownload(revision))) {
		return nil, err
	}

	descriptor, err = remote.download(ctx, mgr.catalog, mgr.baseFolder, revision)
	if err != nil {
		return nil, err
	}
	mgr.touchWorkingSet(descriptor)
	if err := mgr.trimDiskCache(ctx, revision); err != nil {
		return nil, err
	}
	return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor), nil
}

func (mgr *SnapshotManager) hasWorkingSetFiles(desc *SnapshotDescriptor) bool {
	for _, artifact := range []struct {
		enabled bool
		name    string
	}{
		{desc.WorkingSetTrace, desc.Artifacts.WorkingSetTrace},
		{desc.WorkingSet, desc.Artifacts.WorkingSetPages},
	} {
		if !artifact.enabled {
			continue
		}
		if _, err := os.Stat(filepath.Join(mgr.baseFolder, desc.Revision, artifact.name)); err != nil {
			return false
		}
	}
	return true
}

func (mgr *SnapshotManager) touchWorkingSet(desc *SnapshotDescriptor) {
	if desc == nil {
		return
	}
	now := time.Now()
	for _, artifact := range []struct {
		enabled bool
		name    string
	}{
		{desc.WorkingSetTrace, desc.Artifacts.WorkingSetTrace},
		{desc.WorkingSet, desc.Artifacts.WorkingSetPages},
	} {
		if artifact.enabled {
			_ = os.Chtimes(filepath.Join(mgr.baseFolder, desc.Revision, artifact.name), now, now)
		}
	}
}

type workingSetCacheFile struct {
	path     string
	size     int64
	access   time.Time
	revision string
}

// trimDiskCache removes the least-recently-used working-set files before
// handing the remaining budget to the chunk cache. keepRevision is the
// snapshot currently being acquired and is never evicted in that operation.
func (mgr *SnapshotManager) trimDiskCache(ctx context.Context, keepRevision string) error {
	mgr.Lock()
	limit, baseFolder, remote := mgr.diskCacheLimit, mgr.baseFolder, mgr.remote
	mgr.Unlock()
	// Local-only snapshots have no recovery source, so they are never treated
	// as cache entries.
	if limit < 0 || remote == nil {
		return nil
	}
	var cache *FileChunkCache
	if remote != nil {
		remote.mu.Lock()
		cache, _ = remote.chunkCache.(*FileChunkCache)
		remote.mu.Unlock()
	}
	chunkBytes := int64(0)
	if cache != nil {
		// First account for persisted chunks from earlier processes.
		if err := cache.SetCapacity(-1); err != nil {
			return err
		}
		chunkBytes = cache.Metrics().Bytes
	}
	files, err := findWorkingSetCacheFiles(baseFolder, keepRevision)
	if err != nil {
		return err
	}
	workingSetBytes := int64(0)
	for _, file := range files {
		workingSetBytes += file.size
	}
	sort.Slice(files, func(i, j int) bool { return files[i].access.Before(files[j].access) })
	for _, file := range files {
		if workingSetBytes+chunkBytes <= limit {
			break
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(file.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict cached working set %s: %w", file.path, err)
		}
		mgr.Lock()
		mgr.diskCacheEvictions++
		mgr.diskCacheEvictedBytes += file.size
		mgr.Unlock()
		log.WithFields(log.Fields{
			"revision": file.revision,
			"artifact": filepath.Base(file.path),
			"bytes":    file.size,
			"limit":    limit,
		}).Info("Evicted snapshot working-set cache artifact")
		workingSetBytes -= file.size
	}
	if cache != nil {
		return cache.SetCapacity(limit - workingSetBytes)
	}
	return nil
}

func findWorkingSetCacheFiles(baseFolder, keepRevision string) ([]workingSetCacheFile, error) {
	dirs, err := os.ReadDir(baseFolder)
	if err != nil {
		return nil, fmt.Errorf("read snapshot cache directory: %w", err)
	}
	var files []workingSetCacheFile
	for _, dir := range dirs {
		if !dir.IsDir() || dir.Name() == keepRevision {
			continue
		}
		for _, name := range []string{defaultArtifactNames().WorkingSetTrace, defaultArtifactNames().WorkingSetPages} {
			path := filepath.Join(baseFolder, dir.Name(), name)
			info, err := os.Stat(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("stat cached working set %s: %w", path, err)
			}
			if info.Mode().IsRegular() {
				files = append(files, workingSetCacheFile{path: path, size: info.Size(), access: info.ModTime(), revision: dir.Name()})
			}
		}
	}
	return files, nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has
// been created, CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available for use.
func (mgr *SnapshotManager) InitSnapshot(revision, image string) (*Snapshot, error) {
	mgr.Lock()

	logger := log.WithFields(log.Fields{"revision": revision, "image": image})
	logger.Debug("Initializing snapshot corresponding to revision and image")

	if exists, err := mgr.catalog.Exists(revision); err != nil {
		mgr.Unlock()
		return nil, err
	} else if exists {
		mgr.Unlock()
		return nil, fmt.Errorf("add: snapshot for revision %s already exists", revision)
	}

	// Create snapshot object and move into creating state
	descriptor, err := mgr.catalog.Begin(revision, image)
	if err != nil {
		mgr.Unlock()
		return nil, err
	}
	snap := NewSnapshotFromDescriptor(mgr.baseFolder, descriptor)
	mgr.snapshots[snap.GetId()] = snap
	mgr.Unlock()

	return snap, nil
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *SnapshotManager) CommitSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	if err := mgr.catalog.Commit(revision); err != nil {
		return err
	}
	if snap, ok := mgr.snapshots[revision]; ok {
		snap.ready = true
	}
	return nil
}

// PublishSnapshot uploads a locally committed snapshot.  The remote descriptor
// is published last, so a remote reader never accepts a partial transfer.
// When cacheSnaps is false, successful publication removes the local copy.
func (mgr *SnapshotManager) PublishSnapshot(ctx context.Context, revision string) error {
	mgr.Lock()
	remote := mgr.remote
	catalog := mgr.catalog
	baseFolder := mgr.baseFolder
	mgr.Unlock()
	if remote == nil {
		return nil
	}
	if err := remote.publish(ctx, catalog, baseFolder, revision); err != nil {
		return err
	}
	if !remote.cacheSnaps {
		mgr.Lock()
		delete(mgr.snapshots, revision)
		mgr.Unlock()
		if err := catalog.Delete(revision); err != nil {
			return fmt.Errorf("remove published local snapshot %s: %w", revision, err)
		}
	}
	return nil
}

// PublishWorkingSet updates only the optional working-set artifacts of an
// already published snapshot. It is intended for a restored snapshot after
// shutdown, when chunked memory may be recipe-backed and therefore has no
// local memory file to upload.
func (mgr *SnapshotManager) PublishWorkingSet(ctx context.Context, revision string) error {
	mgr.Lock()
	remote := mgr.remote
	catalog := mgr.catalog
	baseFolder := mgr.baseFolder
	mgr.Unlock()
	if remote == nil {
		return nil
	}
	published, err := remote.publishWorkingSet(ctx, catalog, baseFolder, revision)
	if err != nil || !published {
		return err
	}
	if !remote.cacheSnaps {
		mgr.Lock()
		delete(mgr.snapshots, revision)
		mgr.Unlock()
		if err := catalog.Delete(revision); err != nil {
			return fmt.Errorf("remove published local working set %s: %w", revision, err)
		}
	}
	return nil
}

func (mgr *SnapshotManager) RemoteTransferEnabled() bool {
	mgr.Lock()
	defer mgr.Unlock()
	return mgr.remote != nil
}

// Catalog exposes the lifecycle port while SnapshotManager remains as a
// compatibility adapter for callers that need a local Snapshot object.
func (mgr *SnapshotManager) Catalog() Catalog {
	return mgr.catalog
}

// SnapshotForDescriptor returns the existing filesystem adapter for a catalog
// entry without performing another readiness lookup.
func (mgr *SnapshotManager) SnapshotForDescriptor(descriptor *SnapshotDescriptor) *Snapshot {
	return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor)
}
