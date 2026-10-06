// Package zstdstreams stores a working set in at most eight independent long
// Zstd streams. Each stream is fetched once and decoded directly to its output
// extent; there is no fixed-size compression-frame or download task queue.
package zstdstreams

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
	"golang.org/x/sync/errgroup"
)

const (
	Layout         = "streams8-v1"
	Format         = "zstd-streams-v1"
	PageSize       = 4096
	MaxStreams     = 8
	WindowSize     = 8 << 20
	PayloadSuffix  = ".zstd.streams"
	ManifestSuffix = ".zstd.streams.json"
)

type Stream struct {
	Index            int    `json:"index"`
	RawOffset        int64  `json:"raw_offset"`
	RawSize          int64  `json:"raw_size"`
	CompressedOffset int64  `json:"compressed_offset"`
	CompressedSize   int64  `json:"compressed_size"`
	RawSHA256        string `json:"raw_sha256"`
}

type Manifest struct {
	Version        int      `json:"version"`
	Format         string   `json:"format"`
	Codec          string   `json:"codec"`
	Level          int      `json:"level"`
	PageSize       int      `json:"page_size"`
	StreamCount    int      `json:"stream_count"`
	RawSize        int64    `json:"raw_size"`
	CompressedSize int64    `json:"compressed_size"`
	Streams        []Stream `json:"streams"`
}

func (m *Manifest) Validate() error {
	if m == nil {
		return fmt.Errorf("nil streams manifest")
	}
	if m.Version != 1 || m.Format != Format || m.Codec != "zstd" || m.PageSize != PageSize {
		return fmt.Errorf("unsupported streams manifest format")
	}
	if m.RawSize < 0 || m.RawSize%PageSize != 0 || m.CompressedSize < 0 {
		return fmt.Errorf("invalid streams payload sizes")
	}
	pages := m.RawSize / PageSize
	n := min(int64(MaxStreams), pages)
	if int64(m.StreamCount) != n || int64(len(m.Streams)) != n {
		return fmt.Errorf("expected %d streams, got count=%d entries=%d", n, m.StreamCount, len(m.Streams))
	}
	var raw, compressed int64
	for i, s := range m.Streams {
		wantPages := pages / n
		if int64(i) < pages%n {
			wantPages++
		}
		if s.Index != i || s.RawOffset != raw || s.RawSize != wantPages*PageSize ||
			s.CompressedOffset != compressed || s.CompressedSize <= 0 ||
			s.CompressedSize > m.CompressedSize-compressed {
			return fmt.Errorf("invalid extent for stream %d", i)
		}
		if len(s.RawSHA256) != sha256.Size*2 {
			return fmt.Errorf("invalid stream %d SHA length", i)
		}
		if _, err := hex.DecodeString(s.RawSHA256); err != nil {
			return fmt.Errorf("stream %d SHA: %w", i, err)
		}
		raw += s.RawSize
		compressed += s.CompressedSize
	}
	if raw != m.RawSize || compressed != m.CompressedSize {
		return fmt.Errorf("stream extents do not match totals")
	}
	return nil
}

func MarshalManifest(m *Manifest) ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse streams manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}

type countingWriter struct {
	io.Writer
	n int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.Writer.Write(p)
	w.n += int64(n)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	return n, err
}

// EncodeTo writes each independent stream directly to dst, once, in page
// order. Encoding is offline; no full compressed payload buffer is required.
func EncodeTo(dst io.Writer, raw []byte, level int) (*Manifest, error) {
	if len(raw)%PageSize != 0 {
		return nil, fmt.Errorf("WS size %d is not page aligned", len(raw))
	}
	if dst == nil {
		return nil, fmt.Errorf("nil payload writer")
	}
	pages := len(raw) / PageSize
	n := min(MaxStreams, pages)
	m := &Manifest{Version: 1, Format: Format, Codec: "zstd", Level: level, PageSize: PageSize,
		StreamCount: n, RawSize: int64(len(raw)), Streams: make([]Stream, 0, n)}
	w := &countingWriter{Writer: dst}
	var offset int64
	for i := 0; i < n; i++ {
		count := pages / n
		if i < pages%n {
			count++
		}
		s := Stream{Index: i, RawOffset: offset, RawSize: int64(count * PageSize), CompressedOffset: w.n}
		block := raw[offset : offset+s.RawSize]
		encoder, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.EncoderLevelFromZstd(level)),
			zstd.WithEncoderConcurrency(1), zstd.WithEncoderCRC(true), zstd.WithWindowSize(WindowSize))
		if err != nil {
			return nil, err
		}
		_, writeErr := encoder.Write(block)
		closeErr := encoder.Close()
		if writeErr != nil {
			return nil, fmt.Errorf("encode stream %d: %w", i, writeErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close stream %d: %w", i, closeErr)
		}
		s.CompressedSize = w.n - s.CompressedOffset
		sum := sha256.Sum256(block)
		s.RawSHA256 = hex.EncodeToString(sum[:])
		m.Streams = append(m.Streams, s)
		offset += s.RawSize
	}
	m.CompressedSize = w.n
	return m, m.Validate()
}

