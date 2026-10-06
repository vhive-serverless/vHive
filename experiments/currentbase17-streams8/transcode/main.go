// transcode re-encodes frozen WS bytes only. It never re-records a VM, changes
// PFN order, or writes to the source corpus. Legacy frames are an explicit
// offline input format, not a runtime fallback.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	legacy "github.com/vhive-serverless/vhive/snapshotting/zstdstream"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
)

type workload struct {
	Profile  string `json:"profile"`
	Snapshot string `json:"snapshot"`
	MiB      int    `json:"vm_mib"`
}

type row struct {
	Profile             string                `json:"profile"`
	Snapshot            string                `json:"snapshot"`
	MiB                 int                   `json:"vm_mib"`
	Layout              string                `json:"layout"`
	SourceEndpoint      string                `json:"source_endpoint"`
	DestinationEndpoint string                `json:"destination_endpoint"`
	SourceManifest      string                `json:"source_manifest"`
	PayloadKey          string                `json:"payload_key"`
	ManifestKey         string                `json:"manifest_key"`
	IndexKey            string                `json:"index_key"`
	Pages               int                   `json:"pages"`
	RawBytes            int64                 `json:"raw_bytes"`
	CompressedBytes     int64                 `json:"compressed_bytes"`
	OldCompressedBytes  int64                 `json:"old_compressed_bytes"`
	OldFrames           int                   `json:"old_frames"`
	Streams             int                   `json:"stream_count"`
	OutputVerified      bool                  `json:"output_verified"`
	SourceLayout        json.RawMessage       `json:"source_layout"`
	Manifest            *zstdstreams.Manifest `json:"manifest"`
}

func read(ctx context.Context, c *minio.Client, bucket, key string) ([]byte, error) {
	r, err := c.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func pageCount(index []byte) (int, error) {
	rows, err := csv.NewReader(bytes.NewReader(index)).ReadAll()
	if err != nil {
		return 0, err
	}
	if len(rows) == 0 || len(rows[0]) != 1 || rows[0][0] != "pfn" {
		return 0, fmt.Errorf("invalid PFN header")
	}
	seen := map[int]bool{}
	for _, r := range rows[1:] {
		if len(r) != 1 {
			return 0, fmt.Errorf("invalid PFN row")
		}
		p, err := strconv.Atoi(r[0])
		if err != nil || p < 0 || seen[p] {
			return 0, fmt.Errorf("invalid/duplicate PFN %q", r[0])
		}
		seen[p] = true
	}
	return len(rows) - 1, nil
}

func convert(ctx context.Context, source, target *minio.Client, bucket, mode string, w workload) (row, error) {
	base := "working_set_pages_content_private"
	indexName := "working_set_pages_index_private"
	if mode == "full" {
		base = "working_set_pages_content"
		indexName = "working_set_pages"
	}
	key := w.Snapshot + "/" + base
	result := row{Profile: w.Profile, Snapshot: w.Snapshot, MiB: w.MiB, Layout: zstdstreams.Layout,
		SourceEndpoint: source.EndpointURL().Host, DestinationEndpoint: target.EndpointURL().Host,
		SourceManifest: key + ".zstd.json", PayloadKey: key + zstdstreams.PayloadSuffix,
		ManifestKey: key + zstdstreams.ManifestSuffix, IndexKey: w.Snapshot + "/" + indexName}
	// Reject overwrites and mixed layouts. The destination must be a fresh
	// metadata/native-object mirror made without old or new compressed WS files.
	for _, suffix := range []string{".zstd.json", ".zstd.frames", zstdstreams.ManifestSuffix, zstdstreams.PayloadSuffix} {
		_, err := target.StatObject(ctx, bucket, key+suffix, minio.StatObjectOptions{})
		if err == nil {
			return result, fmt.Errorf("destination already contains %s", key+suffix)
		}
		if code := minio.ToErrorResponse(err).Code; code != "NoSuchKey" && code != "NotFound" {
			return result, err
		}
	}
	data, err := read(ctx, source, bucket, result.SourceManifest)
	if err != nil {
		return result, err
	}
	old, err := legacy.ParseManifest(data)
	if err != nil {
		return result, err
	}
	result.SourceLayout = append(json.RawMessage(nil), data...)
	payload, err := read(ctx, source, bucket, key+".zstd.frames")
	if err != nil {
		return result, err
	}
	if int64(len(payload)) != old.CompressedSize {
		return result, fmt.Errorf("legacy payload size mismatch")
	}
	index, err := read(ctx, source, bucket, result.IndexKey)
	if err != nil {
		return result, err
	}
	pages, err := pageCount(index)
	if err != nil {
		return result, err
	}
	if int64(pages)*zstdstreams.PageSize != old.RawSize {
		return result, fmt.Errorf("raw/PFN length mismatch")
	}
	indexExists := false
	if _, err := target.StatObject(ctx, bucket, result.IndexKey, minio.StatObjectOptions{}); err == nil {
		copiedIndex, err := read(ctx, target, bucket, result.IndexKey)
		if err != nil {
			return result, err
		}
		if !bytes.Equal(copiedIndex, index) {
			return result, fmt.Errorf("destination index differs from frozen source")
		}
		indexExists = true
	} else if code := minio.ToErrorResponse(err).Code; code != "NoSuchKey" && code != "NotFound" {
		return result, err
	}
	raw := make([]byte, old.RawSize)
	err = legacy.Decode(ctx, old, func(off, n int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload[off : off+n])), nil
	}, raw, 8)
	if err != nil {
		return result, err
	}
	f, err := os.CreateTemp("", "splitsnap-transcode-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	m, err := zstdstreams.EncodeTo(f, raw, 3)
	if err != nil {
		return result, err
	}
	// One local roundtrip validates each newly encoded representation before
	// publishing. No extra per-page or corpus-wide SHA pass is added.
	out := make([]byte, len(raw))
	err = zstdstreams.Decode(ctx, m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
		return io.NopCloser(io.NewSectionReader(f, off, n)), nil
	}, out)
	if err != nil {
		return result, err
	}
	if !bytes.Equal(raw, out) {
		return result, fmt.Errorf("new streams changed raw content")
	}
	meta, err := zstdstreams.MarshalManifest(m)
	if err != nil {
		return result, err
	}
	if _, err = target.PutObject(ctx, bucket, result.PayloadKey, io.NewSectionReader(f, 0, m.CompressedSize), m.CompressedSize, minio.PutObjectOptions{}); err != nil {
		return result, err
	}
	// The small index remains byte-for-byte unchanged, not sorted or regenerated.
	if !indexExists {
		if _, err = target.PutObject(ctx, bucket, result.IndexKey, bytes.NewReader(index), int64(len(index)), minio.PutObjectOptions{}); err != nil {
			return result, err
		}
	}
	if _, err = target.PutObject(ctx, bucket, result.ManifestKey, bytes.NewReader(meta), int64(len(meta)), minio.PutObjectOptions{}); err != nil {
		return result, err
	}
	result.Pages = pages
	result.RawBytes = m.RawSize
	result.CompressedBytes = m.CompressedSize
	result.OldCompressedBytes = old.CompressedSize
	result.OldFrames = len(old.Frames)
	result.Streams = m.StreamCount
	result.OutputVerified = true
	result.Manifest = m
	return result, nil
}

