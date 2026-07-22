package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/vhive-serverless/vhive/memory/manager"
)

func TestRestoreMaterializerLazyRecipePages(t *testing.T) {
	store := NewMemoryArtifactStore()
	ctx := context.Background()
	data := []byte{1, 2, 3, 4, 0, 0, 0, 0, 9, 10}
	recipe, err := SplitMemory(bytes.NewReader(data), 4, func(id ChunkID, chunk []byte) error {
		return putChunkIfAbsent(ctx, store, id, chunk)
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewRecipePageSource(store, nil, recipe)
	if err != nil {
		t.Fatal(err)
	}
	input, err := (manager.RestoreMaterializer{}).MaterializeLazy(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	page, err := input.PageSource.ReadAt(ctx, 2, 6)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := page.Bytes, []byte{3, 4, 0, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("page = %v, want %v", got, want)
	}
	if page.Zero {
		t.Fatal("mixed page reported as zero")
	}
	zero, err := input.PageSource.ReadAt(ctx, 4, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !zero.Zero {
		t.Fatal("zero page was not identified")
	}
}

func TestRestoreMaterializerLazyRecipeMissingChunk(t *testing.T) {
	store := NewMemoryArtifactStore()
	recipe := MemoryRecipe{Version: 1, ChunkSize: 4, Chunks: []RecipeChunk{{ID: ChunkID("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), Size: 4}}}
	source, err := NewRecipePageSource(store, nil, recipe)
	if err != nil {
		t.Fatal(err)
	}
	input, err := (manager.RestoreMaterializer{}).MaterializeLazy(source)
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	_, err = input.PageSource.ReadAt(context.Background(), 0, 4)
	if err == nil || !errors.Is(err, ErrArtifactNotFound) {
		t.Fatalf("ReadAt error = %v, want missing chunk", err)
	}
}

func TestRestoreMaterializerEagerRecipe(t *testing.T) {
	store := NewMemoryArtifactStore()
	ctx := context.Background()
	data := []byte("recipe-backed eager restore")
	recipe, err := SplitMemory(bytes.NewReader(data), 8, func(id ChunkID, chunk []byte) error { return putChunkIfAbsent(ctx, store, id, chunk) })
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "mem")
	input, err := (manager.RestoreMaterializer{}).MaterializeEager(path, func(writer io.Writer) error {
		return ReconstructMemoryWithCache(ctx, store, nil, recipe, writer)
	})
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	if input.PageSource != nil {
		t.Fatal("eager input unexpectedly has a page source")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("memory = %q, want %q", got, data)
	}
}

func TestRestoreInputAdaptsToUFFDPageServer(t *testing.T) {
	store := NewMemoryArtifactStore()
	ctx := context.Background()
	data := []byte{7, 8, 9, 10}
	recipe, err := SplitMemory(bytes.NewReader(data), 4, func(id ChunkID, chunk []byte) error {
		return putChunkIfAbsent(ctx, store, id, chunk)
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := NewRecipePageSource(store, nil, recipe)
	if err != nil {
		t.Fatal(err)
	}
	input, err := (manager.RestoreMaterializer{}).MaterializeLazy(source)
	if err != nil {
		t.Fatal(err)
	}
	server, err := manager.NewPageServer(input.PageSource)
	if err != nil {
		t.Fatal(err)
	}
	page, err := server.Read(0, 4)
	if err != nil || !bytes.Equal(page.Bytes, data) {
		t.Fatalf("server page = %v, err = %v", page.Bytes, err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
}
