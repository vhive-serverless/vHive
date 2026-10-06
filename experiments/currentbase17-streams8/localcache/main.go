// localcache prepares only an explicitly isolated AES SplitSnap local cache.
// It copies the new compressed bytes, never re-encodes or reconstructs a new WS.
package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
	"golang.org/x/sync/errgroup"
)

func recipeKeys(recipe []byte) (map[string]bool, error) {
	if len(recipe) == 0 || len(recipe)%16 != 0 {
		return nil, fmt.Errorf("invalid recipe length")
	}
	keys := map[string]bool{}
	for i := 0; i < len(recipe); i += 16 {
		h := hex.EncodeToString(recipe[i : i+16])
		keys["_chunks_zstd_v1_l3/"+h[:2]+"/"+h] = true
	}
	return keys, nil
}
func keep(key string) bool {
	n := path.Base(key)
	return n != "mem_file" && n != "working_set_pages_content" && n != "working_set_pages_content_private" &&
		!strings.HasSuffix(n, ".zstd.frames") && !strings.HasSuffix(n, ".zstd.json")
}
func run() error {
	planPath := flag.String("plan", "", "frozen streams8 plan")
	configPath := flag.String("config", "", "isolated AES all-local point config")
	report := flag.String("report", "", "new preparation receipt")
	flag.Parse()
	var cfg struct {
		Root      string `json:"run_root"`
		Cache     string `json:"cache_dir"`
		Mode      string `json:"mode"`
		Security  string `json:"security"`
		Endpoint  string `json:"minio_endpoint"`
		Coalesced bool   `json:"ws_coalescing"`
	}
	load := func(p string, v any) error {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return json.Unmarshal(b, v)
	}
	if err := load(*configPath, &cfg); err != nil {
		return err
	}
	if cfg.Mode != "local" || cfg.Security != "partial" || !cfg.Coalesced || !strings.HasPrefix(cfg.Root, "/users/Liquidz/streams8/") || !strings.HasPrefix(cfg.Cache, cfg.Root+"/points/") || filepath.Clean(cfg.Cache) != cfg.Cache {
		return fmt.Errorf("isolated matched local config required")
	}
	if _, err := os.Stat(*report); !os.IsNotExist(err) {
		return fmt.Errorf("report exists/inaccessible")
	}
	var plan struct {
		Workloads struct {
			Rows []struct {
				Profile   string `json:"profile"`
				Snapshot  string `json:"snapshot"`
				Image     string `json:"image_inventory"`
				Overrides map[string]struct {
					Port int `json:"port"`
				} `json:"corpus_overrides"`
			} `json:"workloads"`
		} `json:"workloads"`
		Backend string `json:"backend"`
	}
	if err := load(*planPath, &plan); err != nil {
		return err
	}
	var snapshot, image string
	for _, w := range plan.Workloads.Rows {
		if w.Profile == "aes-go-45000-45450" {
			if cfg.Endpoint != fmt.Sprintf("%s:%d", plan.Backend, w.Overrides["splitsnap-zstd3"].Port) {
				return fmt.Errorf("wrong corpus endpoint")
			}
			snapshot, image = w.Snapshot, w.Image
		}
	}
	if snapshot == "" {
		return fmt.Errorf("AES input missing")
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Cache), 0755); err != nil {
		return err
	}
	if err := os.Mkdir(cfg.Cache, 0755); err != nil {
		return fmt.Errorf("fresh cache required: %w", err)
	}
	c, err := minio.New(cfg.Endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var files []string
	for _, prefix := range []string{"base/", snapshot + "/", "ws_shared/base_rootfs/", "ws_shared/images/" + image + "/"} {
		for o := range c.ListObjects(ctx, "snapshots", minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if o.Err != nil {
				return o.Err
			}
			if !keep(o.Key) {
				continue
			}
			dst := filepath.Join(cfg.Cache, filepath.FromSlash(o.Key))
			if !strings.HasPrefix(dst, cfg.Cache+"/") {
				return fmt.Errorf("invalid object key")
			}
			opts := minio.GetObjectOptions{}
			if err := opts.SetMatchETag(o.ETag); err != nil {
				return err
			}
			if err := c.FGetObject(ctx, "snapshots", o.Key, dst, opts); err != nil {
				return err
			}
			files = append(files, o.Key)
		}
	}
	keys := map[string]bool{}
	for _, rev := range []string{"base", snapshot} {
		for _, name := range []string{"snap_file", "info_file", "recipe_file"} {
			if _, err := os.Stat(filepath.Join(cfg.Cache, rev, name)); err != nil {
				return err
			}
		}
		b, err := os.ReadFile(filepath.Join(cfg.Cache, rev, "recipe_file"))
		if err != nil {
			return err
		}
		part, err := recipeKeys(b)
		if err != nil {
			return err
		}
		for k := range part {
			keys[k] = true
		}
	}
	base := filepath.Join(cfg.Cache, snapshot, "working_set_pages_content_private")
	data, err := os.ReadFile(base + zstdstreams.ManifestSuffix)
	if err != nil {
		return err
	}
	m, err := zstdstreams.ParseManifest(data)
	if err != nil {
		return err
	}
	st, err := os.Stat(base + zstdstreams.PayloadSuffix)
	if err != nil {
		return err
	}
	if m.Level != 3 || m.StreamCount != 8 || st.Size() != m.CompressedSize {
		return fmt.Errorf("WS layout/size mismatch")
	}
	g, downloadCtx := errgroup.WithContext(ctx)
	g.SetLimit(16)
	var done atomic.Int64
	for key := range keys {
		key := key
		if downloadCtx.Err() != nil {
			break
		}
		g.Go(func() error {
			if err := c.FGetObject(downloadCtx, "snapshots", key, filepath.Join(cfg.Cache, filepath.FromSlash(key)), minio.GetObjectOptions{}); err != nil {
				return err
			}
			if n := done.Add(1); n%10000 == 0 {
				fmt.Printf("LOCAL_CACHE_PROGRESS chunks=%d total=%d\n", n, len(keys))
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if done.Load() != int64(len(keys)) {
		return fmt.Errorf("incomplete local chunks")
	}
	f, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	result := struct {
		Cache    string                `json:"cache"`
		Endpoint string                `json:"source_endpoint"`
		Snapshot string                `json:"snapshot"`
		Files    []string              `json:"metadata_and_ws_files"`
		Chunks   int                   `json:"native_chunks"`
		Manifest *zstdstreams.Manifest `json:"manifest"`
		Boundary string                `json:"boundary"`
	}{cfg.Cache, cfg.Endpoint, snapshot, files, len(keys), m, "Same compressed WS/native bytes, all recipe-referenced chunks; no mem_file, raw WS or re-encoding. Runtime/local-zero-read and formal aliases still require verification."}
	return json.NewEncoder(f).Encode(result)
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
