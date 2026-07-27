package snapshotting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// memoryRecipeArtifact is revision-scoped; its chunks are immutable shared
// objects. Current writers use SHA-256 IDs, but readers do not treat an ID as
// a checksum because future storage encodings may use a different identity.
const memoryRecipeArtifact = ".memory-recipe.json"

type ChunkID string

type RecipeChunk struct {
	ID   ChunkID `json:"id"`
	Size int     `json:"size"`
}

// inMemoryChunkHandle is implemented only by the ephemeral in-memory cache.
// Its bytes remain valid after Release because that cache has no eviction and
// belongs to the lifetime of a single reconstruction or page source.
type inMemoryChunkHandle interface {
	cachedBytes() ([]byte, error)
}

// MemoryRecipe preserves the exact order and length of a chunked memory file.
// The final chunk may be shorter than ChunkSize; empty input has no chunks.
type MemoryRecipe struct {
	Version   int           `json:"version"`
	ChunkSize int           `json:"chunkSize"`
	Chunks    []RecipeChunk `json:"chunks"`
}

func chunkID(data []byte) ChunkID {
	sum := sha256.Sum256(data)
	return ChunkID(hex.EncodeToString(sum[:]))
}

func validChunkID(id ChunkID) bool {
	if len(id) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(string(id))
	return err == nil
}

// SplitMemory emits fixed-size chunks in reader order. fn is called before
// the chunk buffer is reused, so callers must copy data they retain.
func SplitMemory(reader io.Reader, chunkSize int, fn func(ChunkID, []byte) error) (MemoryRecipe, error) {
	if chunkSize <= 0 {
		return MemoryRecipe{}, fmt.Errorf("chunk size must be positive")
	}
	if fn == nil {
		return MemoryRecipe{}, fmt.Errorf("chunk callback is required")
	}
	recipe := MemoryRecipe{Version: 1, ChunkSize: chunkSize}
	buf := make([]byte, chunkSize)
	for {
		n, err := io.ReadFull(reader, buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			id := chunkID(chunk)
			recipe.Chunks = append(recipe.Chunks, RecipeChunk{ID: id, Size: n})
			if callbackErr := fn(id, chunk); callbackErr != nil {
				return MemoryRecipe{}, callbackErr
			}
		}
		if err == nil {
			continue
		}
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return recipe, nil
		}
		return MemoryRecipe{}, fmt.Errorf("read memory chunk: %w", err)
	}
}

func (r MemoryRecipe) Validate() error {
	if r.Version != 1 || r.ChunkSize <= 0 {
		return fmt.Errorf("invalid memory recipe header")
	}
	for index, chunk := range r.Chunks {
		if !validChunkID(chunk.ID) || chunk.Size <= 0 || chunk.Size > r.ChunkSize || (index < len(r.Chunks)-1 && chunk.Size != r.ChunkSize) {
			return fmt.Errorf("invalid memory recipe chunk %d", index)
		}
	}
	return nil
}

// ChunkRepository provides atomic content-addressed insertion. ArtifactStore
// implementations that do not offer it still work through a Stat/Put fallback.
type ChunkRepository interface {
	PutIfAbsent(ctx context.Context, id ChunkID, data []byte) error
	GetChunk(ctx context.Context, id ChunkID) (io.ReadCloser, error)
}

func chunkArtifactKey(id ChunkID) (ArtifactKey, error) {
	return SharedArtifactKey("chunks", string(id))
}

func putChunkIfAbsent(ctx context.Context, store ArtifactStore, id ChunkID, data []byte) error {
	if !validChunkID(id) {
		return fmt.Errorf("invalid chunk %q", id)
	}
	if repository, ok := store.(ChunkRepository); ok {
		return repository.PutIfAbsent(ctx, id, data)
	}
	key, err := chunkArtifactKey(id)
	if err != nil {
		return err
	}
	if _, err = store.Stat(ctx, key); err == nil {
		return nil
	} else if !isArtifactNotFound(err) {
		return fmt.Errorf("stat chunk %s: %w", id, err)
	}
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return fmt.Errorf("upload chunk %s: %w", id, err)
	}
	return nil
}

func getChunk(ctx context.Context, store ArtifactStore, id ChunkID) (io.ReadCloser, error) {
	if !validChunkID(id) {
		return nil, fmt.Errorf("invalid chunk id %q", id)
	}
	if repository, ok := store.(ChunkRepository); ok {
		return repository.GetChunk(ctx, id)
	}
	key, err := chunkArtifactKey(id)
	if err != nil {
		return nil, err
	}
	reader, err := store.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("download chunk %s: %w", id, err)
	}
	return reader, nil
}

