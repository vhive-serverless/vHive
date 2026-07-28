package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReconstructMemoryWithCacheAvoidsSecondRemoteRead(t *testing.T) {
	base := NewMemoryArtifactStore()
	input := []byte("AAAABBBBAAAA")
	recipe, err := SplitMemory(bytes.NewReader(input), 4, func(id ChunkID, data []byte) error {
		return putChunkIfAbsent(context.Background(), base, id, data)
	})
	require.NoError(t, err)
	store := &countingStore{ArtifactStore: base}
	cache, err := NewFileChunkCache(t.TempDir())
	require.NoError(t, err)

	var first bytes.Buffer
	require.NoError(t, ReconstructMemoryWithCache(context.Background(), store, cache, recipe, &first))
	require.Equal(t, input, first.Bytes())
	require.Equal(t, 2, store.GetCount(), "only unique chunks should be fetched")
	require.Eventually(t, func() bool {
		return cache.Metrics().Bytes == 8
	}, time.Second, time.Millisecond, "write-behind chunks must reach the file cache")

	store.ResetGetCount()
	var second bytes.Buffer
	require.NoError(t, ReconstructMemoryWithCache(context.Background(), store, cache, recipe, &second))
	require.Equal(t, input, second.Bytes())
	require.Zero(t, store.GetCount(), "a warm cache must not read chunks from the store")
	metrics := cache.Metrics()
	require.Equal(t, uint64(2), metrics.Misses)
	require.GreaterOrEqual(t, metrics.Hits, uint64(4))
	require.Equal(t, int64(8), metrics.Bytes)
}

func TestReconstructMemoryUsesEphemeralCacheForDuplicateChunks(t *testing.T) {
	base := NewMemoryArtifactStore()
	input := []byte("AAAABBBBAAAA")
	recipe, err := SplitMemory(bytes.NewReader(input), 4, func(id ChunkID, data []byte) error {
		return putChunkIfAbsent(context.Background(), base, id, data)
	})
	require.NoError(t, err)
	store := &countingStore{ArtifactStore: base}

	var reconstructed bytes.Buffer
	require.NoError(t, ReconstructMemory(context.Background(), store, recipe, &reconstructed))
	require.Equal(t, input, reconstructed.Bytes())
	require.Equal(t, 2, store.GetCount(), "one reconstruction should fetch each unique chunk once")
}

func TestReadRecipeChunkReturnsInMemoryCacheEntryDirectly(t *testing.T) {
	ctx := context.Background()
	data := []byte("page")
	cache := newMemoryChunkCache()
	_, err := cache.Insert(ctx, chunkID(data), data)
	require.NoError(t, err)

	got, downloaded, err := readRecipeChunk(ctx, NewMemoryArtifactStore(), cache, chunkID(data), len(data))
	require.NoError(t, err)
	require.False(t, downloaded)

	handle, err := cache.Acquire(ctx, chunkID(data))
	require.NoError(t, err)
	defer handle.Release()
	inMemory, ok := handle.(inMemoryChunkHandle)
	require.True(t, ok)
	cached, err := inMemory.cachedBytes()
	require.NoError(t, err)
	require.Equal(t, &cached[0], &got[0], "read must reuse the in-memory cache entry")
}

func TestFileChunkCacheCleanupKeepsPinnedHandleReadable(t *testing.T) {
	cache, err := NewFileChunkCache(t.TempDir())
	require.NoError(t, err)
	data := []byte("page")
	handle, err := cache.Insert(context.Background(), chunkID(data), data)
	require.NoError(t, err)

	require.NoError(t, cache.Cleanup(context.Background()))
	reader, err := handle.Open()
	require.NoError(t, err)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, data, got)

	require.NoError(t, handle.Release())
	require.NoError(t, cache.Cleanup(context.Background()))
	_, err = cache.Acquire(context.Background(), chunkID(data))
	require.True(t, errors.Is(err, ErrChunkCacheMiss))
}
