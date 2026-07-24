package snapshotting

import (
	"bytes"
	"context"
	"io"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func createPublishedSnapshot(t *testing.T, base, revision string, store ArtifactStore, cacheSnaps bool) *SnapshotManager {
	t.Helper()
	manager := NewSnapshotManager(base)
	manager.EnableRemoteTransfer(store, cacheSnaps)
	snapshot, err := manager.InitSnapshot(revision, "example:image")
	require.NoError(t, err)
	require.NoError(t, snapshot.CreateSnapDir())
	require.NoError(t, os.WriteFile(snapshot.GetSnapshotFilePath(), []byte("vm-state"), 0600))
	require.NoError(t, os.WriteFile(snapshot.GetMemFilePath(), fixedMemoryFixture(16), 0600))
	require.NoError(t, snapshot.SerializeSnapInfo())
	require.NoError(t, manager.CommitSnapshot(revision))
	require.NoError(t, manager.PublishSnapshot(context.Background(), revision))
	return manager
}

func TestRemoteWholeFileSnapshotRoundTripAcrossWorkers(t *testing.T) {
	store := NewMemoryArtifactStore()
	source := createPublishedSnapshot(t, t.TempDir(), "revision-a", store, false)
	_, err := source.Catalog().Get("revision-a")
	require.ErrorIs(t, err, ErrSnapshotNotFound, "cacheSnaps=false removes the published local copy")

	worker := NewSnapshotManager(t.TempDir())
	worker.EnableRemoteTransfer(store, true)
	snapshot, err := worker.AcquireSnapshotContext(context.Background(), "revision-a")
	require.NoError(t, err)
	state, err := os.ReadFile(snapshot.GetSnapshotFilePath())
	require.NoError(t, err)
	memory, err := os.ReadFile(snapshot.GetMemFilePath())
	require.NoError(t, err)
	require.Equal(t, []byte("vm-state"), state)
	require.Equal(t, fixedMemoryFixture(16), memory)
	_, err = worker.Catalog().Get("revision-a")
	require.NoError(t, err, "download commits only after all artifacts validate")
}

func TestRemoteWholeFileSnapshotMissingMetadataLeavesNoLocalEntry(t *testing.T) {
	store := NewMemoryArtifactStore()
	createPublishedSnapshot(t, t.TempDir(), "revision-a", store, true)
	// Replacing the store makes a deliberately incomplete remote publication.
	incomplete := NewMemoryArtifactStore()
	for _, artifact := range []string{remoteDescriptorArtifact, defaultArtifactNames().VMState, defaultArtifactNames().Memory} {
		key, keyErr := RevisionArtifactKey("revision-a", artifact)
		require.NoError(t, keyErr)
		reader, getErr := store.Get(context.Background(), key)
		require.NoError(t, getErr)
		data, readErr := io.ReadAll(reader)
		require.NoError(t, readErr)
		require.NoError(t, reader.Close())
		require.NoError(t, incomplete.Put(context.Background(), key, bytes.NewReader(data), int64(len(data))))
	}
	worker := NewSnapshotManager(t.TempDir())
	worker.EnableRemoteTransfer(incomplete, true)
	_, err := worker.AcquireSnapshotContext(context.Background(), "revision-a")
	require.Error(t, err)
	_, err = worker.Catalog().Get("revision-a")
	require.ErrorIs(t, err, ErrSnapshotNotFound)
}

func TestRemoteWholeFileSnapshotConcurrentDownloadUsesOneTransfer(t *testing.T) {
	base := NewMemoryArtifactStore()
	createPublishedSnapshot(t, t.TempDir(), "revision-a", base, true)
	store := &countingStore{ArtifactStore: base}
	worker := NewSnapshotManager(t.TempDir())
	worker.EnableRemoteTransfer(store, true)

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < cap(errs); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := worker.AcquireSnapshotContext(context.Background(), "revision-a")
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 5, store.gets, "one descriptor, state, memory, metadata, and optional patch lookup")
}

