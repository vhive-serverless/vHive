package snapshotting

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const descriptorFileName = ".snapshot-descriptor.json"

var (
	ErrSnapshotNotFound = errors.New("snapshot does not exist")
	ErrSnapshotNotReady = errors.New("snapshot is not yet usable")
	ErrSnapshotExists   = errors.New("snapshot already exists")
)

// ArtifactNames are the logical names of a snapshot's local artifacts.  The
// catalog deliberately records names rather than paths, so later storage
// implementations need not expose the local filesystem layout.
type ArtifactNames struct {
	VMState string `json:"vmState"`
	Memory  string `json:"memory"`
	Patch   string `json:"patch"`
	Info    string `json:"info"`
}

func defaultArtifactNames() ArtifactNames {
	return ArtifactNames{
		VMState: "snap_file",
		Memory:  "mem_file",
		Patch:   "patch_file",
		Info:    "info_file",
	}
}

// SnapshotDescriptor is the catalog's durable lifecycle record for one
// revision. A descriptor is readable only after Ready is true.
type SnapshotDescriptor struct {
	Revision     string        `json:"revision"`
	Image        string        `json:"image"`
	Ready        bool          `json:"ready"`
	Artifacts    ArtifactNames `json:"artifacts"`
	MemoryRecipe string        `json:"memoryRecipe,omitempty"`
}

// Catalog owns the lifecycle metadata for snapshots. The local implementation
// uses the existing revision directory as its backing store.
type Catalog interface {
	Begin(revision, image string) (*SnapshotDescriptor, error)
	Get(revision string) (*SnapshotDescriptor, error)
	Commit(revision string) error
	Delete(revision string) error
	Exists(revision string) (bool, error)
}

type LocalCatalog struct {
	mu         sync.Mutex
	baseFolder string
}

func NewLocalCatalog(baseFolder string) (*LocalCatalog, error) {
	if err := os.MkdirAll(baseFolder, 0755); err != nil {
		return nil, fmt.Errorf("create snapshot catalog directory: %w", err)
	}
	return &LocalCatalog{baseFolder: baseFolder}, nil
}

func (c *LocalCatalog) Begin(revision, image string) (*SnapshotDescriptor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.read(revision); err == nil {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotExists, revision)
	} else if !errors.Is(err, ErrSnapshotNotFound) {
		return nil, err
	}

	descriptor := &SnapshotDescriptor{Revision: revision, Image: image, Artifacts: defaultArtifactNames()}
	if err := os.MkdirAll(c.snapshotDir(revision), 0755); err != nil {
		return nil, fmt.Errorf("create snapshot directory for %s: %w", revision, err)
	}
	if err := c.write(descriptor); err != nil {
		return nil, err
	}
	return cloneDescriptor(descriptor), nil
}

func (c *LocalCatalog) Get(revision string) (*SnapshotDescriptor, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	descriptor, err := c.read(revision)
	if err != nil {
		return nil, err
	}
	if !descriptor.Ready {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotReady, revision)
	}
	return cloneDescriptor(descriptor), nil
}

func (c *LocalCatalog) Commit(revision string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	descriptor, err := c.read(revision)
	if err != nil {
		return err
	}
	if descriptor.Ready {
		return fmt.Errorf("snapshot %s has already been committed", revision)
	}
	descriptor.Ready = true
	return c.write(descriptor)
}

// SetMemoryRecipe records the optional remote memory recipe before commit so
// a later local acquire can still select recipe-backed lazy restore.
func (c *LocalCatalog) SetMemoryRecipe(revision, recipe string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	descriptor, err := c.read(revision)
	if err != nil {
		return err
	}
	descriptor.MemoryRecipe = recipe
	return c.write(descriptor)
}

func (c *LocalCatalog) Delete(revision string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, err := c.read(revision); err != nil {
		return err
	}
	if err := os.RemoveAll(c.snapshotDir(revision)); err != nil {
		return fmt.Errorf("delete snapshot %s: %w", revision, err)
	}
	return nil
}

func (c *LocalCatalog) Exists(revision string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, err := c.read(revision)
	if errors.Is(err, ErrSnapshotNotFound) {
		return false, nil
	}
	return err == nil, err
}

func (c *LocalCatalog) snapshotDir(revision string) string {
	return filepath.Join(c.baseFolder, revision)
}

func (c *LocalCatalog) descriptorPath(revision string) string {
	return filepath.Join(c.snapshotDir(revision), descriptorFileName)
}

func (c *LocalCatalog) read(revision string) (*SnapshotDescriptor, error) {
	file, err := os.Open(c.descriptorPath(revision))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotFound, revision)
	}
	if err != nil {
		return nil, fmt.Errorf("read snapshot descriptor for %s: %w", revision, err)
	}
	defer file.Close()

	var descriptor SnapshotDescriptor
	if err := json.NewDecoder(file).Decode(&descriptor); err != nil {
		return nil, fmt.Errorf("decode snapshot descriptor for %s: %w", revision, err)
	}
	if descriptor.Revision != revision {
		return nil, fmt.Errorf("snapshot descriptor revision mismatch: got %q, want %q", descriptor.Revision, revision)
	}
	return &descriptor, nil
}

func (c *LocalCatalog) write(descriptor *SnapshotDescriptor) error {
	path := c.descriptorPath(descriptor.Revision)
	temporary, err := os.CreateTemp(filepath.Dir(path), descriptorFileName+"-*")
	if err != nil {
		return fmt.Errorf("create snapshot descriptor for %s: %w", descriptor.Revision, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := json.NewEncoder(temporary).Encode(descriptor); err != nil {
		temporary.Close()
		return fmt.Errorf("encode snapshot descriptor for %s: %w", descriptor.Revision, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snapshot descriptor for %s: %w", descriptor.Revision, err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("publish snapshot descriptor for %s: %w", descriptor.Revision, err)
	}
	return nil
}

func cloneDescriptor(descriptor *SnapshotDescriptor) *SnapshotDescriptor {
	copy := *descriptor
	return &copy
}
