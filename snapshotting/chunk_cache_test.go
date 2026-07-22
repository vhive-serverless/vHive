package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

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
