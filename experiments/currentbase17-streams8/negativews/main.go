// negativews creates one explicitly invalid AES revision in the new corpus.
// Only its WS payload is omitted; canonical inputs and services are untouched.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func run() error {
	planPath := flag.String("plan", "", "new corpus plan")
	report := flag.String("report", "", "new negative-fixture receipt")
	flag.Parse()
	if _, err := os.Stat(*report); !os.IsNotExist(err) {
		return fmt.Errorf("report exists/inaccessible")
	}
	var plan struct {
		RunID     string `json:"run_id"`
		Backend   string `json:"backend"`
		Workloads struct {
			Rows []struct {
				Profile   string `json:"profile"`
				Snapshot  string `json:"snapshot"`
				Overrides map[string]struct {
					Port int    `json:"port"`
					ID   string `json:"corpus_id"`
				} `json:"corpus_overrides"`
			} `json:"workloads"`
		} `json:"workloads"`
	}
	b, err := os.ReadFile(*planPath)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(b, &plan); err != nil {
		return err
	}
	var snapshot, endpoint string
	for _, w := range plan.Workloads.Rows {
		if w.Profile == "aes-go-45000-45450" {
			o := w.Overrides["splitsnap-zstd3"]
			if !strings.HasSuffix(o.ID, "-streams8-"+plan.RunID) {
				return fmt.Errorf("not new corpus")
			}
			snapshot = w.Snapshot
			endpoint = fmt.Sprintf("%s:%d", plan.Backend, o.Port)
		}
	}
	if snapshot == "" {
		return fmt.Errorf("missing AES")
	}
	target := snapshot + "-streams8-missingws-" + plan.RunID
	ctx := context.Background()
	c, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
	if err != nil {
		return err
	}
	for o := range c.ListObjects(ctx, "snapshots", minio.ListObjectsOptions{Prefix: target + "/", Recursive: true}) {
		if o.Err != nil {
			return o.Err
		}
		return fmt.Errorf("negative target already exists; inspect, do not overwrite")
	}
	const name = "working_set_pages_content_private.zstd.streams"
	if _, err = c.StatObject(ctx, "snapshots", snapshot+"/"+name, minio.StatObjectOptions{}); err != nil {
		return err
	}
	var copied []string
	var snap *minio.ObjectInfo
	copyOne := func(o minio.ObjectInfo) error {
		key := target + strings.TrimPrefix(o.Key, snapshot)
		_, err := c.CopyObject(ctx, minio.CopyDestOptions{Bucket: "snapshots", Object: key, ReplaceMetadata: true,
			UserMetadata: map[string]string{"Streams8-Negative-Source": o.Key}}, minio.CopySrcOptions{Bucket: "snapshots", Object: o.Key, MatchETag: o.ETag})
		if err == nil {
			copied = append(copied, key)
		}
		return err
	}
	for o := range c.ListObjects(ctx, "snapshots", minio.ListObjectsOptions{Prefix: snapshot + "/", Recursive: true}) {
		if o.Err != nil {
			return o.Err
		}
		if o.Key == snapshot+"/"+name {
			continue
		}
		if o.Key == snapshot+"/snap_file" {
			v := o
			snap = &v
			continue
		}
		if err := copyOne(o); err != nil {
			return err
		}
	}
	if snap == nil {
		return fmt.Errorf("missing snap_file")
	}
	if err := copyOne(*snap); err != nil {
		return err
	}
	f, err := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(map[string]any{"purpose": "deliberately missing required WS payload; never a formal alias", "source": snapshot, "target": target, "endpoint": endpoint, "omitted": target + "/" + name, "copied": copied})
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
