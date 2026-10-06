package zstdstreams

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func encoded(t *testing.T, pages int) ([]byte, []byte, *Manifest) {
	t.Helper()
	raw := make([]byte, pages*PageSize)
	_, err := rand.New(rand.NewSource(41)).Read(raw)
	require.NoError(t, err)
	var buf bytes.Buffer
	m, err := EncodeTo(&buf, raw, 3)
	require.NoError(t, err)
	return raw, buf.Bytes(), m
}

func TestRoundTrip(t *testing.T) {
	for _, pages := range []int{0, 1, 7, 8, 9, 19, 3000} {
		t.Run(strconv.Itoa(pages), func(t *testing.T) {
			raw, payload, m := encoded(t, pages)
			require.Equal(t, min(8, pages), m.StreamCount)
			data, err := MarshalManifest(m)
			require.NoError(t, err)
			parsed, err := ParseManifest(data)
			require.NoError(t, err)
			require.Equal(t, m, parsed)
			var calls atomic.Int32
			out := make([]byte, len(raw))
			stats, err := DecodeWithStats(context.Background(), parsed, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
				calls.Add(1)
				return io.NopCloser(bytes.NewReader(payload[off : off+n])), nil
			}, out)
			require.NoError(t, err)
			require.Equal(t, int32(m.StreamCount), calls.Load())
			require.Equal(t, raw, out)
			for i, s := range stats.Streams {
				require.True(t, s.Success)
				require.Equal(t, 1, s.RangeOpens)
				require.Equal(t, m.Streams[i].CompressedSize, s.ReadBytes)
			}
		})
	}
}

func TestRejectInvalidInputAndManifest(t *testing.T) {
	_, err := EncodeTo(io.Discard, []byte("not a page"), 3)
	require.Error(t, err)
	_, payload, m := encoded(t, 19)
	m.Streams[0].RawOffset++
	require.Error(t, m.Validate())
	m.Streams[0].RawOffset--
	m.Streams[0].CompressedSize = int64(len(payload)) + 1
	require.Error(t, m.Validate())
	_, err = ParseManifest([]byte(`{"version":1,"codec":"zstd","frame_size":1048576}`))
	require.Error(t, err, "legacy frames must never be accepted as streams")
}

func TestCorruptionAndTruncation(t *testing.T) {
	for _, truncate := range []bool{false, true} {
		raw, payload, m := encoded(t, 80)
		if !truncate {
			payload[m.Streams[0].CompressedSize/2] ^= 0x80
		}
		err := Decode(context.Background(), m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
			if truncate && off == 0 {
				n--
			}
			return io.NopCloser(bytes.NewReader(payload[off : off+n])), nil
		}, make([]byte, len(raw)))
		require.Error(t, err)
	}
}

type gatedReader struct {
	*bytes.Reader
	gate      <-chan struct{}
	remaining int
	once      sync.Once
	closed    chan struct{}
}

func (r *gatedReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		select {
		case <-r.gate:
		case <-r.closed:
			return 0, context.Canceled
		}
	} else if len(p) > r.remaining {
		p = p[:r.remaining]
	}
	n, err := r.Reader.Read(p)
	r.remaining -= n
	return n, err
}
func (r *gatedReader) Close() error { r.once.Do(func() { close(r.closed) }); return nil }

func TestOutputBeforeInputTail(t *testing.T) {
	raw, payload, m := encoded(t, 4096)
	gate := make(chan struct{})
	first := make(chan struct{})
	var firstOnce sync.Once
	var gateOnce sync.Once
	unblock := func() { gateOnce.Do(func() { close(gate) }) }
	defer unblock()
	out := make([]byte, len(raw))
	done := make(chan error, 1)
	go func() {
		_, err := decode(context.Background(), m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
			if off != 0 {
				return io.NopCloser(bytes.NewReader(payload[off : off+n])), nil
			}
			return &gatedReader{Reader: bytes.NewReader(payload[:n]), gate: gate, remaining: 512 * 1024, closed: make(chan struct{})}, nil
		}, out, func(i, written int) {
			if i == 0 {
				firstOnce.Do(func() { close(first) })
			}
		})
		done <- err
	}()
	select {
	case <-first: // Synchronized first output, while stream 0 input is still gated.
	case err := <-done:
		t.Fatalf("decode ended before first output: %v", err)
	case <-time.After(5 * time.Second):
		unblock()
		<-done
		t.Fatal("decoder waited for the complete input")
	}
	unblock()
	require.NoError(t, <-done)
	require.Equal(t, raw, out)
}

type blockedReader struct {
	closed     chan struct{}
	closeCount *atomic.Int32
	once       sync.Once
	allOpened  <-chan struct{}
	fail       bool
}

func (r *blockedReader) Read([]byte) (int, error) {
	<-r.allOpened
	if r.fail {
		return 0, errors.New("injected range failure")
	}
	<-r.closed
	return 0, context.Canceled
}
func (r *blockedReader) Close() error {
	r.once.Do(func() { r.closeCount.Add(1); close(r.closed) })
	return nil
}

func TestFailureCancelsAndClosesEveryRange(t *testing.T) {
	raw, _, m := encoded(t, 80)
	var opened, closed atomic.Int32
	allOpened := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Decode(context.Background(), m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
			if opened.Add(1) == 8 {
				close(allOpened)
			}
			return &blockedReader{closed: make(chan struct{}), closeCount: &closed, allOpened: allOpened, fail: off == 0}, nil
		}, make([]byte, len(raw)))
	}()
	select {
	case err := <-done:
		require.ErrorContains(t, err, "injected range failure")
	case <-time.After(5 * time.Second):
		t.Fatal("sibling reads were not cancelled")
	}
	require.Equal(t, int32(8), closed.Load())
}

func TestParentCancellation(t *testing.T) {
	raw, _, m := encoded(t, 80)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var opened, closed atomic.Int32
	allOpened := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- Decode(ctx, m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
			if opened.Add(1) == 8 {
				close(allOpened)
			}
			return &blockedReader{closed: make(chan struct{}), closeCount: &closed, allOpened: allOpened}, nil
		}, make([]byte, len(raw)))
	}()
	<-allOpened
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("parent cancellation did not stop decode")
	}
	require.Equal(t, int32(8), closed.Load())
}
