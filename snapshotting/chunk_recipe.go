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
const (
	memoryRecipeArtifact = ".memory-recipe.json"
	memoryRecipeVersion  = 2
)

type ChunkID string

type RecipeChunk struct {
	ID ChunkID `json:"id"`
}

// inMemoryChunkHandle is implemented only by the ephemeral in-memory cache.
// Its bytes remain valid after Release because that cache has no eviction and
// belongs to the lifetime of a single reconstruction or page source.
type inMemoryChunkHandle interface {
	cachedBytes() ([]byte, error)
}

// asyncChunkCache accepts downloaded chunks for write-behind persistence.
// It is deliberately private: callers of ChunkCache retain the synchronous
// Insert contract, while recipe reads can keep disk writes off their critical
// path when the selected cache supports it.
type asyncChunkCache interface {
	InsertAsync(id ChunkID, data []byte)
}

// MemoryRecipe preserves the order of fixed-size chunks in a memory file.
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
	recipe := MemoryRecipe{Version: memoryRecipeVersion, ChunkSize: chunkSize}
	buf := make([]byte, chunkSize)
	for {
		_, err := io.ReadFull(reader, buf)
		if err == nil {
			chunk := append([]byte(nil), buf...)
			id := chunkID(chunk)
			recipe.Chunks = append(recipe.Chunks, RecipeChunk{ID: id})
			if callbackErr := fn(id, chunk); callbackErr != nil {
				return MemoryRecipe{}, callbackErr
			}
			continue
		}
		if err == io.EOF {
			return recipe, nil
		}
		if err == io.ErrUnexpectedEOF {
			return MemoryRecipe{}, fmt.Errorf("memory size is not divisible by chunk size %d", chunkSize)
		}
		return MemoryRecipe{}, fmt.Errorf("read memory chunk: %w", err)
	}
}

func (r MemoryRecipe) Validate() error {
	if r.Version != memoryRecipeVersion || r.ChunkSize <= 0 {
		return fmt.Errorf("invalid memory recipe header")
	}
	for index, chunk := range r.Chunks {
		if !validChunkID(chunk.ID) {
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
		data, _, err := readRecipeChunk(ctx, store, cache, expected.ID, recipe.ChunkSize)
		if err != nil {
			return err
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("write chunk %s: %w", expected.ID, err)
		}
	}
	return nil
}

// readRecipeChunk returns whether it fetched the chunk from remote storage.
func readRecipeChunk(ctx context.Context, store ArtifactStore, cache ChunkCache, id ChunkID, chunkSize int) ([]byte, bool, error) {
	if cache == nil {
		data, err := readRemoteChunk(ctx, store, id, chunkSize)
		return data, err == nil, err
	}
	handle, err := cache.Acquire(ctx, id)
	downloaded := false
	if errors.Is(err, ErrChunkCacheMiss) {
		data, fetchErr := readRemoteChunk(ctx, store, id, chunkSize)
		if fetchErr != nil {
			return nil, false, fetchErr
		}
		if async, ok := cache.(asyncChunkCache); ok {
			async.InsertAsync(id, data)
			return data, true, nil
		}
		handle, err = cache.Insert(ctx, id, data)
		downloaded = err == nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("acquire cached chunk %s: %w", id, err)
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
	data, readErr := readExactChunk(reader, id, chunkSize)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, false, fmt.Errorf("read cached chunk %s: %w", id, readErr)
	}
	if closeErr != nil {
		return nil, false, fmt.Errorf("close cached chunk %s: %w", id, closeErr)
	}
	return data, downloaded, nil
}

func readRemoteChunk(ctx context.Context, store ArtifactStore, id ChunkID, chunkSize int) ([]byte, error) {
	reader, err := getChunk(ctx, store, id)
	if err != nil {
		return nil, err
	}
	data, readErr := readExactChunk(reader, id, chunkSize)
	closeErr := reader.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close chunk %s: %w", id, closeErr)
	}
	return data, nil
}

// readExactChunk allocates the recipe-declared size once and rejects remote
// objects or cache entries that are shorter or longer than that declaration.
func readExactChunk(reader io.Reader, id ChunkID, chunkSize int) ([]byte, error) {
	data := make([]byte, chunkSize)
	if _, err := io.ReadFull(reader, data); err != nil {
		return nil, fmt.Errorf("read chunk %s: corrupt size: %w", id, err)
	}
	var extra [1]byte
	if n, err := reader.Read(extra[:]); n != 0 {
		return nil, fmt.Errorf("read chunk %s: corrupt size: got more than %d bytes", id, chunkSize)
	} else if err != io.EOF {
		return nil, fmt.Errorf("read chunk %s: check size: %w", id, err)
	}
	return data, nil
}