func TestRemoteChunkedMemoryRoundTripDeduplicatesAcrossSnapshots(t *testing.T) {
	store := NewMemoryArtifactStore()
	base := t.TempDir()
	first := NewSnapshotManager(base)
	first.EnableRemoteTransfer(store, false)
	require.NoError(t, first.EnableChunkedMemory(4))
	for revision, memory := range map[string][]byte{
		"revision-a": []byte("AAAABBBBCCCC"),
		"revision-b": []byte("AAAABBBBDDDD"),
	} {
		snapshot, err := first.InitSnapshot(revision, "example:image")
		require.NoError(t, err)
		require.NoError(t, snapshot.CreateSnapDir())
		require.NoError(t, os.WriteFile(snapshot.GetSnapshotFilePath(), []byte("vm-state"), 0600))
		require.NoError(t, os.WriteFile(snapshot.GetMemFilePath(), memory, 0600))
		require.NoError(t, snapshot.SerializeSnapInfo())
		require.NoError(t, first.CommitSnapshot(revision))
		require.NoError(t, first.PublishSnapshot(context.Background(), revision))
	}
	chunks, err := store.List(context.Background(), "shared/chunks/")
	require.NoError(t, err)
	require.Len(t, chunks, 4, "two repeated chunks are stored once across both snapshots")

	worker := NewSnapshotManager(t.TempDir())
	worker.EnableRemoteTransfer(store, true)
	require.NoError(t, worker.EnableMemoryReconstruction(true))
	for revision, want := range map[string][]byte{"revision-a": []byte("AAAABBBBCCCC"), "revision-b": []byte("AAAABBBBDDDD")} {
		snapshot, err := worker.AcquireSnapshotContext(context.Background(), revision)
		require.NoError(t, err)
		require.True(t, snapshot.HasMemoryRecipe(), "downloaded recipe marker is retained for lazy restore")
		got, err := os.ReadFile(snapshot.GetMemFilePath())
		require.NoError(t, err)
		require.Equal(t, want, got)
		reacquired, err := worker.AcquireSnapshotContext(context.Background(), revision)
		require.NoError(t, err)
		require.True(t, reacquired.HasMemoryRecipe(), "local catalog retains recipe marker")
	}
}

func TestRemoteChunkedMemoryDefaultsToRecipeOnly(t *testing.T) {
	store := NewMemoryArtifactStore()
	source := NewSnapshotManager(t.TempDir())
	source.EnableRemoteTransfer(store, false)
	require.NoError(t, source.EnableChunkedMemory(4))
	snapshot, err := source.InitSnapshot("revision-a", "example:image")
	require.NoError(t, err)
	require.NoError(t, snapshot.CreateSnapDir())
	require.NoError(t, os.WriteFile(snapshot.GetSnapshotFilePath(), []byte("vm-state"), 0600))
	require.NoError(t, os.WriteFile(snapshot.GetMemFilePath(), []byte("AAAABBBB"), 0600))
	require.NoError(t, snapshot.SerializeSnapInfo())
	require.NoError(t, source.CommitSnapshot("revision-a"))
	require.NoError(t, source.PublishSnapshot(context.Background(), "revision-a"))

	worker := NewSnapshotManager(t.TempDir())
	worker.EnableRemoteTransfer(store, true)
	downloaded, err := worker.AcquireSnapshotContext(context.Background(), "revision-a")
	require.NoError(t, err)
	require.True(t, downloaded.HasMemoryRecipe())
	_, err = os.Stat(downloaded.GetMemFilePath())
	require.ErrorIs(t, err, os.ErrNotExist, "recipe-only download must not reconstruct memory")
}

type countingStore struct {
	ArtifactStore
	mu   sync.Mutex
	gets int
}

func (s *countingStore) Get(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	s.mu.Lock()
	s.gets++
	s.mu.Unlock()
	return s.ArtifactStore.Get(ctx, key)
}

func (s *countingStore) GetCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gets
}

func (s *countingStore) ResetGetCount() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets = 0
}
