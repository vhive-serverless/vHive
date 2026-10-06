// Read-only S3 accounting for the fixed 17-workload, three-memory-tier corpus.
// Run from the existing vHive module to reuse its exact eight-stream Zstd encoder.
package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Workload struct {
	Profile  string `json:"profile"`
	Snapshot string `json:"snapshot"`
	Image    string `json:"image_inventory"`
	MiB      int    `json:"vm_mib"`
}
type Corpus struct {
	Tier   int
	Kind   string
	Client *minio.Client
}

var ctx = context.Background()

const bucket = "snapshots"

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func get(c Corpus, key string) []byte {
	o, e := c.Client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	must(e)
	defer o.Close()
	b, e := io.ReadAll(o)
	must(e)
	return b
}
func size(c Corpus, key string) int64 {
	o, e := c.Client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	must(e)
	return o.Size
}
func recipe(b []byte) []string {
	if len(b)%16 != 0 {
		panic("invalid recipe")
	}
	r := make([]string, len(b)/16)
	for i := range r {
		r[i] = hex.EncodeToString(b[i*16 : (i+1)*16])
	}
	return r
}
func pfns(b []byte) []int {
	rows, e := csv.NewReader(bytes.NewReader(b)).ReadAll()
	must(e)
	if len(rows) == 0 || rows[0][0] != "pfn" {
		panic("missing pfn header")
	}
	var out []int
	for _, r := range rows[1:] {
		n, e := strconv.Atoi(r[0])
		must(e)
		out = append(out, n)
	}
	return out
}
func rawWS(c Corpus, w Workload) []byte {
	m, e := zstdstreams.ParseManifest(get(c, w.Snapshot+"/working_set_pages_content.zstd.streams.json"))
	must(e)
	payload := get(c, w.Snapshot+"/working_set_pages_content.zstd.streams")
	out := make([]byte, m.RawSize)
	must(zstdstreams.Decode(ctx, m, func(ctx context.Context, off, n int64) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(payload[off : off+n])), nil
	}, out))
	return out
}
func encodedBytes(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	m, e := zstdstreams.EncodeTo(io.Discard, b, 3)
	must(e)
	return m.CompressedSize
}
func sum(m map[string]int64) int64 {
	var n int64
	for _, v := range m {
		n += v
	}
	return n
}

