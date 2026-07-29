package snapshotting

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRecipeReadsLegacyJSON(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryArtifactStore()
	recipe := MemoryRecipe{Version: memoryRecipeVersion, ChunkSize: 4, Chunks: []RecipeChunk{{ID: chunkID([]byte("page"))}}}
	data, err := json.Marshal(recipe)
	require.NoError(t, err)
	key, err := RevisionArtifactKey("legacy-recipe", memoryRecipeArtifact)
	require.NoError(t, err)
	require.NoError(t, store.Put(ctx, key, bytes.NewReader(data), int64(len(data))))

	got, err := getRecipe(ctx, store, "legacy-recipe")
	require.NoError(t, err)
	require.Equal(t, recipe, got)
}

func TestSplitMemoryPreservesOrderAndDuplicates(t *testing.T) {
	input := []byte("abcabcabc")
	var got [][]byte
	recipe, err := SplitMemory(bytes.NewReader(input), 3, func(_ ChunkID, chunk []byte) error {
		got = append(got, append([]byte(nil), chunk...))
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, [][]byte{[]byte("abc"), []byte("abc"), []byte("abc")}, got)
	require.Len(t, recipe.Chunks, 3)
	require.Equal(t, recipe.Chunks[0].ID, recipe.Chunks[1].ID)
	require.NoError(t, recipe.Validate())
}

func TestSplitMemoryRejectsPartialFinalChunk(t *testing.T) {
	_, err := SplitMemory(bytes.NewReader([]byte("abcabcde")), 3, func(ChunkID, []byte) error { return nil })
	require.ErrorContains(t, err, "not divisible")
}

func TestSplitMemoryEmptyInput(t *testing.T) {
	recipe, err := SplitMemory(bytes.NewReader(nil), 4, func(ChunkID, []byte) error { return nil })
	require.NoError(t, err)
	require.Empty(t, recipe.Chunks)
	require.NoError(t, recipe.Validate())
}

func TestReconstructMemoryRejectsMalformedRecipeAndAcceptsOpaqueChunkPayload(t *testing.T) {
	store := NewMemoryArtifactStore()
	recipe := MemoryRecipe{Version: memoryRecipeVersion, ChunkSize: 4, Chunks: []RecipeChunk{{ID: ChunkID("not-a-hash")}}}
	require.Error(t, ReconstructMemory(context.Background(), store, recipe, io.Discard))

	id := chunkID([]byte("good"))
	key, err := chunkArtifactKey(id)
	require.NoError(t, err)
	require.NoError(t, store.Put(context.Background(), key, bytes.NewReader([]byte("evil")), 4))
	recipe = MemoryRecipe{Version: memoryRecipeVersion, ChunkSize: 4, Chunks: []RecipeChunk{{ID: id}}}
	var reconstructed bytes.Buffer
	require.NoError(t, ReconstructMemory(context.Background(), store, recipe, &reconstructed))
	require.Equal(t, []byte("evil"), reconstructed.Bytes())
}

func TestReadRemoteChunkAllocatesAndValidatesDeclaredSize(t *testing.T) {
	const size = 4
	id := chunkID([]byte("identity"))
	key, err := chunkArtifactKey(id)
	require.NoError(t, err)

	for name, payload := range map[string][]byte{
		"short": []byte("bad"),
		"long":  []byte("longer"),
	} {
		t.Run(name, func(t *testing.T) {
			store := NewMemoryArtifactStore()
			require.NoError(t, store.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload))))
			_, err := readRemoteChunk(context.Background(), store, id, size)
			require.ErrorContains(t, err, "corrupt size")
		})
	}

	store := NewMemoryArtifactStore()
	require.NoError(t, store.Put(context.Background(), key, bytes.NewReader([]byte("good")), size))
	data, err := readRemoteChunk(context.Background(), store, id, size)
	require.NoError(t, err)
	require.Equal(t, []byte("good"), data)
	require.Equal(t, size, cap(data), "the chunk buffer should be allocated at its declared size")
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
