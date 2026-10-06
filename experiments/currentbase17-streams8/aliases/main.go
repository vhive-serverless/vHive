// aliases materializes 60 distinct cold-cache identities per frozen revision.
// It only writes new, tagged revision prefixes in the new streams8 stores.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

type entry struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	ETag string `json:"etag"`
}
type inventoryRow struct {
	CorpusID string `json:"corpus_id"`
	Endpoint string `json:"endpoint"`
	Snapshot string `json:"snapshot"`
}
type alias struct {
	CorpusID string  `json:"corpus_id"`
	Endpoint string  `json:"endpoint"`
	Source   string  `json:"source_revision"`
	Target   string  `json:"alias_revision"`
	Slot     int     `json:"slot"`
	Entries  []entry `json:"source_entries"`
	missing  []entry
}
type inventory struct {
	RunID   string                      `json:"run_id"`
	Layout  string                      `json:"layout"`
	Rows    []inventoryRow              `json:"rows"`
	Entries map[string]map[string]entry `json:"alias_source_entries"`
}

func planAliases(inv inventory, tag string) ([]alias, error) {
	if inv.Layout != "streams8-v1" || len(inv.Rows) != 102 || !regexp.MustCompile(`^streams8-[a-z0-9-]+$`).MatchString(tag) {
		return nil, fmt.Errorf("actual streams8 inventory and new streams8-prefixed tag required")
	}
	seen := map[string]bool{}
	var aliases []alias
	for _, row := range inv.Rows {
		key := row.Endpoint + "/" + row.Snapshot
		if seen[key] {
			continue
		}
		seen[key] = true
		var files []entry
		for _, e := range inv.Entries[row.CorpusID] {
			if !strings.HasPrefix(e.Key, row.Snapshot+"/") {
				continue
			}
			name := path.Base(e.Key)
			if name == "mem_file" || name == "working_set_pages_content" || name == "working_set_pages_content_private" || strings.HasSuffix(name, ".zstd.frames") || strings.HasSuffix(name, ".zstd.json") {
				return nil, fmt.Errorf("forbidden alias source %s", e.Key)
			}
			files = append(files, e)
		}
		sort.Slice(files, func(i, j int) bool {
			if path.Base(files[i].Key) == "snap_file" {
				return false
			}
			if path.Base(files[j].Key) == "snap_file" {
				return true
			}
			return files[i].Key < files[j].Key
		})
		if len(files) == 0 || path.Base(files[len(files)-1].Key) != "snap_file" {
			return nil, fmt.Errorf("source missing snap_file")
		}
		for i := 0; i < 60; i++ {
			aliases = append(aliases, alias{CorpusID: row.CorpusID, Endpoint: row.Endpoint, Source: row.Snapshot,
				Target: fmt.Sprintf("%s-%s-%d", row.Snapshot, tag, i), Slot: i, Entries: files})
		}
	}
	if len(aliases) != 5100 {
		return nil, fmt.Errorf("expected 85 distinct revisions/5100 aliases, got %d", len(aliases))
	}
	return aliases, nil
}

func targetKey(a alias, e entry) string { return a.Target + strings.TrimPrefix(e.Key, a.Source) }
func inspect(ctx context.Context, c *minio.Client, a alias, e entry) (bool, error) {
	o, err := c.StatObject(ctx, "snapshots", targetKey(a, e), minio.StatObjectOptions{})
	if err != nil {
		if code := minio.ToErrorResponse(err).Code; code == "NoSuchKey" || code == "NotFound" {
			return false, nil
		}
		return false, err
	}
	if o.Size != e.Size || o.Metadata.Get("X-Amz-Meta-Streams8-Alias-Source") != e.Key ||
		o.Metadata.Get("X-Amz-Meta-Streams8-Alias-Etag") != e.ETag {
		return false, fmt.Errorf("existing alias object is not the exact owned copy: %s", targetKey(a, e))
	}
	return true, nil
}

