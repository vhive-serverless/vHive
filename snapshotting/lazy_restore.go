package snapshotting

import (
	"context"
	"fmt"

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
	store  ArtifactStore
	cache  ChunkCache
	recipe MemoryRecipe
	closed bool
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
	var cursor uint64
	written := 0
	for _, chunk := range s.recipe.Chunks {
		chunkEnd := cursor + uint64(chunk.Size)
		if offset >= chunkEnd {
			cursor = chunkEnd
			continue
		}
		if offset+length <= cursor {
			break
		}
		data, err := readRecipeChunk(ctx, s.store, s.cache, chunk)
		if err != nil {
			return manager.PageData{}, err
		}
		if len(data) != chunk.Size || chunkID(data) != chunk.ID {
			return manager.PageData{}, fmt.Errorf("corrupt chunk %s", chunk.ID)
		}
		start := uint64(0)
		if offset > cursor {
			start = offset - cursor
		}
		end := uint64(len(data))
		if offset+length < chunkEnd {
			end = offset + length - cursor
		}
		written += copy(result[written:], data[start:end])
		cursor = chunkEnd
		if written == len(result) {
			break
		}
	}
	if written != len(result) {
		return manager.PageData{}, fmt.Errorf("page range [%d,%d) is outside memory recipe", offset, offset+length)
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

var _ manager.PageSource = (*recipePageSource)(nil)