// A committed row is the only resume authority. Existing remote files without
// a row remain an incomplete attempt, not an implicit success or overwrite.
func resume(ctx context.Context, source, target *minio.Client, bucket, mode string, w workload, r row) error {
	base, index := "working_set_pages_content_private", "working_set_pages_index_private"
	if mode == "full" {
		base, index = "working_set_pages_content", "working_set_pages"
	}
	key := w.Snapshot + "/" + base
	if r.Profile != w.Profile || r.Snapshot != w.Snapshot || r.MiB != w.MiB || !r.OutputVerified ||
		r.Layout != zstdstreams.Layout || r.SourceEndpoint != source.EndpointURL().Host ||
		r.DestinationEndpoint != target.EndpointURL().Host || r.PayloadKey != key+zstdstreams.PayloadSuffix ||
		r.ManifestKey != key+zstdstreams.ManifestSuffix || r.SourceManifest != key+".zstd.json" ||
		r.IndexKey != w.Snapshot+"/"+index {
		return fmt.Errorf("resume row source/config mismatch for %s", w.Profile)
	}
	if err := r.Manifest.Validate(); err != nil {
		return err
	}
	info, err := target.StatObject(ctx, bucket, r.PayloadKey, minio.StatObjectOptions{})
	if err != nil {
		return err
	}
	if info.Size != r.CompressedBytes {
		return fmt.Errorf("resume size mismatch: %s: got %d, expected %d", r.PayloadKey, info.Size, r.CompressedBytes)
	}
	data, err := read(ctx, source, bucket, r.SourceManifest)
	if err != nil {
		return err
	}
	currentSource, err := legacy.ParseManifest(data)
	if err != nil {
		return err
	}
	recordedSource, err := legacy.ParseManifest(r.SourceLayout)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentSource, recordedSource) {
		return fmt.Errorf("legacy source manifest changed for %s", w.Profile)
	}
	data, err = read(ctx, target, bucket, r.ManifestKey)
	if err != nil {
		return err
	}
	m, err := zstdstreams.ParseManifest(data)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(m, r.Manifest) || m.CompressedSize != r.CompressedBytes || m.RawSize != r.RawBytes ||
		m.StreamCount != r.Streams || int64(r.Pages)*zstdstreams.PageSize != r.RawBytes {
		return fmt.Errorf("resume manifest/report mismatch for %s", w.Profile)
	}
	a, err := read(ctx, source, bucket, r.IndexKey)
	if err != nil {
		return err
	}
	b, err := read(ctx, target, bucket, r.IndexKey)
	if err != nil {
		return err
	}
	if !bytes.Equal(a, b) {
		return fmt.Errorf("resume index changed for %s", w.Profile)
	}
	return nil
}

