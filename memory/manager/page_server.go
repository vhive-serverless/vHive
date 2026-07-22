package manager

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// PageData is an immutable page returned to the UFFD handler. Zero lets a
// source communicate that the entire range can be installed as a zero page.
type PageData struct {
	Bytes []byte
	Zero  bool
}

// PageSource hides where lazy memory comes from (a recipe, a cache, or a
// local file) from the UFFD protocol implementation.
type PageSource interface {
	ReadAt(context.Context, uint64, uint64) (PageData, error)
	Close() error
}

// PageServer serializes shutdown with page lookups. The UFFD loop calls Read;
// closing it waits for an in-flight lookup before releasing source handles.
// This prevents a remove/shutdown event from invalidating a page buffer while
// it is being copied into Firecracker's userfaultfd.
type PageServer struct {
	mu     sync.RWMutex
	source PageSource
	closed bool
}

func NewPageServer(source PageSource) (*PageServer, error) {
	if source == nil {
		return nil, fmt.Errorf("page source is required")
	}
	return &PageServer{source: source}, nil
}

func (s *PageServer) Read(offset, length uint64) (PageData, error) {
	if length == 0 {
		return PageData{}, fmt.Errorf("page length must be non-zero")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return PageData{}, errors.New("page server is closed")
	}
	page, err := s.source.ReadAt(context.Background(), offset, length)
	if err != nil {
		return PageData{}, err
	}
	if uint64(len(page.Bytes)) != length {
		return PageData{}, fmt.Errorf("page source returned %d bytes, want %d", len(page.Bytes), length)
	}
	return page, nil
}

func (s *PageServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.source.Close()
}
