// Content-only Figure13 accounting on the actual new stores. Run outside a
// measurement window. Recipe/index reads select objects but are not payload.
package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
	"golang.org/x/sync/errgroup"
)

type Row struct {
	System      string                `json:"system"`
	Profile     string                `json:"profile"`
	Snapshot    string                `json:"snapshot"`
	Endpoint    string                `json:"endpoint"`
	ChunkSize   int                   `json:"chunk_size"`
	Layout      string                `json:"ws_layout"`
	Payload     string                `json:"payload_key"`
	ManifestKey string                `json:"manifest_key"`
	Compressed  int64                 `json:"compressed_bytes"`
	WSPages     int                   `json:"working_set_pages"`
	RecipeBytes int                   `json:"recipe_bytes"`
	Manifest    *zstdstreams.Manifest `json:"manifest,omitempty"`
}
type Result struct {
	System       string   `json:"system"`
	Profile      string   `json:"profile"`
	Snapshot     string   `json:"snapshot"`
	Endpoint     string   `json:"endpoint"`
	Layout       string   `json:"layout"`
	PayloadBytes int64    `json:"compressed_payload_bytes"`
	Objects      int      `json:"unique_payload_objects"`
	Keys         []string `json:"payload_keys"`
}

func nativeKeys(recipe, index []byte, chunkSize int) ([]string, int, error) {
	if (chunkSize != 4096 && chunkSize != 131072) || len(recipe)%16 != 0 {
		return nil, 0, fmt.Errorf("invalid native granularity/recipe")
	}
	rows, err := csv.NewReader(bytes.NewReader(index)).ReadAll()
	if err != nil || len(rows) == 0 || len(rows[0]) != 1 || rows[0][0] != "pfn" {
		return nil, 0, fmt.Errorf("invalid PFN index")
	}
	seen, hashes := map[int]bool{}, map[string]bool{}
	for _, row := range rows[1:] {
		if len(row) != 1 {
			return nil, 0, fmt.Errorf("invalid PFN row")
		}
		pfn, err := strconv.Atoi(row[0])
		if err != nil || pfn < 0 || seen[pfn] {
			return nil, 0, fmt.Errorf("invalid/duplicate PFN")
		}
		seen[pfn] = true
		off := pfn * 4096 / chunkSize * 16
		if off < 0 || off+16 > len(recipe) {
			return nil, 0, fmt.Errorf("PFN beyond recipe")
		}
		h := hex.EncodeToString(recipe[off : off+16])
		hashes["_chunks_zstd_v1_l3/"+h[:2]+"/"+h] = true
	}
	keys := make([]string, 0, len(hashes))
	for k := range hashes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys, len(seen), nil
}

func read(ctx context.Context, c *minio.Client, key string) ([]byte, error) {
	r, err := c.GetObject(ctx, "snapshots", key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	input := flag.String("inventory", "", "verified 102-row new-layout inventory")
	output := flag.String("output", "", "new report path; never overwritten")
	flag.Parse()
	data, err := os.ReadFile(*input)
	must(err)
	var inv struct {
		RunID  string `json:"run_id"`
		Layout string `json:"layout"`
		Rows   []Row  `json:"rows"`
	}
	must(json.Unmarshal(data, &inv))
	if inv.Layout != zstdstreams.Layout || len(inv.Rows) != 102 {
		panic("full new inventory required")
	}
	if _, err := os.Stat(*output); !os.IsNotExist(err) {
		panic("report target exists or inaccessible")
	}
	ctx := context.Background()
	clients := map[string]*minio.Client{}
	sizes := map[string]int64{}
	var lock sync.Mutex
	results := make([]Result, 0, 102)
	seen := map[string]bool{}
	for _, row := range inv.Rows {
		identity := row.System + "/" + row.Profile
		if seen[identity] {
			panic("duplicate point")
		}
		seen[identity] = true
		c := clients[row.Endpoint]
		if c == nil {
			c, err = minio.New(row.Endpoint, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", "")})
			must(err)
			clients[row.Endpoint] = c
		}
		keys := []string{row.Payload}
		if row.Layout == "not-applicable" {
			if row.System != "chunks-128k-zstd3" && row.System != "pages-4k-zstd3" {
				panic("unexpected native row")
			}
			recipe, e := read(ctx, c, row.Snapshot+"/recipe_file")
			must(e)
			index, e := read(ctx, c, row.Snapshot+"/working_set_pages")
			must(e)
			var pages int
			keys, pages, e = nativeKeys(recipe, index, row.ChunkSize)
			must(e)
			if pages != row.WSPages || len(recipe) != row.RecipeBytes {
				panic("native input differs from inventory")
			}
		} else {
			if row.Layout != zstdstreams.Layout {
				panic("wrong coalesced layout")
			}
			meta, e := read(ctx, c, row.ManifestKey)
			must(e)
			m, e := zstdstreams.ParseManifest(meta)
			must(e)
			if !reflect.DeepEqual(m, row.Manifest) {
				panic("manifest differs from frozen inventory")
			}
		}
		group, getCtx := errgroup.WithContext(ctx)
		group.SetLimit(16)
		for _, key := range keys {
			key := key
			id := row.Endpoint + "/" + key
			lock.Lock()
			_, exists := sizes[id]
			lock.Unlock()
			if exists {
				continue
			}
			group.Go(func() error {
				stat, e := c.StatObject(getCtx, "snapshots", key, minio.StatObjectOptions{})
				if e != nil {
					return e
				}
				lock.Lock()
				sizes[id] = stat.Size
				lock.Unlock()
				return nil
			})
		}
		must(group.Wait())
		var total int64
		for _, k := range keys {
			total += sizes[row.Endpoint+"/"+k]
		}
		if row.Layout == zstdstreams.Layout && total != row.Compressed {
			panic("payload size differs")
		}
		results = append(results, Result{row.System, row.Profile, row.Snapshot, row.Endpoint, row.Layout, total, len(keys), keys})
		fmt.Printf("PAYLOAD_POINT %s %s bytes=%d objects=%d\n", row.System, row.Profile, total, len(keys))
	}
	report := map[string]any{"run_id": inv.RunID, "layout": inv.Layout, "generated_at": time.Now().UTC(),
		"method": "content-only: unique native objects for frozen WS PFNs, or one actual coalesced payload; recipe/manifest/index/lazy extras excluded", "rows": results, "object_sizes": sizes}
	f, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	must(err)
	err = json.NewEncoder(f).Encode(report)
	must(err)
	must(f.Close())
	fmt.Println("PAYLOAD_COMPLETE rows=102")
}