func uploadChunkedMemory(ctx context.Context, store ArtifactStore, filename string, chunkSize int) (MemoryRecipe, error) {
	file, err := os.Open(filename)
	if err != nil {
		return MemoryRecipe{}, fmt.Errorf("open memory file: %w", err)
	}
	defer file.Close()
	return SplitMemory(file, chunkSize, func(id ChunkID, data []byte) error { return putChunkIfAbsent(ctx, store, id, data) })
}

func putRecipe(ctx context.Context, store ArtifactStore, revision string, recipe MemoryRecipe) error {
	if err := recipe.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(recipe)
	if err != nil {
		return fmt.Errorf("encode memory recipe: %w", err)
	}
	key, err := RevisionArtifactKey(revision, memoryRecipeArtifact)
	if err != nil {
		return err
	}
	if err := store.Put(ctx, key, bytes.NewReader(data), int64(len(data))); err != nil {
		return fmt.Errorf("upload memory recipe: %w", err)
	}
	return nil
}

func getRecipe(ctx context.Context, store ArtifactStore, revision string) (MemoryRecipe, error) {
	key, err := RevisionArtifactKey(revision, memoryRecipeArtifact)
	if err != nil {
		return MemoryRecipe{}, err
	}
	reader, err := store.Get(ctx, key)
	if err != nil {
		return MemoryRecipe{}, fmt.Errorf("download memory recipe: %w", err)
	}
	defer reader.Close()
	var recipe MemoryRecipe
	if err := json.NewDecoder(reader).Decode(&recipe); err != nil {
		return MemoryRecipe{}, fmt.Errorf("decode memory recipe: %w", err)
	}
	if err := recipe.Validate(); err != nil {
		return MemoryRecipe{}, err
	}
	return recipe, nil
}

// ReconstructMemory writes a recipe eagerly.
func ReconstructMemory(ctx context.Context, store ArtifactStore, recipe MemoryRecipe, writer io.Writer) error {
	return ReconstructMemoryWithCache(ctx, store, newMemoryChunkCache(), recipe, writer)
}

// ReconstructMemoryWithCache writes a recipe eagerly, using cache when it is
// provided. Chunk lengths are checked against the recipe before insertion and
// again before they are written to the reconstructed memory file.
func ReconstructMemoryWithCache(ctx context.Context, store ArtifactStore, cache ChunkCache, recipe MemoryRecipe, writer io.Writer) error {
	if err := recipe.Validate(); err != nil {
		return err
	}
	for _, expected := range recipe.Chunks {
		data, _, err := readRecipeChunk(ctx, store, cache, expected)
		if err != nil {
			return err
		}
		if len(data) != expected.Size {
			return fmt.Errorf("chunk %s has size %d, want %d", expected.ID, len(data), expected.Size)
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("write chunk %s: %w", expected.ID, err)
		}
	}
	return nil
}

// readRecipeChunk returns whether it fetched the chunk from remote storage.
func readRecipeChunk(ctx context.Context, store ArtifactStore, cache ChunkCache, expected RecipeChunk) ([]byte, bool, error) {
	if cache == nil {
		data, err := readRemoteChunk(ctx, store, expected.ID)
		return data, err == nil, err
	}
	handle, err := cache.Acquire(ctx, expected.ID)
	downloaded := false
	if errors.Is(err, ErrChunkCacheMiss) {
		data, fetchErr := readRemoteChunk(ctx, store, expected.ID)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		if len(data) != expected.Size {
			return nil, false, fmt.Errorf("chunk %s has size %d, want %d", expected.ID, len(data), expected.Size)
		}
		handle, err = cache.Insert(ctx, expected.ID, data)
		downloaded = err == nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("acquire cached chunk %s: %w", expected.ID, err)
	}
	defer handle.Release()
	if inMemory, ok := handle.(inMemoryChunkHandle); ok {
		data, err := inMemory.cachedBytes()
		if err != nil {
			return nil, false, err
		}
		return data, downloaded, nil
	}

	// File-backed caches require a read into a page buffer. In-memory caches
	// return their immutable entry above, without copying it for every fault.
	reader, err := handle.Open()
	if err != nil {
		return nil, false, err
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, false, fmt.Errorf("read cached chunk %s: %w", expected.ID, readErr)
	}
	if closeErr != nil {
		return nil, false, fmt.Errorf("close cached chunk %s: %w", expected.ID, closeErr)
	}
	return data, downloaded, nil
}

func readRemoteChunk(ctx context.Context, store ArtifactStore, id ChunkID) ([]byte, error) {
	reader, err := getChunk(ctx, store, id)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read chunk %s: %w", id, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close chunk %s: %w", id, closeErr)
	}
	return data, nil
}