func publishJSON(path string, value any) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("report already exists: %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	e := json.NewEncoder(f)
	e.SetIndent("", "  ")
	writeErr := e.Encode(value)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func run() error {
	sourceEndpoint := flag.String("source", "", "read-only legacy MinIO endpoint")
	targetEndpoint := flag.String("destination", "", "new isolated MinIO endpoint")
	input := flag.String("workloads", "", "frozen tier workload manifest")
	report := flag.String("report", "", "new output report path")
	mode := flag.String("mode", "private", "full (Sabre) or private (SplitSnap variants/oracle view)")
	bucket := flag.String("bucket", "snapshots", "bucket")
	access := flag.String("access-key", "minio", "MinIO access key")
	secret := flag.String("secret-key", "minio123", "MinIO secret key")
	flag.Parse()
	if *sourceEndpoint == "" || *targetEndpoint == "" || *sourceEndpoint == *targetEndpoint || *report == "" || *input == "" {
		return fmt.Errorf("distinct source/destination, workloads and report are required")
	}
	if *mode != "full" && *mode != "private" {
		return fmt.Errorf("mode must be full or private")
	}
	if _, err := os.Stat(*report); err == nil {
		return fmt.Errorf("report exists: %s", *report)
	} else if !os.IsNotExist(err) {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	newClient := func(endpoint string) (*minio.Client, error) {
		return minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(*access, *secret, ""), Secure: false})
	}
	source, err := newClient(*sourceEndpoint)
	if err != nil {
		return err
	}
	target, err := newClient(*targetEndpoint)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*input)
	if err != nil {
		return err
	}
	var conf struct {
		Workloads []workload `json:"workloads"`
	}
	if err := json.Unmarshal(data, &conf); err != nil {
		return err
	}
	if len(conf.Workloads) == 0 {
		return fmt.Errorf("empty workload manifest")
	}
	rows := make([]row, 0, len(conf.Workloads))
	if err := os.MkdirAll(*report+".rows", 0755); err != nil {
		return err
	}
	lock, err := os.OpenFile(*report+".lock", os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("transcode report is owned by another live process: %w", err)
	}
	for _, w := range conf.Workloads {
		if w.Snapshot == "" || strings.ContainsAny(w.Snapshot, "/\\") {
			return fmt.Errorf("invalid snapshot name")
		}
		rowPath := filepath.Join(*report+".rows", w.Snapshot+".json")
		var r row
		prior, err := os.ReadFile(rowPath)
		if err == nil {
			if err := json.Unmarshal(prior, &r); err != nil {
				return err
			}
			if err := resume(ctx, source, target, *bucket, *mode, w, r); err != nil {
				return err
			}
			rows = append(rows, r)
			fmt.Fprintf(os.Stderr, "TRANSCODE_RESUMED profile=%s streams=%d\n", w.Profile, r.Streams)
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		r, err = convert(ctx, source, target, *bucket, *mode, w)
		if err != nil {
			return fmt.Errorf("%s: %w", w.Profile, err)
		}
		if err := publishJSON(rowPath, r); err != nil {
			return err
		}
		rows = append(rows, r)
		fmt.Fprintf(os.Stderr, "TRANSCODE_OK profile=%s old_frames=%d streams=%d raw=%d compressed=%d\n", w.Profile, r.OldFrames, r.Streams, r.RawBytes, r.CompressedBytes)
	}
	build, _ := debug.ReadBuildInfo()
	result := struct {
		Layout string           `json:"layout"`
		Rows   []row            `json:"rows"`
		Build  *debug.BuildInfo `json:"build"`
	}{zstdstreams.Layout, rows, build}
	return publishJSON(*report, result)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
