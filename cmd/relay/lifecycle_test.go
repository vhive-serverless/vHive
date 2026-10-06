package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCleanupSurvivesRequestCancellation(t *testing.T) {
	type key struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), key{}, "request"))
	start, done := make(chan struct{}), make(chan struct{})
	runAsyncCleanup(ctx, func(cleanup context.Context) {
		defer close(done)
		<-start
		if cleanup.Err() != nil || cleanup.Done() != nil || cleanup.Value(key{}) != "request" {
			t.Errorf("cleanup inherited cancellation or lost context values")
		}
	})
	if cleanupTasks.Load() != 1 {
		t.Fatal("cleanup must be counted before scheduling")
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatal("restore request must still be cancellable")
	}
	close(start)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not exit")
	}
	deadline := time.Now().Add(time.Second)
	for cleanupTasks.Load() != 0 {
		if time.Now().After(deadline) {
			t.Fatal("cleanup counter did not settle")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestRuntimeStatusDoesNotEnterFunctionHandler(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, runtimeStatePath, nil)
	w := httptest.NewRecorder()
	if !handleRuntimeState(w, r) || w.Code != http.StatusServiceUnavailable {
		t.Fatal("uninitialized runtime must report not ready")
	}
	r = httptest.NewRequest(http.MethodPost, runtimeStatePath, nil)
	w = httptest.NewRecorder()
	if !handleRuntimeState(w, r) || w.Code != http.StatusMethodNotAllowed {
		t.Fatal("runtime status is read-only")
	}
}
