// inventory checks the actual new stores against the frozen copy selection and
// verified transcodes. It reads metadata/indices, not every payload again.
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
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
)

type workload struct {
	Profile  string `json:"profile"`
	Snapshot string `json:"snapshot"`
}
type job struct {
	ID          string   `json:"corpus_id"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Kind        string   `json:"kind"`
	Mode        string   `json:"transcode_mode"`
	Tier        int      `json:"tier"`
	Prefixes    []string `json:"prefixes"`
	Workloads   struct {
		Rows []workload `json:"workloads"`
	} `json:"workloads"`
}
type entry struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	ETag string `json:"etag"`
}
type scanJob struct {
	job
	Objects []entry `json:"objects"`
}
type configRow struct {
	System    string `json:"system"`
	Profile   string `json:"profile"`
	Snapshot  string `json:"snapshot"`
	CorpusID  string `json:"corpus_id"`
	Tier      int    `json:"tier"`
	Port      int    `json:"port"`
	Security  string `json:"security"`
	ChunkSize int    `json:"chunk_size"`
	Layout    string `json:"ws_layout"`
	Streams   int    `json:"ws_streams"`
}
type row struct {
	configRow
	Endpoint        string                `json:"endpoint"`
	Source          string                `json:"source_endpoint"`
	PayloadKey      string                `json:"payload_key"`
	ManifestKey     string                `json:"manifest_key"`
	IndexKey        string                `json:"index_key"`
	RawBytes        int64                 `json:"raw_bytes"`
	CompressedBytes int64                 `json:"compressed_bytes"`
	IndexPages      int                   `json:"index_pages"`
	WSPages         int                   `json:"working_set_pages"`
	RecipeBytes     int64                 `json:"recipe_bytes"`
	Manifest        *zstdstreams.Manifest `json:"manifest,omitempty"`
}
type proof struct {
	Snapshot       string                `json:"snapshot"`
	OutputVerified bool                  `json:"output_verified"`
	Manifest       *zstdstreams.Manifest `json:"manifest"`
	Destination    string                `json:"destination_endpoint"`
}
type storeReport struct {
	ID            string `json:"corpus_id"`
	Endpoint      string `json:"endpoint"`
	CopiedObjects int    `json:"copied_objects"`
	CopiedBytes   int64  `json:"copied_bytes"`
	NewObjects    int    `json:"new_ws_objects"`
	NewBytes      int64  `json:"new_ws_bytes"`
}

func load(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(value)
}
func read(ctx context.Context, c *minio.Client, key string) ([]byte, error) {
	f, err := c.GetObject(ctx, "snapshots", key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
func pfns(data []byte) ([]int, error) {
	rows, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 || len(rows[0]) != 1 || rows[0][0] != "pfn" {
		return nil, fmt.Errorf("invalid PFN header")
	}
	seen := map[int]bool{}
	result := make([]int, 0, len(rows)-1)
	for _, fields := range rows[1:] {
		if len(fields) != 1 {
			return nil, fmt.Errorf("invalid PFN row")
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil || n < 0 || seen[n] {
			return nil, fmt.Errorf("invalid/duplicate PFN")
		}
		seen[n] = true
		result = append(result, n)
	}
	return result, nil
}
func payloadBase(mode string) string {
	if mode == "full" {
		return "working_set_pages_content"
	}
	return "working_set_pages_content_private"
}
func expectedExtras(j job) map[string]bool {
	out := map[string]bool{}
	if j.Mode == "none" {
		return out
	}
	for _, w := range j.Workloads.Rows {
		base := w.Snapshot + "/" + payloadBase(j.Mode)
		out[base+zstdstreams.PayloadSuffix] = true
		out[base+zstdstreams.ManifestSuffix] = true
	}
	return out
}
func checkCopied(ctx context.Context, c *minio.Client, got minio.ObjectInfo, want entry, source string) error {
	if got.Size != want.Size {
		return fmt.Errorf("size differs: %s", want.Key)
	}
	if got.ETag == want.ETag {
		return nil
	}
	// Multipart ETags may differ when the upload part size changes. The copy tool
	// uses If-Match on the source and records that identity on the destination.
	info, err := c.StatObject(ctx, "snapshots", want.Key, minio.StatObjectOptions{})
	if err != nil {
		return err
	}
	if info.Metadata.Get("X-Amz-Meta-Streams8-Source") != source ||
		info.Metadata.Get("X-Amz-Meta-Streams8-Source-Etag") != want.ETag {
		return fmt.Errorf("copy identity differs: %s", want.Key)
	}
	return nil
}
func inspectStore(ctx context.Context, c *minio.Client, j job, frozen scanJob) (storeReport, map[string]entry, error) {
	r := storeReport{ID: j.ID, Endpoint: j.Destination}
	wanted := make(map[string]entry, len(frozen.Objects))
	for _, o := range frozen.Objects {
		wanted[o.Key] = o
	}
	extra := expectedExtras(j)
	// Keep small per-revision entries for the rows/alias budget, not the whole namespace.
	revisionEntries := map[string]entry{}
	revisions := map[string]bool{}
	for _, w := range j.Workloads.Rows {
		revisions[w.Snapshot] = true
	}
	for _, prefix := range j.Prefixes {
		for o := range c.ListObjects(ctx, "snapshots", minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if o.Err != nil {
				return r, nil, o.Err
			}
			if want, ok := wanted[o.Key]; ok {
				if err := checkCopied(ctx, c, o, want, j.Source); err != nil {
					return r, nil, err
				}
				delete(wanted, o.Key)
				r.CopiedObjects++
				r.CopiedBytes += o.Size
			} else if extra[o.Key] {
				delete(extra, o.Key)
				r.NewObjects++
				r.NewBytes += o.Size
			} else {
				return r, nil, fmt.Errorf("unexpected/duplicate object: %s/%s", j.ID, o.Key)
			}
			rev, _, _ := strings.Cut(o.Key, "/")
			if revisions[rev] {
				revisionEntries[o.Key] = entry{o.Key, o.Size, o.ETag}
			}
		}
	}
	if len(wanted) != 0 || len(extra) != 0 {
		return r, nil, fmt.Errorf("missing copied=%d WS=%d in %s", len(wanted), len(extra), j.ID)
	}
	return r, revisionEntries, nil
}
func inspectRow(ctx context.Context, c *minio.Client, j job, cfg configRow, entries map[string]entry, verified map[string]proof) (row, error) {
	r := row{configRow: cfg, Endpoint: j.Destination, Source: j.Source}
	for _, name := range []string{"snap_file", "recipe_file", "working_set_pages"} {
		if _, ok := entries[cfg.Snapshot+"/"+name]; !ok {
			return r, fmt.Errorf("required %s missing", name)
		}
	}
	r.RecipeBytes = entries[cfg.Snapshot+"/recipe_file"].Size
	wsData, err := read(ctx, c, cfg.Snapshot+"/working_set_pages")
	if err != nil {
		return r, err
	}
	ws, err := pfns(wsData)
	if err != nil {
		return r, err
	}
	r.WSPages = len(ws)
	if cfg.Layout == "not-applicable" {
		if cfg.Streams != 0 || (cfg.System != "chunks-128k-zstd3" && cfg.System != "pages-4k-zstd3") {
			return r, fmt.Errorf("invalid native configuration")
		}
		return r, nil
	}
	if cfg.Layout != zstdstreams.Layout || cfg.Streams != 8 {
		return r, fmt.Errorf("invalid coalesced configuration")
	}
	base := cfg.Snapshot + "/" + payloadBase(j.Mode)
	r.PayloadKey, r.ManifestKey = base+zstdstreams.PayloadSuffix, base+zstdstreams.ManifestSuffix
	r.IndexKey = cfg.Snapshot + "/working_set_pages_index_private"
	if j.Mode == "full" {
		r.IndexKey = cfg.Snapshot + "/working_set_pages"
	}
	data, err := read(ctx, c, r.ManifestKey)
	if err != nil {
		return r, err
	}
	r.Manifest, err = zstdstreams.ParseManifest(data)
	if err != nil {
		return r, err
	}
	if r.Manifest.Level != 3 || r.Manifest.StreamCount != 8 {
		return r, fmt.Errorf("unexpected encoding settings")
	}
	r.RawBytes, r.CompressedBytes = r.Manifest.RawSize, r.Manifest.CompressedSize
	if entries[r.PayloadKey].Size != r.CompressedBytes {
		return r, fmt.Errorf("actual payload size differs")
	}
	data, err = read(ctx, c, r.IndexKey)
	if err != nil {
		return r, err
	}
	index, err := pfns(data)
	if err != nil {
		return r, err
	}
	r.IndexPages = len(index)
	if int64(len(index))*4096 != r.RawBytes {
		return r, fmt.Errorf("raw/index length differs")
	}
	wsSet := map[int]bool{}
	for _, p := range ws {
		wsSet[p] = true
	}
	for _, p := range index {
		if !wsSet[p] {
			return r, fmt.Errorf("private PFN not in original WS")
		}
	}
	p, ok := verified[cfg.Snapshot]
	if !ok || !p.OutputVerified || !reflect.DeepEqual(p.Manifest, r.Manifest) {
		return r, fmt.Errorf("missing/mismatched frozen transcode proof")
	}
	return r, nil
}

func run() error {
	root := flag.String("root", "", "backend run root containing config/stages")
	report := flag.String("report", "", "new actual-layout inventory JSON")
	flag.Parse()
	if *root == "" || *report == "" {
		return fmt.Errorf("explicit root/report required")
	}
	if _, err := os.Stat(*report); !os.IsNotExist(err) {
		return fmt.Errorf("report exists/inaccessible")
	}
	var plan struct {
		RunID string      `json:"run_id"`
		Jobs  []job       `json:"jobs"`
		Rows  []configRow `json:"configuration_rows"`
	}
	if err := load(filepath.Join(*root, "config/plan.json"), &plan); err != nil {
		return err
	}
	if len(plan.Jobs) != 18 || len(plan.Rows) != 102 {
		return fmt.Errorf("expected complete 18-store/102-row plan")
	}
	var done struct {
		RunID   string   `json:"run_id"`
		Layout  string   `json:"layout"`
		Reports []string `json:"reports"`
	}
	if err := load(filepath.Join(*root, "ORACLE_VIEWS_COMPLETE.json"), &done); err != nil {
		return err
	}
	if done.RunID != plan.RunID || done.Layout != zstdstreams.Layout || len(done.Reports) != 3 {
		return fmt.Errorf("oracle views incomplete")
	}
	var frozen struct {
		Jobs []scanJob `json:"jobs"`
	}
	if err := load(filepath.Join(*root, "source-inventory.json"), &frozen); err != nil {
		return err
	}
	if len(frozen.Jobs) != 18 {
		return fmt.Errorf("source inventory incomplete")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var stores []storeReport
	var rows []row
	aliasEntries := map[string]map[string]entry{}
	seen := map[string]bool{}
	for i, j := range plan.Jobs {
		f := frozen.Jobs[i]
		if f.ID != j.ID || f.Source != j.Source || f.Destination != j.Destination || !reflect.DeepEqual(f.Prefixes, j.Prefixes) {
			return fmt.Errorf("frozen plan mismatch")
		}
		c, err := minio.New(j.Destination, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
		if err != nil {
			return err
		}
		s, entries, err := inspectStore(ctx, c, j, f)
		if err != nil {
			return err
		}
		stores = append(stores, s)
		if j.Kind != "full-dedup-4k" {
			aliasEntries[j.ID] = entries
		}
		verified := map[string]proof{}
		if j.Mode != "none" {
			ref := j
			if j.Mode == "view" {
				for _, other := range plan.Jobs {
					if other.Tier == j.Tier && other.Kind == "partial-4k" {
						ref = other
					}
				}
			}
			var transcode struct {
				Rows []proof `json:"rows"`
			}
			if err := load(filepath.Join(*root, "stages", "transcode-"+ref.ID+".report.json"), &transcode); err != nil {
				return err
			}
			for _, p := range transcode.Rows {
				if p.Destination != ref.Destination {
					return fmt.Errorf("transcode endpoint mismatch")
				}
				verified[p.Snapshot] = p
			}
		}
		for _, cfg := range plan.Rows {
			if cfg.CorpusID != j.ID {
				continue
			}
			key := cfg.System + "/" + cfg.Profile
			if seen[key] || cfg.Tier != j.Tier || !strings.HasSuffix(j.Destination, ":"+strconv.Itoa(cfg.Port)) {
				return fmt.Errorf("duplicate/mismatched row")
			}
			seen[key] = true
			r, err := inspectRow(ctx, c, j, cfg, entries, verified)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			rows = append(rows, r)
		}
		fmt.Printf("INVENTORY_STORE_OK corpus=%s copied=%d new_ws=%d rows=%d\n", j.ID, s.CopiedObjects, s.NewObjects, len(rows))
	}
	if len(rows) != 102 {
		return fmt.Errorf("actual rows=%d, expected 102", len(rows))
	}
	build, _ := debug.ReadBuildInfo()
	result := struct {
		RunID        string                      `json:"run_id"`
		Layout       string                      `json:"layout"`
		Rows         []row                       `json:"rows"`
		Stores       []storeReport               `json:"stores"`
		AliasEntries map[string]map[string]entry `json:"alias_source_entries"`
		Build        *debug.BuildInfo            `json:"build"`
		Definition   string                      `json:"verification_definition"`
	}{plan.RunID, zstdstreams.Layout, rows, stores, aliasEntries, build,
		"Actual selected keys and sizes; ETag match or If-Match copy provenance, not a new full payload hash scan. WS manifests equal verified transcodes; PFNs preserved by immutable index copies. Oracle canonical validation is recorded separately. Aliases and runtime gates are not implied."}
	f, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(result)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
