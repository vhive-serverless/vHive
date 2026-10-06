package snapshotting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
)

func streamsManager(t *testing.T, mode string, clean bool) (*SnapshotManager, *Snapshot, *memoryRangeStorage, []byte, string) {
	t.Helper()
	store := newMemoryRangeStorage()
	mgr := NewSnapshotManager(t.TempDir(), store, true, false, true, true, true, false, 4096, 1<<20, mode, 8, false, clean)
	mgr.WaitForInit()
	t.Cleanup(func() { mgr.wsStreamWrites.Wait() })
	require.NoError(t, mgr.ConfigureCompression(CompressionConfig{WorkingSet: true, Codec: CompressionCodecZstd, Level: 3, WSLayout: zstdstreams.Layout, Fetchers: 8}))
	snap, err := mgr.InitSnapshot("streams-test", "test")
	require.NoError(t, err)
	path := snap.GetWSPrivateContentFilePath()
	if mode == SecurityModeFull {
		path = snap.GetWSContentFilePath()
	}
	raw := bytes.Repeat([]byte{0x53, 0x19, 0x71, 0x27}, 4096*19)
	require.NoError(t, mgr.persistWorkingSetContent(snap.GetId(), path, raw))
	return mgr, snap, store, raw, path
}

func TestStreamsPolicyRemoteAndLocal(t *testing.T) {
	for _, mode := range []string{SecurityModeFull, SecurityModePartial, SecurityModeNoImageSharing, SecurityModeFullDedup} {
		t.Run(mode, func(t *testing.T) {
			mgr, snap, store, raw, path := streamsManager(t, mode, false)
			require.NoError(t, os.Remove(workingSetZstdPayloadPath(path)))
			require.NoError(t, os.Remove(workingSetZstdManifestPath(path)))
			out, release, err := mgr.GetWorkingSetContentManagedContext(context.Background(), snap)
			require.NoError(t, err)
			require.Equal(t, raw, out)
			release()
			release()
			mgr.wsStreamWrites.Wait()
			require.Equal(t, 8, store.rangeOpenCount())
			require.Positive(t, mgr.wsRegistry.GetTotalSizeInChunks())
			// Remove storage entirely: a complete local hit must need zero GETs,
			// including the compressed manifest.
			mgr.storage = nil
			out, release, err = mgr.GetWorkingSetContentManagedContext(context.Background(), snap)
			require.NoError(t, err)
			require.Equal(t, raw, out)
			release()
			require.Equal(t, 8, store.rangeOpenCount())
		})
	}
}

func TestStreamsMissingPrivateDoesNotFallbackToFull(t *testing.T) {
	mgr, snap, _, raw, path := streamsManager(t, SecurityModePartial, true)
	require.NoError(t, mgr.persistWorkingSetContent(snap.GetId(), snap.GetWSContentFilePath(), raw))
	require.NoError(t, os.Remove(workingSetZstdManifestPath(path)))
	mgr.storage = nil
	_, release, err := mgr.GetWorkingSetContentManagedContext(context.Background(), snap)
	release()
	require.Error(t, err)
}

func TestStreamsRequiresPrivateIndex(t *testing.T) {
	mgr, snap, _, _, _ := streamsManager(t, SecurityModePartial, true)
	_, release, err := mgr.GetWorkingSetContentSourcesManagedContext(context.Background(), snap)
	release()
	require.ErrorContains(t, err, "required private WS index")
}

func TestStreamsCorruptRemoteNotPublished(t *testing.T) {
	mgr, snap, store, _, path := streamsManager(t, SecurityModeFull, false)
	require.NoError(t, os.Remove(workingSetZstdPayloadPath(path)))
	require.NoError(t, os.Remove(workingSetZstdManifestPath(path)))
	key := mgr.getObjectKey(snap.GetId(), filepath.Base(workingSetZstdPayloadPath(path)))
	payload, _ := store.get(key)
	payload[6] ^= 0xff
	store.put(key, payload)
	_, release, err := mgr.GetWorkingSetContentManagedContext(context.Background(), snap)
	release()
	require.Error(t, err)
	mgr.wsStreamWrites.Wait()
	_, err = os.Stat(workingSetZstdPayloadPath(path))
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(workingSetZstdManifestPath(path))
	require.True(t, os.IsNotExist(err))
}

func TestStreamsExpiredPublishDoesNotResurrect(t *testing.T) {
	mgr, snap, _, _, path := streamsManager(t, SecurityModeFull, false)
	token := mgr.streamEpoch(snap.GetId()).Load()
	payload, err := os.ReadFile(workingSetZstdPayloadPath(path))
	require.NoError(t, err)
	meta, err := os.ReadFile(workingSetZstdManifestPath(path))
	require.NoError(t, err)
	mgr.wsRegistry.deletionLock.Lock()
	require.NoError(t, mgr.removeWorkingSetFiles(snap.GetId()))
	mgr.wsRegistry.deletionLock.Unlock()
	require.NoError(t, mgr.publishStreamCache(snap.GetId(), token, []streamCacheFile{{workingSetZstdPayloadPath(path), payload}, {workingSetZstdManifestPath(path), meta}}))
	f, _, _, err := mgr.openCachedStreams(snap.GetId(), path)
	require.NoError(t, err)
	require.Nil(t, f)
	_, err = os.Stat(workingSetZstdPayloadPath(path))
	require.True(t, os.IsNotExist(err))
}

func TestStreamsConcurrentPublishReadEvict(t *testing.T) {
	mgr, snap, _, _, path := streamsManager(t, SecurityModeFull, false)
	payload, err := os.ReadFile(workingSetZstdPayloadPath(path))
	require.NoError(t, err)
	meta, err := os.ReadFile(workingSetZstdManifestPath(path))
	require.NoError(t, err)
	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for kind := 0; kind < 3; kind++ {
		kind := kind
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				switch kind {
				case 0:
					token := mgr.streamEpoch(snap.GetId()).Load()
					if err := mgr.publishStreamCache(snap.GetId(), token, []streamCacheFile{{workingSetZstdPayloadPath(path), payload}, {workingSetZstdManifestPath(path), meta}}); err != nil {
						errs <- err
						return
					}
				case 1:
					f, data, _, err := mgr.openCachedStreams(snap.GetId(), path)
					if err != nil {
						errs <- err
						return
					}
					if f != nil {
						got, err := io.ReadAll(f)
						f.Close()
						if err != nil || !bytes.Equal(got, payload) || !bytes.Equal(data, meta) {
							errs <- fmt.Errorf("partial cache hit: %v", err)
							return
						}
					}
				case 2:
					mgr.wsRegistry.deletionLock.Lock()
					err := mgr.removeWorkingSetFiles(snap.GetId())
					mgr.wsRegistry.deletionLock.Unlock()
					if err != nil {
						errs <- err
						return
					}
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

func TestStreamsRejectOldConfiguration(t *testing.T) {
	mgr, _, _, _, _ := streamsManager(t, SecurityModeFull, true)
	for _, cfg := range []CompressionConfig{
		{WorkingSet: true, Codec: CompressionCodecZstd, Level: 3, Fetchers: 10},
		{WorkingSet: true, Codec: CompressionCodecZstd, Level: 3, Fetchers: 8, FrameSize: 1 << 20},
		{WorkingSet: true, Codec: CompressionCodecZstd, Level: 3, Fetchers: 8, WSLayout: "frames-v1"},
	} {
		require.Error(t, mgr.ConfigureCompression(cfg))
	}
}
