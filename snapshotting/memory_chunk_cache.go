package snapshotting

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"sync"
)

// memoryChunkCache is intentionally short-lived: callers create one for a
// single reconstruction or recipe page source. Unlike FileChunkCache, it has
// no storage lifecycle and is discarded with its owning restore.
type memoryChunkCache struct {
	mu      sync.Mutex
	entries map[ChunkID][]byte
	metrics ChunkCacheMetrics
}

func newMemoryChunkCache() *memoryChunkCache {
	return &memoryChunkCache{entries: make(map[ChunkID][]byte)}
}

func (c *memoryChunkCache) Acquire(ctx context.Context, id ChunkID) (ChunkHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validChunkID(id) {
		return nil, fmt.Errorf("invalid chunk id %q", id)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.entries[id]
	if !ok {
		c.metrics.Misses++
		return nil, fmt.Errorf("%w: %s", ErrChunkCacheMiss, id)
	}
	c.metrics.Hits++
	c.metrics.Accesses++
	return &memoryChunkHandle{data: data}, nil
}

func (c *memoryChunkCache) Insert(ctx context.Context, id ChunkID, data []byte) (ChunkHandle, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validChunkID(id) || chunkID(data) != id {
		return nil, fmt.Errorf("invalid chunk %q", id)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.entries[id]; ok {
		c.metrics.Hits++
		c.metrics.Accesses++
		return &memoryChunkHandle{data: existing}, nil
	}
	copyOfData := append([]byte(nil), data...)
	c.entries[id] = copyOfData
	c.metrics.Bytes += int64(len(copyOfData))
	c.metrics.Accesses++
	return &memoryChunkHandle{data: copyOfData}, nil
}

func (c *memoryChunkCache) Cleanup(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (c *memoryChunkCache) Metrics() ChunkCacheMetrics {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.metrics
}

type memoryChunkHandle struct {
	data     []byte
	released bool
}

func (h *memoryChunkHandle) Open() (io.ReadCloser, error) {
	if h.released {
		return nil, fmt.Errorf("cached chunk handle is released")
	}
	return io.NopCloser(bytes.NewReader(h.data)), nil
}

func (h *memoryChunkHandle) Release() error {
	if h.released {
		return fmt.Errorf("cached chunk handle is released")
	}
	h.released = true
	return nil
}

var _ ChunkCache = (*memoryChunkCache)(nil)
