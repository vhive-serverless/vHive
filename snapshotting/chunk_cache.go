package snapshotting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// ErrChunkCacheMiss distinguishes a cache miss from an unusable cache.
var ErrChunkCacheMiss = errors.New("chunk is not cached")

// ChunkCache stores verified chunks locally. Acquire and Insert return a
// pinned handle: callers must Release it once they no longer need the chunk.
// Cleanup is deliberately limited to removing unpinned cache entries; Stage 5
// intentionally has no capacity limit or eviction policy.
type ChunkCache interface {
	Acquire(ctx context.Context, id ChunkID) (ChunkHandle, error)
	Insert(ctx context.Context, id ChunkID, data []byte) (ChunkHandle, error)
	Cleanup(ctx context.Context) error
	Metrics() ChunkCacheMetrics
}

type ChunkHandle interface {
	Open() (io.ReadCloser, error)
	Release() error
}

type ChunkCacheMetrics struct {
	Hits     uint64
	Misses   uint64
	Bytes    int64
	Accesses uint64
}

// FileChunkCache is a simple, process-local cache of chunk files. Its mutex
// protects pins and makes cleanup unable to remove a chunk held by a handle.
type FileChunkCache struct {
	directory string

	mu      sync.Mutex
	entries map[ChunkID]*fileCacheEntry
	metrics ChunkCacheMetrics
}

type fileCacheEntry struct {
	size int64
	pins int
}

func NewFileChunkCache(directory string) (*FileChunkCache, error) {
	if directory == "" {
		return nil, fmt.Errorf("chunk cache directory is required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create chunk cache directory: %w", err)
	}
	return &FileChunkCache{directory: directory, entries: make(map[ChunkID]*fileCacheEntry)}, nil
}

func (c *FileChunkCache) Acquire(ctx context.Context, id ChunkID) (ChunkHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validChunkID(id) {
		return nil, fmt.Errorf("invalid chunk id %q", id)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, err := c.entryLocked(id)
	if err != nil {
		if errors.Is(err, ErrChunkCacheMiss) {
			c.metrics.Misses++
		}
		return nil, err
	}
	entry.pins++
	c.metrics.Hits++
	c.metrics.Accesses++
	return &fileChunkHandle{cache: c, id: id}, nil
}

func (c *FileChunkCache) Insert(ctx context.Context, id ChunkID, data []byte) (ChunkHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validChunkID(id) || chunkID(data) != id {
		return nil, fmt.Errorf("invalid chunk %q", id)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, err := c.entryLocked(id); err == nil {
		entry.pins++
		c.metrics.Hits++
		c.metrics.Accesses++
		return &fileChunkHandle{cache: c, id: id}, nil
	} else if !errors.Is(err, ErrChunkCacheMiss) {
		return nil, err
	}
	temporary, err := os.CreateTemp(c.directory, ".chunk-*")
	if err != nil {
		return nil, fmt.Errorf("create cached chunk: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return nil, fmt.Errorf("write cached chunk: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return nil, fmt.Errorf("close cached chunk: %w", err)
	}
	if err := os.Rename(temporaryName, c.filename(id)); err != nil && !os.IsExist(err) {
		return nil, fmt.Errorf("publish cached chunk: %w", err)
	}
	entry := &fileCacheEntry{size: int64(len(data)), pins: 1}
	c.entries[id] = entry
	c.metrics.Bytes += entry.size
	c.metrics.Accesses++
	return &fileChunkHandle{cache: c, id: id}, nil
}

// Cleanup only considers valid chunk files immediately inside the configured
// cache directory. It never recurses or deletes a pinned entry.
func (c *FileChunkCache) Cleanup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entries, err := os.ReadDir(c.directory)
	if err != nil {
		return fmt.Errorf("read chunk cache directory: %w", err)
	}
	for _, item := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		id := ChunkID(item.Name())
		if item.IsDir() || !validChunkID(id) {
			continue
		}
		entry, err := c.entryLocked(id)
		if err != nil {
			return err
		}
		if entry.pins != 0 {
			continue
		}
		if err := os.Remove(c.filename(id)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove cached chunk %s: %w", id, err)
		}
		delete(c.entries, id)
		c.metrics.Bytes -= entry.size
	}
	return nil
}

func (c *FileChunkCache) Metrics() ChunkCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metrics
}

func (c *FileChunkCache) entryLocked(id ChunkID) (*fileCacheEntry, error) {
	if entry := c.entries[id]; entry != nil {
		return entry, nil
	}
	info, err := os.Stat(c.filename(id))
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrChunkCacheMiss, id)
	}
	if err != nil {
		return nil, fmt.Errorf("stat cached chunk %s: %w", id, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("cached chunk %s is not a regular file", id)
	}
	entry := &fileCacheEntry{size: info.Size()}
	c.entries[id] = entry
	c.metrics.Bytes += entry.size
	return entry, nil
}

func (c *FileChunkCache) filename(id ChunkID) string { return filepath.Join(c.directory, string(id)) }

type fileChunkHandle struct {
	cache    *FileChunkCache
	id       ChunkID
	released bool
}

func (h *fileChunkHandle) Open() (io.ReadCloser, error) {
	if h.released {
		return nil, fmt.Errorf("cached chunk handle is released")
	}
	file, err := os.Open(h.cache.filename(h.id))
	if err != nil {
		return nil, fmt.Errorf("open cached chunk %s: %w", h.id, err)
	}
	return file, nil
}

func (h *fileChunkHandle) Release() error {
	if h.released {
		return nil
	}
	h.cache.mu.Lock()
	defer h.cache.mu.Unlock()
	entry := h.cache.entries[h.id]
	if entry == nil || entry.pins == 0 {
		return fmt.Errorf("invalid release for cached chunk %s", h.id)
	}
	entry.pins--
	h.released = true
	return nil
}
