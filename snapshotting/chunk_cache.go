package snapshotting

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// ErrChunkCacheMiss distinguishes a cache miss from an unusable cache.
var ErrChunkCacheMiss = errors.New("chunk is not cached")

// ChunkCache stores chunks locally. Acquire and Insert return a pinned handle:
// callers must Release it once they no longer need the chunk.
// Cleanup removes unpinned entries. File-backed implementations may also
// enforce a capacity while retaining pinned entries.
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
	// pending retains a fetched chunk until its asynchronous write reaches the
	// file cache. It lets concurrent page faults use the downloaded bytes
	// without waiting for disk I/O or fetching the same chunk again.
	pending  map[ChunkID][]byte
	metrics  ChunkCacheMetrics
	capacity int64 // negative means unlimited
	clock    uint64
}

type fileCacheEntry struct {
	size       int64
	pins       int
	lastAccess uint64
}

func NewFileChunkCache(directory string) (*FileChunkCache, error) {
	if directory == "" {
		return nil, fmt.Errorf("chunk cache directory is required")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, fmt.Errorf("create chunk cache directory: %w", err)
	}
	return &FileChunkCache{
		directory: directory,
		entries:   make(map[ChunkID]*fileCacheEntry),
		pending:   make(map[ChunkID][]byte),
		capacity:  -1,
	}, nil
}

// SetCapacity sets the maximum number of bytes retained on disk. A negative
// value disables eviction. Pinned chunks are never removed, so a temporary
// overage is possible until their handles are released.
func (c *FileChunkCache) SetCapacity(capacity int64) error {
	if capacity < -1 {
		return fmt.Errorf("chunk cache capacity must be non-negative or -1")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.capacity = capacity
	if err := c.loadEntriesLocked(); err != nil {
		return err
	}
	return c.evictLocked()
}

// loadEntriesLocked accounts for chunks left by a previous process so a
// capacity is enforced across restarts as well as within one process.
func (c *FileChunkCache) loadEntriesLocked() error {
	items, err := os.ReadDir(c.directory)
	if err != nil {
		return fmt.Errorf("read chunk cache directory: %w", err)
	}
	for _, item := range items {
		id := ChunkID(item.Name())
		if item.IsDir() || !validChunkID(id) || c.entries[id] != nil {
			continue
		}
		info, err := item.Info()
		if err != nil {
			return fmt.Errorf("stat cached chunk %s: %w", id, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		entry := &fileCacheEntry{size: info.Size()}
		c.touchLocked(entry)
		c.entries[id] = entry
		c.metrics.Bytes += entry.size
	}
	return nil
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
		if !errors.Is(err, ErrChunkCacheMiss) {
			return nil, err
		}
		if data := c.pending[id]; data != nil {
			c.metrics.Hits++
			c.metrics.Accesses++
			return &memoryChunkHandle{data: data}, nil
		}
		c.metrics.Misses++
		return nil, err
	}
	entry.pins++
	c.touchLocked(entry)
	c.metrics.Hits++
	c.metrics.Accesses++
	return &fileChunkHandle{cache: c, id: id}, nil
}

// InsertAsync schedules a write-behind insertion. It is used only after a
// chunk was successfully downloaded, so the caller can return those bytes to
// the page-fault path without waiting for local disk I/O. Until the cache file
// is published, Acquire serves the pending bytes directly from memory.
//
// Cache persistence is an optimization: a failed asynchronous write simply
// leaves the chunk uncached for a later download.
func (c *FileChunkCache) InsertAsync(id ChunkID, data []byte) {
	if !validChunkID(id) {
		return
	}
	c.mu.Lock()
	if _, err := c.entryLocked(id); err == nil {
		c.mu.Unlock()
		return
	} else if !errors.Is(err, ErrChunkCacheMiss) {
		c.mu.Unlock()
		return
	}
	if _, ok := c.pending[id]; ok {
		c.mu.Unlock()
		return
	}
	c.pending[id] = data
	c.mu.Unlock()

	go c.persistAsync(id, data)
}

func (c *FileChunkCache) persistAsync(id ChunkID, data []byte) {
	temporary, err := os.CreateTemp(c.directory, ".chunk-*")
	if err != nil {
		c.clearPending(id)
		return
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		c.clearPending(id)
		return
	}
	if err := temporary.Close(); err != nil {
		c.clearPending(id)
		return
	}
	if err := os.Rename(temporaryName, c.filename(id)); err != nil && !os.IsExist(err) {
		c.clearPending(id)
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
	if _, ok := c.entries[id]; !ok {
		entry := &fileCacheEntry{size: int64(len(data))}
		c.touchLocked(entry)
		c.entries[id] = entry
		c.metrics.Bytes += entry.size
	}
	_ = c.evictLocked()
}

func (c *FileChunkCache) clearPending(id ChunkID) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *FileChunkCache) Insert(ctx context.Context, id ChunkID, data []byte) (ChunkHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validChunkID(id) {
		return nil, fmt.Errorf("invalid chunk %q", id)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, err := c.entryLocked(id); err == nil {
		entry.pins++
		c.touchLocked(entry)
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
	c.touchLocked(entry)
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
	c.touchLocked(entry)
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
	return h.cache.evictLocked()
}

func (c *FileChunkCache) touchLocked(entry *fileCacheEntry) {
	c.clock++
	entry.lastAccess = c.clock
}

// evictLocked discards least-recently-used unpinned files until the cache is
// within its configured capacity. c.mu must be held.
func (c *FileChunkCache) evictLocked() error {
	if c.capacity < 0 || c.metrics.Bytes <= c.capacity {
		return nil
	}
	ids := make([]ChunkID, 0, len(c.entries))
	for id, entry := range c.entries {
		if entry.pins == 0 {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return c.entries[ids[i]].lastAccess < c.entries[ids[j]].lastAccess
	})
	for _, id := range ids {
		if c.metrics.Bytes <= c.capacity {
			break
		}
		entry := c.entries[id]
		if err := os.Remove(c.filename(id)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("evict cached chunk %s: %w", id, err)
		}
		delete(c.entries, id)
		c.metrics.Bytes -= entry.size
	}
	return nil
}
