// copycorpus performs object-level copies of explicitly selected immutable
// prefixes. It does not clone MinIO's internal directory or mutate the source.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"golang.org/x/sync/errgroup"
)

const bucket = "snapshots"

type Job struct {
	ID          string   `json:"corpus_id"`
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Prefixes    []string `json:"prefixes"`
}
type Entry struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
	ETag string `json:"etag"`
}
type InventoryJob struct {
	Job
	Objects []Entry `json:"objects"`
	Bytes   int64   `json:"bytes"`
}
type Inventory struct {
	Jobs    []InventoryJob `json:"jobs"`
	Bytes   int64          `json:"bytes"`
	Created string         `json:"created"`
}

func client(endpoint string) (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
}

func keep(key string) bool {
	base := path.Base(key)
	if base == "mem_file" || base == "working_set_pages_content" || base == "working_set_pages_content_private" {
		return false
	}
	if strings.HasPrefix(base, "working_set_pages_content") &&
		(strings.HasSuffix(base, ".zstd.frames") || strings.HasSuffix(base, ".zstd.json") ||
			strings.HasSuffix(base, ".zstd.streams") || strings.HasSuffix(base, ".zstd.streams.json")) {
		return false
	}
	return true
}

func publish(path string, data any) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("refusing existing report %s", path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".copy-report-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	writeErr := enc.Encode(data)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), path)
}

func scan(ctx context.Context, jobs []Job) (*Inventory, error) {
	inventory := &Inventory{Created: time.Now().UTC().Format(time.RFC3339)}
	for _, job := range jobs {
		if job.Source == job.Destination {
			return nil, fmt.Errorf("source equals destination")
		}
		c, err := client(job.Source)
		if err != nil {
			return nil, err
		}
		out := InventoryJob{Job: job}
		seen := map[string]bool{}
		for _, prefix := range job.Prefixes {
			if prefix == "" || !strings.HasSuffix(prefix, "/") {
				return nil, fmt.Errorf("unbounded prefix %q", prefix)
			}
			for info := range c.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
				if info.Err != nil {
					return nil, info.Err
				}
				if !keep(info.Key) || seen[info.Key] {
					continue
				}
				seen[info.Key] = true
				out.Objects = append(out.Objects, Entry{info.Key, info.Size, info.ETag})
				out.Bytes += info.Size
			}
		}
		if len(out.Objects) == 0 {
			return nil, fmt.Errorf("empty source selection for %s", job.ID)
		}
		inventory.Jobs = append(inventory.Jobs, out)
		inventory.Bytes += out.Bytes
		fmt.Printf("COPY_PLAN corpus=%s objects=%d bytes=%d\n", job.ID, len(out.Objects), out.Bytes)
	}
	return inventory, nil
}

func copyJob(ctx context.Context, job InventoryJob, workers int) error {
	source, err := client(job.Source)
	if err != nil {
		return err
	}
	target, err := client(job.Destination)
	if err != nil {
		return err
	}
	exists, err := target.BucketExists(ctx, bucket)
	if err != nil {
		return err
	}
	if !exists {
		if err := target.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return err
		}
	}
	existing := map[string]bool{}
	for _, prefix := range job.Prefixes {
		for info := range target.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: true}) {
			if info.Err != nil {
				return info.Err
			}
			existing[info.Key] = true
		}
	}
	g, copyCtx := errgroup.WithContext(ctx)
	g.SetLimit(workers)
	var done, copiedBytes atomic.Int64
	for _, entry := range job.Objects {
		entry := entry
		if copyCtx.Err() != nil {
			break
		}
		g.Go(func() error {
			if existing[entry.Key] {
				info, err := target.StatObject(copyCtx, bucket, entry.Key, minio.StatObjectOptions{})
				if err != nil {
					return err
				}
				if info.Size != entry.Size || info.Metadata.Get("X-Amz-Meta-Streams8-Source-Etag") != entry.ETag ||
					info.Metadata.Get("X-Amz-Meta-Streams8-Source") != job.Source {
					return fmt.Errorf("existing object not owned/matched: %s", entry.Key)
				}
			} else {
				opts := minio.GetObjectOptions{}
				if err := opts.SetMatchETag(entry.ETag); err != nil {
					return err
				}
				r, err := source.GetObject(copyCtx, bucket, entry.Key, opts)
				if err != nil {
					return err
				}
				_, err = target.PutObject(copyCtx, bucket, entry.Key, io.LimitReader(r, entry.Size), entry.Size,
					minio.PutObjectOptions{UserMetadata: map[string]string{"Streams8-Source-Etag": entry.ETag, "Streams8-Source": job.Source}})
				closeErr := r.Close()
				if err != nil {
					return fmt.Errorf("copy %s: %w", entry.Key, err)
				}
				if closeErr != nil {
					return closeErr
				}
				copiedBytes.Add(entry.Size)
			}
			if n := done.Add(1); n%10000 == 0 {
				fmt.Printf("COPY_PROGRESS corpus=%s done=%d total=%d new_bytes=%d\n", job.ID, n, len(job.Objects), copiedBytes.Load())
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
	if done.Load() != int64(len(job.Objects)) {
		return fmt.Errorf("incomplete corpus %s", job.ID)
	}
	fmt.Printf("COPY_COMPLETE corpus=%s objects=%d new_bytes=%d\n", job.ID, done.Load(), copiedBytes.Load())
	return nil
}

func run() error {
	planPath := flag.String("plan", "", "configuration plan JSON (required for scan and copy)")
	inventoryPath := flag.String("inventory", "", "new scan report, or frozen report for --copy")
	copyMode := flag.Bool("copy", false, "execute copies in frozen inventory to new endpoints")
	workers := flag.Int("workers", 16, "concurrent object copies")
	only := flag.String("only", "", "optional exact corpus ID")
	flag.Parse()
	if *planPath == "" || *inventoryPath == "" || *workers < 1 {
		return fmt.Errorf("plan, inventory and positive workers required")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	b, err := os.ReadFile(*planPath)
	if err != nil {
		return err
	}
	var plan struct {
		Jobs []Job `json:"jobs"`
	}
	if err := json.Unmarshal(b, &plan); err != nil {
		return err
	}
	if len(plan.Jobs) != 18 {
		return fmt.Errorf("expected 18 corpus jobs")
	}
	if !*copyMode {
		inventory, err := scan(ctx, plan.Jobs)
		if err != nil {
			return err
		}
		return publish(*inventoryPath, inventory)
	}
	b, err = os.ReadFile(*inventoryPath)
	if err != nil {
		return err
	}
	var inventory Inventory
	if err := json.Unmarshal(b, &inventory); err != nil {
		return err
	}
	if len(inventory.Jobs) != len(plan.Jobs) {
		return fmt.Errorf("inventory/plan count differs")
	}
	for i, job := range inventory.Jobs {
		if !reflect.DeepEqual(job.Job, plan.Jobs[i]) {
			return fmt.Errorf("inventory/plan differs for %s", job.ID)
		}
	}
	selected := 0
	for _, job := range inventory.Jobs {
		if *only != "" && job.ID != *only {
			continue
		}
		selected++
		if err := copyJob(ctx, job, *workers); err != nil {
			return err
		}
	}
	if selected == 0 {
		return fmt.Errorf("no matching corpus")
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
