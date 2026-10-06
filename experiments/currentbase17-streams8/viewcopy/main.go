// viewcopy preserves the accepted oracle boundary: reuse the new SplitSnap
// private representation, never re-encode a different monolithic Full Dedup WS.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
)

type job struct {
	ID          string `json:"corpus_id"`
	Kind        string `json:"kind"`
	Tier        int    `json:"tier"`
	Destination string `json:"destination"`
	Workloads   struct {
		Rows []struct {
			Snapshot string `json:"snapshot"`
		} `json:"workloads"`
	} `json:"workloads"`
}
type row struct {
	Snapshot    string                `json:"snapshot"`
	Source      string                `json:"source"`
	Destination string                `json:"destination"`
	Manifest    *zstdstreams.Manifest `json:"manifest"`
}

func read(ctx context.Context, c *minio.Client, key string) ([]byte, error) {
	r, e := c.GetObject(ctx, "snapshots", key, minio.GetObjectOptions{})
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return io.ReadAll(r)
}
func client(endpoint string) (*minio.Client, error) {
	return minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
}

func run() error {
	planPath := flag.String("plan", "", "frozen plan")
	report := flag.String("report", "", "new copy receipt (not oracle validation)")
	flag.Parse()
	if _, e := os.Stat(*report); !os.IsNotExist(e) {
		return fmt.Errorf("report already exists or inaccessible")
	}
	data, e := os.ReadFile(*planPath)
	if e != nil {
		return e
	}
	var plan struct {
		Jobs []job `json:"jobs"`
	}
	if e = json.Unmarshal(data, &plan); e != nil {
		return e
	}
	if len(plan.Jobs) != 18 {
		return fmt.Errorf("expected 18 stores")
	}
	ctx := context.Background()
	var rows []row
	for _, j := range plan.Jobs {
		if j.Kind != "full-dedup-oracle" {
			continue
		}
		var sourceEndpoint string
		for _, s := range plan.Jobs {
			if s.Tier == j.Tier && s.Kind == "partial-4k" {
				sourceEndpoint = s.Destination
			}
		}
		if sourceEndpoint == "" || sourceEndpoint == j.Destination {
			return fmt.Errorf("invalid view source")
		}
		source, e := client(sourceEndpoint)
		if e != nil {
			return e
		}
		dest, e := client(j.Destination)
		if e != nil {
			return e
		}
		for _, w := range j.Workloads.Rows {
			base := w.Snapshot + "/working_set_pages_content_private"
			metadata, e := read(ctx, source, base+zstdstreams.ManifestSuffix)
			if e != nil {
				return e
			}
			manifest, e := zstdstreams.ParseManifest(metadata)
			if e != nil {
				return e
			}
			for _, key := range []string{w.Snapshot + "/working_set_pages_index_private", base + zstdstreams.PayloadSuffix, base + zstdstreams.ManifestSuffix} {
				info, e := source.StatObject(ctx, "snapshots", key, minio.StatObjectOptions{})
				if e != nil {
					return e
				}
				existing, e := dest.StatObject(ctx, "snapshots", key, minio.StatObjectOptions{})
				if e == nil {
					if existing.Size != info.Size {
						return fmt.Errorf("view size differs: %s", key)
					}
					if key == base+zstdstreams.PayloadSuffix {
						if existing.Metadata.Get("X-Amz-Meta-Streams8-View-Source") != sourceEndpoint || existing.Metadata.Get("X-Amz-Meta-Streams8-View-Etag") != info.ETag {
							return fmt.Errorf("existing view payload not owned/matched")
						}
					} else {
						a, e := read(ctx, source, key)
						if e != nil {
							return e
						}
						b, e := read(ctx, dest, key)
						if e != nil {
							return e
						}
						if !bytes.Equal(a, b) {
							return fmt.Errorf("existing view metadata differs: %s", key)
						}
					}
					continue
				}
				if code := minio.ToErrorResponse(e).Code; code != "NoSuchKey" && code != "NotFound" {
					return e
				}
				opts := minio.GetObjectOptions{}
				if e = opts.SetMatchETag(info.ETag); e != nil {
					return e
				}
				r, e := source.GetObject(ctx, "snapshots", key, opts)
				if e != nil {
					return e
				}
				_, e = dest.PutObject(ctx, "snapshots", key, r, info.Size, minio.PutObjectOptions{UserMetadata: map[string]string{"Streams8-View-Source": sourceEndpoint, "Streams8-View-Etag": info.ETag}})
				closeErr := r.Close()
				if e != nil {
					return e
				}
				if closeErr != nil {
					return closeErr
				}
			}
			rows = append(rows, row{w.Snapshot, sourceEndpoint, j.Destination, manifest})
			fmt.Printf("VIEW_COPIED snapshot=%s streams=%d bytes=%d\n", w.Snapshot, manifest.StreamCount, manifest.CompressedSize)
		}
	}
	if len(rows) != 17 {
		return fmt.Errorf("expected 17 views, got %d", len(rows))
	}
	f, e := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(struct {
		Rows                []row  `json:"rows"`
		CanonicalValidation string `json:"canonical_validation"`
	}{rows, "pending; run materializer against canonical stores"})
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
