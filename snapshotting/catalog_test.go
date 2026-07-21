package snapshotting

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLocalCatalogLifecycle(t *testing.T) {
	catalog, err := NewLocalCatalog(t.TempDir())
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}

	if _, err := catalog.Get("missing"); !errors.Is(err, ErrSnapshotNotFound) {
		t.Fatalf("Get(missing) error = %v, want ErrSnapshotNotFound", err)
	}

	descriptor, err := catalog.Begin("revision-1", "example-image")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if descriptor.Ready || descriptor.Artifacts != defaultArtifactNames() {
		t.Fatalf("Begin descriptor = %#v, want creating descriptor with default artifact names", descriptor)
	}
	if _, err := catalog.Get(descriptor.Revision); !errors.Is(err, ErrSnapshotNotReady) {
		t.Fatalf("Get(creating) error = %v, want ErrSnapshotNotReady", err)
	}
	if _, err := catalog.Begin(descriptor.Revision, descriptor.Image); !errors.Is(err, ErrSnapshotExists) {
		t.Fatalf("duplicate Begin error = %v, want ErrSnapshotExists", err)
	}

	if err := catalog.Commit(descriptor.Revision); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	committed, err := catalog.Get(descriptor.Revision)
	if err != nil {
		t.Fatalf("Get(committed): %v", err)
	}
	if !committed.Ready || committed.Image != descriptor.Image {
		t.Fatalf("committed descriptor = %#v", committed)
	}
	if err := catalog.Commit(descriptor.Revision); err == nil {
		t.Fatal("second Commit unexpectedly succeeded")
	}

	if err := catalog.Delete(descriptor.Revision); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, err := catalog.Exists(descriptor.Revision)
	if err != nil || exists {
		t.Fatalf("Exists after Delete = (%t, %v), want (false, nil)", exists, err)
	}
}

func TestLocalCatalogRejectsInterruptedCreationAfterRestart(t *testing.T) {
	baseFolder := t.TempDir()
	catalog, err := NewLocalCatalog(baseFolder)
	if err != nil {
		t.Fatalf("create catalog: %v", err)
	}
	if _, err := catalog.Begin("interrupted", "example-image"); err != nil {
		t.Fatalf("Begin: %v", err)
	}

	// A new catalog instance models a coordinator restart. The durable creating
	// descriptor must still not be returned as a usable snapshot.
	restarted, err := NewLocalCatalog(filepath.Clean(baseFolder))
	if err != nil {
		t.Fatalf("restart catalog: %v", err)
	}
	if _, err := restarted.Get("interrupted"); !errors.Is(err, ErrSnapshotNotReady) {
		t.Fatalf("Get(interrupted) error = %v, want ErrSnapshotNotReady", err)
	}
}