// OpenRange must honor ctx while opening the response. Its returned reader
// must allow Close to interrupt a blocked Read (as MinIO and files do).
type OpenRange func(ctx context.Context, offset, length int64) (io.ReadCloser, error)

type StreamStats struct {
	Index         int   `json:"index"`
	RangeOpens    int   `json:"range_opens"`
	ReadBytes     int64 `json:"read_bytes"`
	FirstReadUS   int64 `json:"first_read_us"`
	ReadDoneUS    int64 `json:"read_done_us"`
	FirstOutputUS int64 `json:"first_output_us"`
	DoneUS        int64 `json:"done_us"`
	VerifyUS      int64 `json:"verify_us"`
	Success       bool  `json:"success"`
}

type DecodeStats struct {
	Streams   []StreamStats `json:"streams"`
	ElapsedUS int64         `json:"elapsed_us"`
}

type streamReader struct {
	io.ReadCloser
	ctx      context.Context
	start    time.Time
	stats    *StreamStats
	expected int64
	once     sync.Once
	closeErr error
}

func (r *streamReader) Close() error {
	r.once.Do(func() { r.closeErr = r.ReadCloser.Close() })
	return r.closeErr
}
func (r *streamReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		if r.stats.ReadBytes == 0 {
			r.stats.FirstReadUS = time.Since(r.start).Microseconds()
		}
		r.stats.ReadBytes += int64(n)
		if r.stats.ReadBytes == r.expected {
			r.stats.ReadDoneUS = time.Since(r.start).Microseconds()
		}
	}
	return n, err
}

func Decode(ctx context.Context, m *Manifest, open OpenRange, dst []byte) error {
	_, err := DecodeWithStats(ctx, m, open, dst)
	return err
}

func DecodeWithStats(ctx context.Context, m *Manifest, open OpenRange, dst []byte) (*DecodeStats, error) {
	return decode(ctx, m, open, dst, nil)
}

// onOutput is only used by synchronized streaming tests, never by the runtime.
func decode(ctx context.Context, m *Manifest, open OpenRange, dst []byte, onOutput func(int, int)) (*DecodeStats, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	if int64(len(dst)) != m.RawSize {
		return nil, fmt.Errorf("destination size mismatch")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if open == nil && m.StreamCount != 0 {
		return nil, fmt.Errorf("nil range opener")
	}
	start := time.Now()
	stats := &DecodeStats{Streams: make([]StreamStats, m.StreamCount)}
	group, decodeCtx := errgroup.WithContext(ctx)
	for i, stream := range m.Streams {
		i, stream := i, stream
		group.Go(func() error {
			s := &stats.Streams[i]
			s.Index = i
			defer func() { s.DoneUS = time.Since(start).Microseconds() }()
			if err := decodeCtx.Err(); err != nil {
				return err
			}
			s.RangeOpens++
			r, err := open(decodeCtx, stream.CompressedOffset, stream.CompressedSize)
			if err != nil {
				return fmt.Errorf("open stream %d: %w", i, err)
			}
			reader := &streamReader{ReadCloser: r, ctx: decodeCtx, start: start, stats: s, expected: stream.CompressedSize}
			stop := context.AfterFunc(decodeCtx, func() { reader.Close() })
			defer func() { stop(); reader.Close() }()
			decoder, err := zstd.NewReader(reader, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxWindow(WindowSize))
			if err != nil {
				return fmt.Errorf("create stream %d decoder: %w", i, err)
			}
			defer decoder.Close()
			raw := dst[stream.RawOffset : stream.RawOffset+stream.RawSize]
			written := 0
			for written < len(raw) {
				if err := decodeCtx.Err(); err != nil {
					return err
				}
				n, err := decoder.Read(raw[written:])
				if n > 0 {
					if written == 0 {
						s.FirstOutputUS = time.Since(start).Microseconds()
					}
					written += n
					if onOutput != nil {
						onOutput(i, written)
					}
				}
				if err != nil && !(err == io.EOF && written == len(raw)) {
					return fmt.Errorf("decode stream %d: %w", i, err)
				}
			}
			var extra [1]byte
			if n, err := decoder.Read(extra[:]); n != 0 || err != io.EOF {
				return fmt.Errorf("stream %d invalid end: n=%d err=%v", i, n, err)
			}
			if s.ReadBytes != stream.CompressedSize {
				return fmt.Errorf("stream %d compressed size mismatch: %d != %d", i, s.ReadBytes, stream.CompressedSize)
			}
			verifyStart := time.Now()
			sum := sha256.Sum256(raw)
			s.VerifyUS = time.Since(verifyStart).Microseconds()
			if hex.EncodeToString(sum[:]) != stream.RawSHA256 {
				return fmt.Errorf("stream %d raw SHA mismatch", i)
			}
			if err := decodeCtx.Err(); err != nil {
				return err
			}
			s.Success = true
			return nil
		})
	}
	err := group.Wait() // No caller may release dst until all writers have exited.
	stats.ElapsedUS = time.Since(start).Microseconds()
	if err == nil {
		err = ctx.Err()
	}
	return stats, err
}
