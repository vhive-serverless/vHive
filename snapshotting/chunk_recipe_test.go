package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitMemoryPreservesOrderDuplicatesAndFinalChunk(t *testing.T) {
	input := []byte("abcabcde")
	var got [][]byte
	recipe, err := SplitMemory(bytes.NewReader(input), 3, func(_ ChunkID, chunk []byte) error {
		got = append(got, append([]byte(nil), chunk...))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("abc"), []byte("abc"), []byte("de")}, got)
	require.Len(t, recipe.Chunks, 3)
	require.Equal(t, recipe.Chunks[0].ID, recipe.Chunks[1].ID)
	require.Equal(t, 2, recipe.Chunks[2].Size)
	require.NoError(t, recipe.Validate())
}

func TestSplitMemoryEmptyInput(t *testing.T) {
	recipe, err := SplitMemory(bytes.NewReader(nil), 4, func(ChunkID, []byte) error { return nil })
	require.NoError(t, err)
	require.Empty(t, recipe.Chunks)
	require.NoError(t, recipe.Validate())
}

func TestReconstructMemoryRejectsCorruptRecipeAndChunk(t *testing.T) {
	store := NewMemoryArtifactStore()
	recipe := MemoryRecipe{Version: 1, ChunkSize: 4, Chunks: []RecipeChunk{{ID: ChunkID("not-a-hash"), Size: 1}}}
	require.Error(t, ReconstructMemory(context.Background(), store, recipe, io.Discard))

	id := chunkID([]byte("good"))
	key, err := chunkArtifactKey(id)
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), key, bytes.NewReader([]byte("evil")), 4))
	recipe = MemoryRecipe{Version: 1, ChunkSize: 4, Chunks: []RecipeChunk{{ID: id, Size: 4}}}
	require.Error(t, ReconstructMemory(context.Background(), store, recipe, io.Discard))
}

func TestChunkPutIfAbsentSupportsDuplicatesAndConcurrentWriters(t *testing.T) {
	store := NewMemoryArtifactStore()
	data := []byte("same page")
	id := chunkID(data)
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range cap(errs) {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- putChunkIfAbsent(context.Background(), store, id, data) }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	objects, err := store.List(context.Background(), "shared/chunks/")
	require.NoError(t, err)
	require.Len(t, objects, 1)
}

func TestUploadChunkedMemoryPropagatesStoreFailure(t *testing.T) {
	store := NewMemoryArtifactStore()
	store.SetFailures(ArtifactStoreFailures{Put: errors.New("store unavailable")})
	_, err := SplitMemory(bytes.NewReader([]byte("page")), 4, func(id ChunkID, data []byte) error {
		return putChunkIfAbsent(context.Background(), store, id, data)
	})
	require.ErrorContains(t, err, "store unavailable")
}
