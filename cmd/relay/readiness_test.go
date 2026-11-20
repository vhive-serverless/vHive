package main

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestWaitForEndpoint(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	endpoint := listener.Addr().String()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := waitForEndpoint(ctx, endpoint); err != nil {
		t.Fatal(err)
	}
	listener.Close()
	timeout, stop := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer stop()
	if err := waitForEndpoint(timeout, endpoint); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unavailable endpoint returned %v", err)
	}
}
