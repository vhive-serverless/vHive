package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrArtifactNotFound is returned when an artifact key does not exist.
var ErrArtifactNotFound = errors.New("artifact does not exist")

// ArtifactKey is the typed namespace used by remote snapshot artifacts.
// Keys are deliberately logical names; a store decides how they map to its
// underlying object names.
type ArtifactKey string

// ArtifactInfo is the metadata available without downloading an artifact.
type ArtifactInfo struct {
	Key     ArtifactKey
	Size    int64
	ModTime time.Time
}

// ArtifactStore is the storage boundary for snapshot artifacts. Callers own
// the reader supplied to Put and must close the reader returned by Get.
type ArtifactStore interface {
	Put(ctx context.Context, key ArtifactKey, reader io.Reader, size int64) error
	Get(ctx context.Context, key ArtifactKey) (io.ReadCloser, error)
	Stat(ctx context.Context, key ArtifactKey) (ArtifactInfo, error)
	List(ctx context.Context, prefix ArtifactKey) ([]ArtifactInfo, error)
}

// RevisionArtifactKey returns the key for an artifact private to one snapshot
// revision, such as its VM state, memory, patch, or descriptor.
func RevisionArtifactKey(revision, artifact string) (ArtifactKey, error) {
	if err := validateKeyPart("revision", revision); err != nil {
		return "", err
	}
	if err := validateKeyPart("artifact", artifact); err != nil {
		return "", err
	}
	return ArtifactKey(path.Join("revisions", revision, artifact)), nil
}

// SharedArtifactKey returns the key for immutable content that may later be
// shared across revisions. Stage 2 only establishes this namespace.
func SharedArtifactKey(kind, identity string) (ArtifactKey, error) {
	if err := validateKeyPart("shared artifact kind", kind); err != nil {
		return "", err
	}
	if err := validateKeyPart("shared artifact identity", identity); err != nil {
		return "", err
	}
	return ArtifactKey(path.Join("shared", kind, identity)), nil
}

func validateKeyPart(name, value string) error {
	if value == "" || value == "." || value == ".." || strings.Contains(value, "/") || strings.Contains(value, "\\") {
		return fmt.Errorf("invalid %s %q", name, value)
	}
	return nil
}

// ArtifactStoreFailures provides deterministic error injection for tests.
type ArtifactStoreFailures struct {
	Put  error
	Get  error
	Stat error
	List error
}

// MemoryArtifactStore is a byte-preserving ArtifactStore implementation for
// unit and integration tests. It is safe for concurrent use.
type MemoryArtifactStore struct {
	mu       sync.RWMutex
	objects  map[ArtifactKey]memoryArtifact
	failures ArtifactStoreFailures
}

type memoryArtifact struct {
	data    []byte
	modTime time.Time
}

func NewMemoryArtifactStore() *MemoryArtifactStore {
	return &MemoryArtifactStore{objects: make(map[ArtifactKey]memoryArtifact)}
}

func (s *MemoryArtifactStore) SetFailures(failures ArtifactStoreFailures) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failures = failures
}

func (s *MemoryArtifactStore) Put(ctx context.Context, key ArtifactKey, reader io.Reader, size int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	err := s.failures.Put
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read artifact %q: %w", key, err)
	}
	if size >= 0 && int64(len(data)) != size {
		return fmt.Errorf("artifact %q size mismatch: got %d, want %d", key, len(data), size)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = memoryArtifact{data: append([]byte(nil), data...), modTime: time.Now().UTC()}
	return nil
}

// PutIfAbsent atomically stores a chunk only when it has not already been
// published. Equal IDs always represent equal bytes.
func (s *MemoryArtifactStore) PutIfAbsent(ctx context.Context, id ChunkID, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validChunkID(id) || chunkID(data) != id {
		return fmt.Errorf("invalid chunk %q", id)
	}
	key, err := chunkArtifactKey(id)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failures.Put != nil {
		return s.failures.Put
	}
	if existing, ok := s.objects[key]; ok {
		if !bytes.Equal(existing.data, data) {
			return fmt.Errorf("chunk collision for %s", id)
		}
		return nil
	}
	s.objects[key] = memoryArtifact{data: append([]byte(nil), data...), modTime: time.Now().UTC()}
	return nil
}

func (s *MemoryArtifactStore) GetChunk(ctx context.Context, id ChunkID) (io.ReadCloser, error) {
	key, err := chunkArtifactKey(id)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx, key)
}

func (s *MemoryArtifactStore) Get(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.failures.Get != nil {
		return nil, s.failures.Get
	}
	artifact, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrArtifactNotFound, key)
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), artifact.data...))), nil
}

func (s *MemoryArtifactStore) Stat(ctx context.Context, key ArtifactKey) (ArtifactInfo, error) {
	if err := ctx.Err(); err != nil {
		return ArtifactInfo{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.failures.Stat != nil {
		return ArtifactInfo{}, s.failures.Stat
	}
	artifact, ok := s.objects[key]
	if !ok {
		return ArtifactInfo{}, fmt.Errorf("%w: %s", ErrArtifactNotFound, key)
	}
	return ArtifactInfo{Key: key, Size: int64(len(artifact.data)), ModTime: artifact.modTime}, nil
}

func (s *MemoryArtifactStore) List(ctx context.Context, prefix ArtifactKey) ([]ArtifactInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.failures.List != nil {
		return nil, s.failures.List
	}
	keys := make([]string, 0, len(s.objects))
	for key := range s.objects {
		if strings.HasPrefix(string(key), string(prefix)) {
			keys = append(keys, string(key))
		}
	}
	sort.Strings(keys)
	info := make([]ArtifactInfo, 0, len(keys))
	for _, rawKey := range keys {
		key := ArtifactKey(rawKey)
		artifact := s.objects[key]
		info = append(info, ArtifactInfo{Key: key, Size: int64(len(artifact.data)), ModTime: artifact.modTime})
	}
	return info, nil
}
