package main

import (
	"context"
	"net"
	"time"
)

// waitForEndpoint allows lazy-loaded runtimes to finish starting before the RPC.
func waitForEndpoint(ctx context.Context, endpoint string) error {
	dialer := net.Dialer{Timeout: 200 * time.Millisecond}
	for {
		conn, err := dialer.DialContext(ctx, "tcp", endpoint)
		if err == nil {
			conn.Close()
			return nil
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}
