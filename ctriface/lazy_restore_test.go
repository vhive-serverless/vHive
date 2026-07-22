package ctriface

import (
	"context"
	"testing"

	"github.com/vhive-serverless/vhive/memory/manager"
	"github.com/vhive-serverless/vhive/snapshotting"
)

type lazyRestoreTestSource struct{ closed bool }

func (s *lazyRestoreTestSource) ReadAt(_ context.Context, _ uint64, length uint64) (manager.PageData, error) {
	return manager.PageData{Bytes: make([]byte, length), Zero: true}, nil
}

func (s *lazyRestoreTestSource) Close() error { s.closed = true; return nil }

func TestLazyRecipePageServerSelection(t *testing.T) {
	snap := snapshotting.NewSnapshotFromDescriptor(t.TempDir(), &snapshotting.SnapshotDescriptor{
		Revision:     "recipe-revision",
		Image:        "test-image",
		Ready:        true,
		MemoryRecipe: ".memory-recipe.json",
	})
	store := snapshotting.NewMemoryArtifactStore()

	original := newRecipePageSourceForRevision
	t.Cleanup(func() { newRecipePageSourceForRevision = original })
	called := false
	source := &lazyRestoreTestSource{}
	newRecipePageSourceForRevision = func(_ context.Context, gotStore snapshotting.ArtifactStore, _ snapshotting.ChunkCache, revision string) (manager.PageSource, error) {
		called = true
		if gotStore != store || revision != snap.GetId() {
			t.Fatalf("recipe source requested store=%p revision=%q", gotStore, revision)
		}
		return source, nil
	}

	server, err := lazyRecipePageServer(context.Background(), true, store, snap)
	if err != nil {
		t.Fatal(err)
	}
	if !called || server == nil {
		t.Fatal("eligible lazy recipe restore did not create a page server")
	}
	page, err := server.Read(0, 4096)
	if err != nil || !page.Zero {
		t.Fatalf("page server read = %+v, error = %v", page, err)
	}
	if err := server.Close(); err != nil || !source.closed {
		t.Fatalf("page server close = %v, source closed = %v", err, source.closed)
	}
}

func TestLazyRecipePageServerLeavesNonRecipeRestoreFileBacked(t *testing.T) {
	snap := snapshotting.NewSnapshot("local-revision", t.TempDir(), "test-image")
	server, err := lazyRecipePageServer(context.Background(), true, snapshotting.NewMemoryArtifactStore(), snap)
	if err != nil {
		t.Fatal(err)
	}
	if server != nil {
		t.Fatal("non-recipe snapshot unexpectedly selected lazy page server")
	}
}