func main() {
	manifest := flag.String("workloads", "", "fixed manifest")
	host := flag.String("host", "10.0.1.2", "single-function backend")
	output := flag.String("output", "", "report path")
	portBases := flag.String("tierPortBases", "", "required JSON map of memory tier to port base, e.g. {\"512\":9560,\"2048\":9570,\"3072\":9580}")
	flag.Parse()
	b, e := os.ReadFile(*manifest)
	must(e)
	var conf struct {
		Workloads []Workload `json:"workloads"`
	}
	must(json.Unmarshal(b, &conf))
	if len(conf.Workloads) != 17 {
		panic("require 17 representative workloads")
	}
	kinds := []string{"full-128k", "full-4k", "no-image-4k", "partial-4k", "full-dedup-4k"}
	offsets := map[string]int{"full-128k": 1, "full-4k": 2, "full-dedup-4k": 3, "no-image-4k": 4, "partial-4k": 5}
	tiers := []int{512, 2048, 3072}
	ports := map[int]int{}
	must(json.Unmarshal([]byte(*portBases), &ports))
	for _, tier := range tiers {
		if ports[tier] < 1 || ports[tier] > 65529 {
			panic("invalid/missing tier port base")
		}
	}
	corp := map[string]map[int]Corpus{}
	for _, kind := range kinds {
		corp[kind] = map[int]Corpus{}
		for _, tier := range tiers {
			c, e := minio.New(fmt.Sprintf("%s:%d", *host, ports[tier]+offsets[kind]), &minio.Options{Creds: credentials.NewStaticV4("minio", "minio123", ""), Secure: false})
			must(e)
			corp[kind][tier] = Corpus{tier, kind, c}
		}
	}
	var denominator int64
	byTier := map[int][]Workload{}
	profiles := []string{}
	for _, w := range conf.Workloads {
		denominator += int64(w.MiB) * 1024 * 1024
		byTier[w.MiB] = append(byTier[w.MiB], w)
		profiles = append(profiles, w.Profile)
	}
	if denominator != int64(15.5*1024*1024*1024) {
		panic("unexpected VM sizes")
	}
	canonical := map[string][]string{}
	wsPFNs := map[string][]int{}
	for _, w := range conf.Workloads {
		canonical[w.Profile] = recipe(get(corp["full-dedup-4k"][w.MiB], w.Snapshot+"/recipe_file"))
		wsPFNs[w.Profile] = pfns(get(corp["full-4k"][w.MiB], w.Snapshot+"/working_set_pages"))
	}
	snapshotBytes := map[string]int64{}
	cacheBytes := map[string]int64{}
	audit := map[string]any{}
	for _, kind := range kinds {
		globalSnapshot := map[string]int64{}
		globalWS := map[string]int64{}
		var tierAudit []any
		chunk := 4096
		if kind == "full-128k" {
			chunk = 131072
		}
		for _, tier := range tiers {
			c := corp[kind][tier]
			wanted := map[string]bool{}
			wsWanted := map[string]bool{}
			for _, w := range byTier[tier] {
				r := recipe(get(c, w.Snapshot+"/recipe_file"))
				if len(r)*chunk != w.MiB*1024*1024 {
					panic("recipe/memory mismatch")
				}
				for _, h := range r {
					wanted[h] = true
				}
				for _, p := range wsPFNs[w.Profile] {
					wsWanted[r[p/(chunk/4096)]] = true
				}
			}
			found := map[string]bool{}
			foundWS := map[string]bool{}
			var scanned int64
			fmt.Fprintf(os.Stderr, "%s scan %s tier%d needed=%d\n", time.Now().Format(time.RFC3339), kind, tier, len(wanted))
			for o := range c.Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: "_chunks_zstd_v1_l3/", Recursive: true}) {
				must(o.Err)
				scanned++
				parts := strings.Split(o.Key, "/")
				if len(parts) != 3 {
					panic(o.Key)
				}
				h := parts[2]
				if wanted[h] {
					if old, ok := globalSnapshot[h]; ok && old != o.Size {
						panic("same identity different compressed size")
					}
					globalSnapshot[h] = o.Size
					found[h] = true
				}
				if wsWanted[h] {
					globalWS[h] = o.Size
					foundWS[h] = true
				}
			}
			if len(found) != len(wanted) || len(foundWS) != len(wsWanted) {
				panic(fmt.Sprintf("missing objects: %s/%d %d/%d WS %d/%d", kind, tier, len(found), len(wanted), len(foundWS), len(wsWanted)))
			}
			tierAudit = append(tierAudit, map[string]any{"tier_mib": tier, "snapshot_objects": len(found), "ws_objects": len(foundWS), "scanned": scanned})
		}
		snapshotBytes[kind] = sum(globalSnapshot)
		cacheBytes[kind] = sum(globalWS)
		audit[kind] = map[string]any{"tiers": tierAudit, "global_snapshot_objects": len(globalSnapshot), "global_ws_objects": len(globalWS), "snapshot_bytes": sum(globalSnapshot), "ws_chunk_bytes": sum(globalWS)}
		fmt.Fprintf(os.Stderr, "DONE %s global snapshot=%d WS=%d\n", kind, sum(globalSnapshot), sum(globalWS))
	}
	var fullWS int64
	private := map[string]int64{}
	var perWorkload []any
	var dedupRaw []byte
	seen := map[string]bool{}
	for _, w := range conf.Workloads {
		n := size(corp["full-4k"][w.MiB], w.Snapshot+"/working_set_pages_content.zstd.streams")
		fullWS += n
		row := map[string]any{"profile": w.Profile, "snapshot": w.Snapshot, "vm_mib": w.MiB, "full_ws_compressed_bytes": n}
		for _, kind := range []string{"no-image-4k", "partial-4k"} {
			v := size(corp[kind][w.MiB], w.Snapshot+"/working_set_pages_content_private.zstd.streams")
			private[kind] += v
			row[kind+"_private_ws_bytes"] = v
		}
		raw := rawWS(corp["full-4k"][w.MiB], w)
		ps := wsPFNs[w.Profile]
		if len(raw) != len(ps)*4096 {
			panic("raw WS/PFN size mismatch")
		}
		for i, p := range ps {
			page := raw[i*4096 : (i+1)*4096]
			h := canonical[w.Profile][p]
			hash := md5.Sum(page)
			if hex.EncodeToString(hash[:]) != h {
				panic("WS/canonical content mismatch")
			}
			if !seen[h] {
				dedupRaw = append(dedupRaw, page...)
				seen[h] = true
			}
		}
		perWorkload = append(perWorkload, row)
	}
	if len(seen) != audit["full-dedup-4k"].(map[string]any)["global_ws_objects"].(int) {
		panic("global WS coverage mismatch")
	}
	fullDedupWS := encodedBytes(dedupRaw)
	// Shared source selection is global across ALL memory-tier stores, not a sum
	// of independently deduplicated tier subtotals. Preserve source/PFN order.
	sharedBytes := map[string]int64{}
	sharedAudit := map[string]any{}
	for _, kind := range []string{"no-image-4k", "partial-4k"} {
		wanted := map[string]bool{}
		imageTiers := map[string]map[int]bool{}
		for _, w := range conf.Workloads {
			c := corp[kind][w.MiB]
			priv := map[int]bool{}
			for _, p := range pfns(get(c, w.Snapshot+"/working_set_pages_index_private")) {
				priv[p] = true
			}
			for _, p := range wsPFNs[w.Profile] {
				if !priv[p] {
					wanted[canonical[w.Profile][p]] = true
				}
			}
			if kind == "partial-4k" {
				if imageTiers[w.Image] == nil {
					imageTiers[w.Image] = map[int]bool{}
				}
				imageTiers[w.Image][w.MiB] = true
			}
		}
		type Source struct {
			tier int
			key  string
		}
		var sources []Source
		for _, tier := range tiers {
			sources = append(sources, Source{tier, "ws_shared/base_rootfs"})
		}
		names := []string{}
		for name := range imageTiers {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			for _, tier := range tiers {
				if imageTiers[name][tier] {
					sources = append(sources, Source{tier, "ws_shared/images/" + name})
				}
			}
		}
		emitted := map[string]bool{}
		var details []any
		for _, src := range sources {
			c := corp[kind][src.tier]
			raw := get(c, src.key+"/content")
			rows, e := csv.NewReader(bytes.NewReader(get(c, src.key+"/index"))).ReadAll()
			must(e)
			if len(rows) == 0 || rows[0][0] != "hash" || (len(rows)-1)*4096 != len(raw) {
				panic("shared source index size")
			}
			var selected []byte
			for i, r := range rows[1:] {
				h := r[0]
				if wanted[h] && !emitted[h] {
					selected = append(selected, raw[i*4096:(i+1)*4096]...)
					emitted[h] = true
				}
			}
			n := encodedBytes(selected)
			sharedBytes[kind] += n
			details = append(details, map[string]any{"tier_mib": src.tier, "source": src.key, "original_pages": len(rows) - 1, "selected_pages": len(selected) / 4096, "compressed_bytes": n})
		}
		if len(emitted) != len(wanted) {
			panic(fmt.Sprintf("shared coverage %s %d/%d", kind, len(emitted), len(wanted)))
		}
		sharedAudit[kind] = map[string]any{"unique_selected_pages": len(emitted), "compressed_bytes": sharedBytes[kind], "sources": details}
	}
	var systems []any
	for _, spec := range []struct {
		name, kind string
		ws, cache  int64
	}{
		{"Chunks", "full-128k", 0, cacheBytes["full-128k"]}, {"Pages", "full-4k", 0, cacheBytes["full-4k"]},
		{"WS", "full-4k", fullWS, fullWS}, {"No-image", "no-image-4k", private["no-image-4k"] + sharedBytes["no-image-4k"], private["no-image-4k"] + sharedBytes["no-image-4k"]},
		{"SplitSnap", "partial-4k", private["partial-4k"] + sharedBytes["partial-4k"], private["partial-4k"] + sharedBytes["partial-4k"]},
		{"Full Dedup", "full-dedup-4k", fullDedupWS, fullDedupWS},
	} {
		snap := snapshotBytes[spec.kind]
		systems = append(systems, map[string]any{"system": spec.name, "snapshot_bytes": snap, "working_set_storage_bytes": spec.ws, "total_storage_bytes": snap + spec.ws, "active_cache_bytes": spec.cache, "storage_normalized_to_full": float64(snap+spec.ws) / float64(denominator), "cache_normalized_to_ws": float64(spec.cache) / float64(fullWS)})
	}
	report := map[string]any{"generated_at": time.Now().Format(time.RFC3339), "semantic_revisions": 17, "profiles": profiles, "raw_full_snapshot_bytes": denominator, "systems": systems, "corpora": audit, "per_workload": perWorkload, "shared_sources": sharedAudit,
		"method":                      "Static content-only accounting over 17 representative single-workload snapshots; global object identity union across three memory-tier endpoints; no trace invocation weighting or node multiplier",
		"layout":                      zstdstreams.Layout,
		"compression":                 "Zstd-3; independent 128-KiB/4-KiB objects; coalesced WS eight long streams; required shared source content globally deduplicated and re-encoded in deterministic source order",
		"persistent_metadata":         "Recipe/info/snap/index/manifest and MinIO internals excluded, matching previous payload-only reports",
		"full_dedup_boundary":         "Canonical globally unique snapshot page store plus ideal coalesced global WS union in manifest/PFN first-occurrence order; offline assembly and temporary footprint excluded",
		"full_dedup_coalesced_oracle": map[string]any{"working_set_unique_hashes": len(seen), "raw_bytes": len(dedupRaw), "compressed_bytes": fullDedupWS}}
	data, e := json.MarshalIndent(report, "", "  ")
	must(e)
	must(os.WriteFile(*output, append(data, '\n'), 0644))
	fmt.Fprintln(os.Stderr, "ANALYSIS_COMPLETE", *output)
}
