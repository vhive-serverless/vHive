package snapshotting

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/vhive-serverless/vhive/memory/manager"
)

// NewRecipePageSource turns an immutable memory recipe into the manager's
// page-source contract. The caller gives it to RestoreMaterializer when lazy
// restore has been selected.
func NewRecipePageSource(store ArtifactStore, cache ChunkCache, recipe MemoryRecipe) (manager.PageSource, error) {
	if err := recipe.Validate(); err != nil {
		return nil, err
	}
	if store == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	// A page source represents one restore. When no persistent cache has been
	// selected, retain fetched chunks in memory for that source's lifetime so
	// repeated page faults do not re-download identical recipe chunks.
	if cache == nil {
		cache = newMemoryChunkCache()
	}
	return &recipePageSource{store: store, cache: cache, recipe: recipe}, nil
}

// NewRecipePageSourceForRevision loads a committed revision's recipe and
// exposes it through the memory manager's lazy page-source contract.
func NewRecipePageSourceForRevision(ctx context.Context, store ArtifactStore, cache ChunkCache, revision string) (manager.PageSource, error) {
	recipe, err := getRecipe(ctx, store, revision)
	if err != nil {
		return nil, err
	}
	return NewRecipePageSource(store, cache, recipe)
}

type recipePageSource struct {
	store            ArtifactStore
	cache            ChunkCache
	recipe           MemoryRecipe
	downloadedChunks atomic.Uint64
	closed           bool
}

func (s *recipePageSource) ReadAt(ctx context.Context, offset uint64, length uint64) (manager.PageData, error) {
	if s.closed {
		return manager.PageData{}, fmt.Errorf("recipe page source is closed")
	}
	if length == 0 {
		return manager.PageData{}, fmt.Errorf("page length must be non-zero")
	}
	if offset > uint64(^uint(0)>>1) || length > uint64(^uint(0)>>1)-offset {
		return manager.PageData{}, fmt.Errorf("page range overflows host integer")
	}
	result := make([]byte, int(length))
	chunkSize := uint64(s.recipe.ChunkSize)
	chunkIndex := offset / chunkSize
	chunkOffset := offset % chunkSize
	written := 0
	for written < len(result) {
		if chunkIndex >= uint64(len(s.recipe.Chunks)) {
			return manager.PageData{}, fmt.Errorf("page range [%d,%d) is outside memory recipe", offset, offset+length)
		}
		chunk := s.recipe.Chunks[chunkIndex]
		data, downloaded, err := readRecipeChunk(ctx, s.store, s.cache, chunk)
		if err != nil {
			return manager.PageData{}, err
		}
		if downloaded {
			s.downloadedChunks.Add(1)
		}
		if len(data) != chunk.Size {
			return manager.PageData{}, fmt.Errorf("chunk %s has size %d, want %d", chunk.ID, len(data), chunk.Size)
		}
		if chunkOffset >= uint64(len(data)) {
			return manager.PageData{}, fmt.Errorf("page range [%d,%d) is outside memory recipe", offset, offset+length)
		}
		start := chunkOffset
		end := uint64(len(data))
		remaining := uint64(len(result) - written)
		if end-start > remaining {
			end = start + remaining
		}
		written += copy(result[written:], data[start:end])
		chunkIndex++
		chunkOffset = 0
	}
	zero := true
	for _, value := range result {
		if value != 0 {
			zero = false
			break
		}
	}
	return manager.PageData{Bytes: result, Zero: zero}, nil
}

func (s *recipePageSource) Close() error {
	s.closed = true
	return nil
}

// DownloadedChunkCount reports successful remote chunk downloads made while
// this source served page reads. Cache hits do not increase the count.
func (s *recipePageSource) DownloadedChunkCount() uint64 {
	return s.downloadedChunks.Load()
}

var _ manager.PageSource = (*recipePageSource)(nil)
var _ manager.ChunkDownloadCounter = (*recipePageSource)(nil)
