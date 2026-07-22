package manager

import (
	"context"
	"errors"
	"testing"
)

type testPageSource struct {
	data   []byte
	closed bool
	err    error
}

func (s *testPageSource) ReadAt(_ context.Context, offset, length uint64) (PageData, error) {
	if s.err != nil {
		return PageData{}, s.err
	}
	if offset+length > uint64(len(s.data)) {
		return PageData{}, errors.New("outside source")
	}
	page := append([]byte(nil), s.data[offset:offset+length]...)
	zero := true
	for _, b := range page {
		if b != 0 {
			zero = false
		}
	}
	return PageData{Bytes: page, Zero: zero}, nil
}
func (s *testPageSource) Close() error { s.closed = true; return nil }

func TestPageServerPageHitZeroAndClose(t *testing.T) {
	source := &testPageSource{data: []byte{1, 2, 0, 0}}
	server, err := NewPageServer(source)
	if err != nil {
		t.Fatal(err)
	}
	page, err := server.Read(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if page.Zero || string(page.Bytes) != "\x01\x02" {
		t.Fatalf("page = %+v", page)
	}
	zero, err := server.Read(2, 2)
	if err != nil || !zero.Zero {
		t.Fatalf("zero page = %+v, err %v", zero, err)
	}
	if err := server.Close(); err != nil || !source.closed {
		t.Fatalf("close err = %v, source closed = %v", err, source.closed)
	}
	if _, err := server.Read(0, 2); err == nil {
		t.Fatal("Read after Close succeeded")
	}
}

func TestPageServerPropagatesMissingPage(t *testing.T) {
	missing := errors.New("chunk missing")
	server, err := NewPageServer(&testPageSource{err: missing})
	if err != nil {
		t.Fatal(err)
	}
	_, err = server.Read(0, 4096)
	if !errors.Is(err, missing) {
		t.Fatalf("Read error = %v, want %v", err, missing)
	}
}