func run() error {
	input := flag.String("inventory", "", "actual-layout inventory JSON")
	tag := flag.String("tag", "", "new streams8-prefixed alias tag")
	output := flag.String("report", "", "new report path")
	copyMode := flag.Bool("copy", false, "materialize new alias objects; default is read-only preflight")
	diskPath := flag.String("data-root", "", "actual backend streams8 data directory for capacity check")
	workers := flag.Int("workers", 8, "concurrent aliases")
	flag.Parse()
	if *workers < 1 || *workers > 32 || *output == "" {
		return fmt.Errorf("invalid workers/report")
	}
	if _, err := os.Stat(*output); !os.IsNotExist(err) {
		return fmt.Errorf("report exists/inaccessible")
	}
	f, err := os.Open(*input)
	if err != nil {
		return err
	}
	var inv inventory
	err = json.NewDecoder(f).Decode(&inv)
	f.Close()
	if err != nil {
		return err
	}
	aliases, err := planAliases(inv, *tag)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	clients := map[string]*minio.Client{}
	for _, a := range aliases {
		if clients[a.Endpoint] != nil {
			continue
		}
		c, err := minio.New(a.Endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
		if err != nil {
			return err
		}
		clients[a.Endpoint] = c
	}
	var missingBytes, missingObjects atomic.Int64
	g, scanCtx := errgroup.WithContext(ctx)
	g.SetLimit(*workers)
	for i := range aliases {
		i := i
		if scanCtx.Err() != nil {
			break
		}
		g.Go(func() error {
			a := &aliases[i]
			for _, e := range a.Entries {
				exists, err := inspect(scanCtx, clients[a.Endpoint], *a, e)
				if err != nil {
					return err
				}
				if !exists {
					a.missing = append(a.missing, e)
					missingBytes.Add(e.Size)
					missingObjects.Add(1)
				}
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
	required := missingBytes.Load() + 8192*missingObjects.Load() + (16 << 30)
	fmt.Printf("ALIASES_PREFLIGHT revisions=%d missing_objects=%d missing_bytes=%d required_with_reserve=%d\n", len(aliases), missingObjects.Load(), missingBytes.Load(), required)
	if *copyMode {
		if !strings.HasPrefix(*diskPath, "/mnt/snapshare-zstd-streaming/streams8/") || !strings.HasSuffix(*diskPath, "/"+inv.RunID) {
			return fmt.Errorf("explicit matching backend data root required")
		}
		var fs syscall.Statfs_t
		if err := syscall.Statfs(*diskPath, &fs); err != nil {
			return err
		}
		free := int64(fs.Bavail) * int64(fs.Bsize)
		if free < required {
			return fmt.Errorf("alias capacity insufficient: free=%d required=%d", free, required)
		}
		var done atomic.Int64
		g, copyCtx := errgroup.WithContext(ctx)
		g.SetLimit(*workers)
		for i := range aliases {
			i := i
			if copyCtx.Err() != nil {
				break
			}
			g.Go(func() error {
				a := aliases[i]
				c := clients[a.Endpoint]
				for _, e := range a.missing {
					_, err := c.CopyObject(copyCtx, minio.CopyDestOptions{Bucket: "snapshots", Object: targetKey(a, e), ReplaceMetadata: true,
						UserMetadata: map[string]string{"Streams8-Alias-Source": e.Key, "Streams8-Alias-Etag": e.ETag}},
						minio.CopySrcOptions{Bucket: "snapshots", Object: e.Key, MatchETag: e.ETag})
					if err != nil {
						return err
					}
					if ok, err := inspect(copyCtx, c, a, e); err != nil {
						return err
					} else if !ok {
						return fmt.Errorf("copy not visible")
					}
				}
				if n := done.Add(1); n%60 == 0 {
					fmt.Printf("ALIASES_PROGRESS completed=%d total=%d\n", n, len(aliases))
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
		if done.Load() != int64(len(aliases)) {
			return fmt.Errorf("incomplete alias batch")
		}
	}
	result := struct {
		RunID          string  `json:"run_id"`
		Layout         string  `json:"layout"`
		Tag            string  `json:"tag"`
		Materialized   bool    `json:"materialized"`
		Aliases        []alias `json:"aliases"`
		MissingBytes   int64   `json:"initial_missing_bytes"`
		MissingObjects int64   `json:"initial_missing_objects"`
	}{inv.RunID, inv.Layout, *tag, *copyMode, aliases, missingBytes.Load(), missingObjects.Load()}
	f, err = os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
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
