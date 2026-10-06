package uffd_handler

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestUnconnectedUffdCancellationReleases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var released atomic.Int32
	ready, done := make(chan error, 1), make(chan error, 1)
	go func() {
		done <- StartUffdHandlerContext(ctx, filepath.Join(t.TempDir(), "vm.sock"), nil, "", nil, nil, nil, false, nil, 1, func() { released.Add(1) }, ready)
	}()
	if err := <-ready; err != nil {
		t.Fatal(err)
	}
	// Simulate CreateVM failing without ever connecting to its UFFD listener.
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || released.Load() != 1 {
			t.Fatalf("error=%v release count=%d", err, released.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listener did not exit and release failed restore buffers")
	}
}

func TestAcceptedUffdOutlivesRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addr := &net.UnixAddr{Name: filepath.Join(t.TempDir(), "vm.sock"), Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan *net.UnixConn, 1)
	go func() {
		conn, _ := acceptUffdConnection(ctx, listener)
		accepted <- conn
	}()
	peer, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	conn := <-accepted
	if conn == nil {
		t.Fatal("accept failed")
	}
	defer conn.Close()
	cancel()
	peer.SetWriteDeadline(time.Now().Add(time.Second))
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := peer.Write([]byte{42}); err != nil {
		t.Fatal(err)
	}
	var b [1]byte
	if _, err := conn.Read(b[:]); err != nil || b[0] != 42 {
		t.Fatalf("request cancellation closed the VM-owned connection: %v", err)
	}
}
