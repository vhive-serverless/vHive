// capacity accounts for the selected corpus, new WS representation and aliases.
// It reads old manifests only; it never writes to MinIO.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type entry struct {
	Key  string `json:"key"`
	Size int64  `json:"size"`
}
type job struct {
	ID        string `json:"corpus_id"`
	Source    string `json:"source"`
	Kind      string `json:"kind"`
	Mode      string `json:"transcode_mode"`
	Workloads struct {
		Rows []struct {
			Snapshot string `json:"snapshot"`
		} `json:"workloads"`
	} `json:"workloads"`
}
type store struct {
	ID      string  `json:"corpus_id"`
	Objects []entry `json:"objects"`
	Bytes   int64   `json:"bytes"`
}
type budget struct {
	SourceBytes              int64  `json:"source_bytes"`
	SourceObjects            int64  `json:"source_objects"`
	ObjectAllowance          int64  `json:"object_allowance_bytes"`
	NewWSRawBound            int64  `json:"new_ws_raw_bound_bytes"`
	LegacyWSBytes            int64  `json:"legacy_ws_compressed_bytes_reference_only"`
	AliasMetadataBytes       int64  `json:"alias_metadata_bytes"`
	AllAliasesLegacyEstimate int64  `json:"all_aliases_legacy_estimate_bytes"`
	AllAliasesRawBound       int64  `json:"all_aliases_raw_bound_bytes"`
	LargestAliasBatchBound   int64  `json:"largest_alias_batch_bound_bytes"`
	FoundationRequired       int64  `json:"foundation_required_bytes"`
	StagedRequired           int64  `json:"staged_required_bytes"`
	AllRequiredBound         int64  `json:"all_required_bound_bytes"`
	AliasRevisions           int    `json:"alias_revisions"`
	Note                     string `json:"note"`
}

func run() error {
	planPath := flag.String("plan", "", "corpus plan")
	inventoryPath := flag.String("inventory", "", "frozen source inventory")
	report := flag.String("report", "", "new capacity report")
	flag.Parse()
	var p struct {
		Jobs []job `json:"jobs"`
	}
	var inventory struct {
		Jobs []store `json:"jobs"`
	}
	read := func(path string, dst any) error {
		f, e := os.Open(path)
		if e != nil {
			return e
		}
		defer f.Close()
		return json.NewDecoder(f).Decode(dst)
	}
	if e := read(*planPath, &p); e != nil {
		return e
	}
	if e := read(*inventoryPath, &inventory); e != nil {
		return e
	}
	if len(p.Jobs) != 18 || len(inventory.Jobs) != 18 {
		return fmt.Errorf("expected 18 stores")
	}
	b := budget{Note: "8 KiB/object is a planning allowance, not a physical upper bound. New WS raw + 6.25% + 1 MiB/object is a conservative encoding reserve. Alias estimates exclude global native/shared namespaces. Actual new WS sizes and disk allocation must be checked before aliases; no raw bound is reported as measured compression."}
	for i, j := range p.Jobs {
		s := inventory.Jobs[i]
		if s.ID != j.ID {
			return fmt.Errorf("inventory mismatch")
		}
		b.SourceBytes += s.Bytes
		b.SourceObjects += int64(len(s.Objects))
		if j.Kind == "full-dedup-4k" {
			continue
		}
		metadata := map[string]int64{}
		counts := map[string]int64{}
		for _, w := range j.Workloads.Rows {
			metadata[w.Snapshot] = 0
		}
		for _, o := range s.Objects {
			revision, _, ok := strings.Cut(o.Key, "/")
			if _, found := metadata[revision]; ok && found {
				metadata[revision] += o.Size
				counts[revision]++
			}
		}
		c, e := minio.New(j.Source, &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
		if e != nil {
			return e
		}
		for _, w := range j.Workloads.Rows {
			var rawBound, compressed int64
			if j.Mode == "full" || j.Mode == "private" || j.Mode == "view" {
				base := "working_set_pages_content_private"
				if j.Mode == "full" {
					base = "working_set_pages_content"
				}
				r, e := c.GetObject(context.Background(), "snapshots", w.Snapshot+"/"+base+".zstd.json", minio.GetObjectOptions{})
				if e != nil {
					return e
				}
				var m struct {
					RawSize        int64 `json:"raw_size"`
					CompressedSize int64 `json:"compressed_size"`
				}
				e = json.NewDecoder(io.LimitReader(r, 4<<20)).Decode(&m)
				r.Close()
				if e != nil {
					return fmt.Errorf("%s: %w", w.Snapshot, e)
				}
				if m.RawSize <= 0 || m.CompressedSize <= 0 {
					return fmt.Errorf("invalid WS sizes")
				}
				rawBound = m.RawSize + m.RawSize/16 + (1 << 20)
				compressed = m.CompressedSize
				b.NewWSRawBound += rawBound
				b.LegacyWSBytes += compressed
				counts[w.Snapshot] += 2
			}
			base := metadata[w.Snapshot] + 8192*counts[w.Snapshot]
			batch := 60 * (base + rawBound)
			b.AliasMetadataBytes += 60 * base
			b.AllAliasesLegacyEstimate += 60 * (base + compressed)
			b.AllAliasesRawBound += batch
			if batch > b.LargestAliasBatchBound {
				b.LargestAliasBatchBound = batch
			}
			b.AliasRevisions += 60
		}
	}
	b.ObjectAllowance = b.SourceObjects * 8192
	b.FoundationRequired = b.SourceBytes + b.ObjectAllowance + b.NewWSRawBound + (16 << 30)
	b.StagedRequired = b.FoundationRequired + b.LargestAliasBatchBound
	b.AllRequiredBound = b.FoundationRequired + b.AllAliasesRawBound
	f, e := os.OpenFile(*report, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if e != nil {
		return e
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if e = enc.Encode(b); e != nil {
		return e
	}
	return json.NewEncoder(os.Stdout).Encode(b)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
