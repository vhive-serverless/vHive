package snapshotting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// memoryRecipeArtifact is revision-scoped; its chunks are immutable shared
// objects addressed by the SHA-256 of their plaintext bytes.
const memoryRecipeArtifact = ".memory-recipe.json"

type ChunkID string

type RecipeChunk struct {
	ID   ChunkID `json:"id"`
	Size int     `json:"size"`
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
	if !validChunkID(id) || chunkID(data) != id {
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

// ReconstructMemory writes a recipe eagerly and verifies every fetched chunk.
func ReconstructMemory(ctx context.Context, store ArtifactStore, recipe MemoryRecipe, writer io.Writer) error {
	if err := recipe.Validate(); err != nil {
		return err
	}
	for _, expected := range recipe.Chunks {
		reader, err := getChunk(ctx, store, expected.ID)
		if err != nil {
			return err
		}
		data, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return fmt.Errorf("read chunk %s: %w", expected.ID, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close chunk %s: %w", expected.ID, closeErr)
		}
		if len(data) != expected.Size || chunkID(data) != expected.ID {
			return fmt.Errorf("corrupt chunk %s", expected.ID)
		}
		if _, err := writer.Write(data); err != nil {
			return fmt.Errorf("write chunk %s: %w", expected.ID, err)
		}
	}
	return nil
}
