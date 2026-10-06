// materialize_full_dedup_oracle_ws constructs an explicitly idealized,
// revision-local transfer view from a globally content-addressed page store.
//
// The canonical storage representation remains one object per unique raw page
// hash.  The generated coalesced object is an ephemeral evaluation artifact:
// its creation latency and bytes must not be counted as part of Full Dedup's
// persistent footprint.  It exists only to measure the best-case transfer and
// restore path after a hypothetical zero-cost gather operation.
package main

import (
	"bytes"
	"context"
	"crypto/md5" // #nosec G501 -- MD5 is the existing snapshot content identity, not a cryptographic claim.
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting/zstdstreams"
	"github.com/vhive-serverless/vhive/storage"
)

const pageSize = 4096

type report struct {
	Method                    string `json:"method"`
	TimingBoundary            string `json:"timing_boundary"`
	Snapshot                  string `json:"snapshot"`
	PageSize                  int    `json:"page_size"`
	RecipeReferences          int    `json:"recipe_references"`
	RecipeUniqueHashes        int    `json:"recipe_unique_hashes"`
	CanonicalPageObjects      int    `json:"canonical_page_objects"`
	CanonicalUnreferenced     int    `json:"canonical_unreferenced_objects"`
	VerifiedUniquePageObjects int    `json:"verified_unique_page_objects"`
	WorkingSetPages           int    `json:"working_set_pages"`
	WorkingSetUniqueHashes    int    `json:"working_set_unique_hashes"`
	RawBytes                  int    `json:"raw_bytes"`
	CompressedBytes           int    `json:"compressed_bytes"`
	CompressionRatio          string `json:"compression_ratio"`
	ZstdLevel                 int    `json:"zstd_level"`
	WSLayout                  string `json:"ws_layout"`
	ZstdStreams               int    `json:"zstd_streams"`
	PayloadObject             string `json:"payload_object"`
	ManifestObject            string `json:"manifest_object"`
	AssemblyElapsedUS         int64  `json:"assembly_elapsed_us_excluded"`
	ReferenceFramework        string `json:"reference_framework,omitempty"`
	ReferenceEndpoint         string `json:"reference_endpoint,omitempty"`
	ReferenceImageKey         string `json:"reference_image_key,omitempty"`
	SharedBaseHashes          int    `json:"shared_base_hashes,omitempty"`
	SharedImageHashes         int    `json:"shared_image_hashes,omitempty"`
	PrivateWorkingSetPages    int    `json:"private_working_set_pages,omitempty"`
	CoveredWorkingSetPages    int    `json:"covered_working_set_pages,omitempty"`
}

type batchWorkload struct {
	Profile        string `json:"profile"`
	Snapshot       string `json:"snapshot"`
	ImageInventory string `json:"image_inventory"`
}

type batchWorkloadFile struct {
	Workloads []batchWorkload `json:"workloads"`
}

type batchRevisionReport struct {
	Profile                  string `json:"profile"`
	Snapshot                 string `json:"snapshot"`
	ReferenceImageKey        string `json:"reference_image_key"`
	RecipeReferences         int    `json:"recipe_references"`
	RecipeUniqueHashes       int    `json:"recipe_unique_hashes"`
	WorkingSetPages          int    `json:"working_set_pages"`
	WorkingSetUniqueHashes   int    `json:"working_set_unique_hashes"`
	SharedBaseHashes         int    `json:"shared_base_hashes"`
	SharedImageHashes        int    `json:"shared_image_hashes"`
	PrivateWorkingSetPages   int    `json:"private_working_set_pages"`
	CoveredWorkingSetPages   int    `json:"covered_working_set_pages"`
	PrivateRawBytes          int    `json:"private_raw_bytes"`
	PrivateCompressedBytes   int    `json:"private_compressed_bytes"`
	PrivateCompressedStreams int    `json:"private_compressed_streams"`
}

type batchReport struct {
	Method                    string                `json:"method"`
	TimingBoundary            string                `json:"timing_boundary"`
	VerificationScope         string                `json:"verification_scope"`
	ReferenceEndpoint         string                `json:"reference_endpoint"`
	SemanticRevisions         int                   `json:"semantic_revisions"`
	CanonicalPageObjects      int                   `json:"canonical_page_objects"`
	VerifiedUniquePageObjects int                   `json:"verified_unique_page_objects"`
	RecipeReferences          int                   `json:"recipe_references"`
	RecipeUniqueHashes        int                   `json:"recipe_unique_hashes"`
	WorkingSetReferences      int                   `json:"working_set_references"`
	WorkingSetUniqueHashes    int                   `json:"working_set_unique_hashes"`
	GlobalWorkingSetRawBytes  int64                 `json:"global_working_set_raw_bytes"`
	GlobalWorkingSetZstdBytes int64                 `json:"global_working_set_zstd_bytes"`
	GlobalWorkingSetStreams   int                   `json:"global_working_set_zstd_streams"`
	GlobalWorkingSetOrder     string                `json:"global_working_set_order"`
	HashOrderZstdBytes        int64                 `json:"hash_order_zstd_bytes,omitempty"`
	HashOrderZstdStreams      int                   `json:"hash_order_zstd_streams,omitempty"`
	ZstdLevel                 int                   `json:"zstd_level"`
	WSLayout                  string                `json:"ws_layout"`
	Revisions                 []batchRevisionReport `json:"revisions"`
}

func estimateGlobalWorkingSet(
	store *storage.MinioStorage,
	workloads []batchWorkload,
	workers int,
	zstdLevel int,
	wsLayout string,
) (batchReport, error) {
	allWorkingSetHashes := make(map[string]struct{})
	firstOccurrenceOrder := make([]string, 0)
	workingSetReferences := 0
	for _, workload := range workloads {
		recipeData, err := store.DownloadObject(workload.Snapshot + "/recipe_file")
		if err != nil {
			return batchReport{}, fmt.Errorf("download %s recipe: %w", workload.Snapshot, err)
		}
		recipeHashes, err := parseRecipe(recipeData)
		if err != nil {
			return batchReport{}, fmt.Errorf("parse %s recipe: %w", workload.Snapshot, err)
		}
		wsData, err := store.DownloadObject(workload.Snapshot + "/working_set_pages")
		if err != nil {
			return batchReport{}, fmt.Errorf("download %s working set: %w", workload.Snapshot, err)
		}
		pfns, err := parseWorkingSetPFNs(wsData, len(recipeHashes))
		if err != nil {
			return batchReport{}, fmt.Errorf("parse %s working set: %w", workload.Snapshot, err)
		}
		for _, pfn := range pfns {
			hash := recipeHashes[pfn]
			if _, seen := allWorkingSetHashes[hash]; !seen {
				allWorkingSetHashes[hash] = struct{}{}
				firstOccurrenceOrder = append(firstOccurrenceOrder, hash)
			}
		}
		workingSetReferences += len(pfns)
	}

	ordered := sortedSetKeys(allWorkingSetHashes)
	pageByHash, err := verifyAndCollect(store, ordered, allWorkingSetHashes, workers)
	if err != nil {
		return batchReport{}, fmt.Errorf("download exact-content working-set union: %w", err)
	}
	raw := make([]byte, 0, len(firstOccurrenceOrder)*pageSize)
	for _, hash := range firstOccurrenceOrder {
		raw = append(raw, pageByHash[hash]...)
	}
	manifest, err := zstdstreams.EncodeTo(io.Discard, raw, zstdLevel)
	if err != nil {
		return batchReport{}, fmt.Errorf("compress global unique working-set union: %w", err)
	}
	hashOrderedRaw := make([]byte, 0, len(ordered)*pageSize)
	for _, hash := range ordered {
		hashOrderedRaw = append(hashOrderedRaw, pageByHash[hash]...)
	}
	hashOrderedManifest, err := zstdstreams.EncodeTo(io.Discard, hashOrderedRaw, zstdLevel)
	if err != nil {
		return batchReport{}, fmt.Errorf("compress hash-ordered working-set union: %w", err)
	}
	return batchReport{
		Method:                    "offline ideal Full Dedup cache accounting: globally union exact-content WS pages, retain deterministic first-occurrence order, and encode in coalesced Zstd streams; hash-order bytes are reported as an order-sensitivity check",
		TimingBoundary:            "read-only accounting; page gathering and compression are excluded from restore latency and the derived payload is not installed",
		VerificationScope:         "working-set-only",
		SemanticRevisions:         len(workloads),
		VerifiedUniquePageObjects: len(ordered),
		WorkingSetReferences:      workingSetReferences,
		WorkingSetUniqueHashes:    len(ordered),
		GlobalWorkingSetRawBytes:  int64(len(raw)),
		GlobalWorkingSetZstdBytes: manifest.CompressedSize,
		GlobalWorkingSetStreams:   len(manifest.Streams),
		GlobalWorkingSetOrder:     "first occurrence in frozen workload-manifest and PFN order; each unique WS page appears once",
		HashOrderZstdBytes:        hashOrderedManifest.CompressedSize,
		HashOrderZstdStreams:      len(hashOrderedManifest.Streams),
		ZstdLevel:                 zstdLevel,
		WSLayout:                  wsLayout,
	}, nil
}

type loadedBatchRevision struct {
	workload     batchWorkload
	recipeHashes []string
	pfns         []int
}

type pageResult struct {
	hash string
	data []byte
	err  error
}

func chunkObject(hash string) string {
	return fmt.Sprintf("_chunks/%s/%s", hash[:2], hash)
}

func parseRecipe(data []byte) ([]string, error) {
	if len(data)%md5.Size != 0 {
		return nil, fmt.Errorf("recipe size %d is not divisible by %d", len(data), md5.Size)
	}
	hashes := make([]string, 0, len(data)/md5.Size)
	for offset := 0; offset < len(data); offset += md5.Size {
		hashes = append(hashes, hex.EncodeToString(data[offset:offset+md5.Size]))
	}
	return hashes, nil
}

func parseWorkingSetPFNs(data []byte, recipePages int) ([]int, error) {
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse working-set CSV: %w", err)
	}
	if len(records) == 0 || len(records[0]) == 0 || records[0][0] != "pfn" {
		return nil, fmt.Errorf("working-set CSV must begin with a pfn header")
	}
	pfns := make([]int, 0, len(records)-1)
	seen := make(map[int]struct{}, len(records)-1)
	for row, record := range records[1:] {
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}
		pfn64, parseErr := strconv.ParseUint(strings.TrimSpace(record[0]), 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("parse pfn at row %d: %w", row+2, parseErr)
		}
		if pfn64 >= uint64(recipePages) {
			return nil, fmt.Errorf("pfn %d at row %d exceeds recipe page count %d", pfn64, row+2, recipePages)
		}
		pfn := int(pfn64)
		if _, duplicate := seen[pfn]; duplicate {
			return nil, fmt.Errorf("duplicate pfn %d at row %d", pfn, row+2)
		}
		seen[pfn] = struct{}{}
		pfns = append(pfns, pfn)
	}
	if len(pfns) == 0 {
		return nil, fmt.Errorf("working-set CSV has no pages")
	}
	return pfns, nil
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	result := make([]string, 0, len(seen))
	for value := range seen {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func canonicalHashes(objectKeys []string) ([]string, map[string]struct{}, error) {
	hashes := make([]string, 0, len(objectKeys))
	seen := make(map[string]struct{}, len(objectKeys))
	for _, key := range objectKeys {
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[0] != "_chunks" || len(parts[1]) != 2 || len(parts[2]) != md5.Size*2 {
			return nil, nil, fmt.Errorf("unexpected canonical page object key %q", key)
		}
		hash := parts[2]
		if _, err := hex.DecodeString(hash); err != nil {
			return nil, nil, fmt.Errorf("decode canonical page hash %q: %w", hash, err)
		}
		if key != chunkObject(hash) {
			return nil, nil, fmt.Errorf("canonical page object %q is not stored under its hash prefix", key)
		}
		if _, duplicate := seen[hash]; duplicate {
			return nil, nil, fmt.Errorf("canonical page hash %s appears more than once", hash)
		}
		seen[hash] = struct{}{}
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	return hashes, seen, nil
}

func verifyAndCollect(store *storage.MinioStorage, hashes []string, wanted map[string]struct{}, workers int) (map[string][]byte, error) {
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan string)
	results := make(chan pageResult, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for hash := range jobs {
				data, err := store.DownloadObject(chunkObject(hash))
				if err == nil && len(data) != pageSize {
					err = fmt.Errorf("page object %s has size %d, expected %d", hash, len(data), pageSize)
				}
				if err == nil {
					sum := md5.Sum(data) // #nosec G401 -- validates the existing raw-content object key.
					if actual := hex.EncodeToString(sum[:]); actual != hash {
						err = fmt.Errorf("page object key/content mismatch: key=%s content_md5=%s", hash, actual)
					}
				}
				if err == nil {
					if _, keep := wanted[hash]; !keep {
						data = nil
					}
				}
				results <- pageResult{hash: hash, data: data, err: err}
			}
		}()
	}
	go func() {
		for _, hash := range hashes {
			jobs <- hash
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	collected := make(map[string][]byte, len(wanted))
	var firstErr error
	for result := range results {
		if result.err != nil {
			if firstErr == nil {
				firstErr = result.err
			}
			continue
		}
		if result.data != nil {
			collected[result.hash] = result.data
		}
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if len(collected) != len(wanted) {
		return nil, fmt.Errorf("collected %d working-set hashes, expected %d", len(collected), len(wanted))
	}
	return collected, nil
}

func parseHashIndexedSource(index, content []byte) (map[string][]byte, error) {
	records, err := csv.NewReader(bytes.NewReader(index)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse hash-indexed source: %w", err)
	}
	if len(records) == 0 || len(records[0]) == 0 || records[0][0] != "hash" {
		return nil, fmt.Errorf("hash-indexed source must begin with a hash header")
	}
	hashes := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		if len(record) == 0 || strings.TrimSpace(record[0]) == "" {
			continue
		}
		hashes = append(hashes, strings.TrimSpace(record[0]))
	}
	if len(content) != len(hashes)*pageSize {
		return nil, fmt.Errorf("hash-indexed content size %d does not match %d pages", len(content), len(hashes))
	}
	result := make(map[string][]byte, len(hashes))
	for i, hash := range hashes {
		if len(hash) != md5.Size*2 {
			return nil, fmt.Errorf("invalid shared page hash %q", hash)
		}
		page := content[i*pageSize : (i+1)*pageSize]
		sum := md5.Sum(page) // #nosec G401 -- validates the existing raw-content identity.
		if actual := hex.EncodeToString(sum[:]); actual != hash {
			return nil, fmt.Errorf("shared page key/content mismatch: key=%s content_md5=%s", hash, actual)
		}
		if _, duplicate := result[hash]; duplicate {
			return nil, fmt.Errorf("duplicate shared page hash %s", hash)
		}
		result[hash] = append([]byte(nil), page...)
	}
	return result, nil
}

type splitSnapView struct {
	baseHashes       int
	imageHashes      int
	privatePages     int
	coveredPages     int
	privateRawBytes  int
	privateCompBytes int
	privateStreams   int
}

func validateSplitSnapView(
	reference *storage.MinioStorage,
	snapshot, imageKey string,
	recipeHashes []string,
	wsPFNs []int,
	canonical map[string][]byte,
	workers int,
) (splitSnapView, error) {
	load := func(key string) ([]byte, error) {
		data, err := reference.DownloadObject(key)
		if err != nil {
			return nil, fmt.Errorf("download SplitSnap reference object %s: %w", key, err)
		}
		return data, nil
	}
	baseIndex, err := load("ws_shared/base_rootfs/index")
	if err != nil {
		return splitSnapView{}, err
	}
	baseContent, err := load("ws_shared/base_rootfs/content")
	if err != nil {
		return splitSnapView{}, err
	}
	basePages, err := parseHashIndexedSource(baseIndex, baseContent)
	if err != nil {
		return splitSnapView{}, fmt.Errorf("validate SplitSnap base source: %w", err)
	}
	imagePrefix := fmt.Sprintf("ws_shared/images/%s", imageKey)
	imageIndex, err := load(imagePrefix + "/index")
	if err != nil {
		return splitSnapView{}, err
	}
	imageContent, err := load(imagePrefix + "/content")
	if err != nil {
		return splitSnapView{}, err
	}
	imagePages, err := parseHashIndexedSource(imageIndex, imageContent)
	if err != nil {
		return splitSnapView{}, fmt.Errorf("validate SplitSnap image source: %w", err)
	}

	privatePrefix := snapshot + "/working_set_pages_content_private"
	privateIndex, err := load(snapshot + "/working_set_pages_index_private")
	if err != nil {
		return splitSnapView{}, err
	}
	privatePFNs, err := parseWorkingSetPFNs(privateIndex, len(recipeHashes))
	if err != nil {
		return splitSnapView{}, fmt.Errorf("parse SplitSnap private index: %w", err)
	}

	manifestData, err := load(privatePrefix + ".zstd.streams.json")
	if err != nil {
		return splitSnapView{}, err
	}
	manifest, err := zstdstreams.ParseManifest(manifestData)
	if err != nil {
		return splitSnapView{}, fmt.Errorf("parse SplitSnap private WS manifest: %w", err)
	}
	payload, err := load(privatePrefix + ".zstd.streams")
	if err != nil {
		return splitSnapView{}, err
	}
	decoded := make([]byte, manifest.RawSize)
	openRange := func(ctx context.Context, offset, length int64) (io.ReadCloser, error) {
		if offset < 0 || length <= 0 || offset+length > int64(len(payload)) {
			return nil, fmt.Errorf("invalid private WS compressed range [%d,%d)", offset, offset+length)
		}
		return io.NopCloser(bytes.NewReader(payload[offset : offset+length])), nil
	}
	if err := zstdstreams.Decode(context.Background(), manifest, openRange, decoded); err != nil {
		return splitSnapView{}, fmt.Errorf("decode SplitSnap private WS: %w", err)
	}
	privateRaw := decoded
	if len(privateRaw) != len(privatePFNs)*pageSize {
		return splitSnapView{}, fmt.Errorf("private WS content size %d does not match %d PFNs", len(privateRaw), len(privatePFNs))
	}
	privateByPFN := make(map[int][]byte, len(privatePFNs))
	for i, pfn := range privatePFNs {
		page := privateRaw[i*pageSize : (i+1)*pageSize]
		sum := md5.Sum(page) // #nosec G401 -- validates the existing raw-content identity.
		hash := hex.EncodeToString(sum[:])
		if hash != recipeHashes[pfn] {
			return splitSnapView{}, fmt.Errorf("private PFN %d content hash %s does not match Full Dedup recipe %s", pfn, hash, recipeHashes[pfn])
		}
		privateByPFN[pfn] = append([]byte(nil), page...)
	}

	wsSet := make(map[int]struct{}, len(wsPFNs))
	covered := 0
	for _, pfn := range wsPFNs {
		wsSet[pfn] = struct{}{}
		hash := recipeHashes[pfn]
		var page []byte
		if private, ok := privateByPFN[pfn]; ok {
			page = private
		} else if base, ok := basePages[hash]; ok {
			page = base
		} else if image, ok := imagePages[hash]; ok {
			page = image
		} else {
			return splitSnapView{}, fmt.Errorf("working-set PFN %d hash %s is absent from every SplitSnap source", pfn, hash)
		}
		if !bytes.Equal(page, canonical[hash]) {
			return splitSnapView{}, fmt.Errorf("working-set PFN %d differs between SplitSnap view and Full Dedup canonical store", pfn)
		}
		covered++
	}
	for pfn := range privateByPFN {
		if _, ok := wsSet[pfn]; !ok {
			return splitSnapView{}, fmt.Errorf("private PFN %d is absent from the accepted working set", pfn)
		}
	}
	return splitSnapView{
		baseHashes:       len(basePages),
		imageHashes:      len(imagePages),
		privatePages:     len(privatePFNs),
		coveredPages:     covered,
		privateRawBytes:  len(privateRaw),
		privateCompBytes: len(payload),
		privateStreams:   len(manifest.Streams),
	}, nil
}

func parseBatchWorkloads(data []byte) ([]batchWorkload, error) {
	var config batchWorkloadFile
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse workload manifest: %w", err)
	}
	if len(config.Workloads) == 0 {
		return nil, fmt.Errorf("workload manifest is empty")
	}
	seenProfiles := make(map[string]struct{}, len(config.Workloads))
	seenSnapshots := make(map[string]struct{}, len(config.Workloads))
	for index, workload := range config.Workloads {
		if strings.TrimSpace(workload.Profile) == "" || strings.TrimSpace(workload.Snapshot) == "" || strings.TrimSpace(workload.ImageInventory) == "" {
			return nil, fmt.Errorf("workload %d requires profile, snapshot, and image_inventory", index)
		}
		if _, duplicate := seenProfiles[workload.Profile]; duplicate {
			return nil, fmt.Errorf("duplicate profile %s", workload.Profile)
		}
		if _, duplicate := seenSnapshots[workload.Snapshot]; duplicate {
			return nil, fmt.Errorf("duplicate semantic snapshot %s", workload.Snapshot)
		}
		seenProfiles[workload.Profile] = struct{}{}
		seenSnapshots[workload.Snapshot] = struct{}{}
	}
	return config.Workloads, nil
}

func sortedSetKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func writeJSONReport(value any, reportPath string) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if reportPath != "" {
		if err := os.WriteFile(reportPath, append(data, '\n'), 0644); err != nil {
			return fmt.Errorf("write report: %w", err)
		}
	}
	fmt.Println(string(data))
	return nil
}

func validateBatch(
	store, reference *storage.MinioStorage,
	workloads []batchWorkload,
	workers int,
	verificationScope string,
	zstdLevel int,
	wsLayout string,
) (batchReport, error) {
	loaded := make([]loadedBatchRevision, 0, len(workloads))
	allRecipeHashes := make(map[string]struct{})
	allWorkingSetHashes := make(map[string]struct{})
	recipeReferences := 0
	workingSetReferences := 0
	for _, workload := range workloads {
		recipeData, err := store.DownloadObject(workload.Snapshot + "/recipe_file")
		if err != nil {
			return batchReport{}, fmt.Errorf("download %s recipe: %w", workload.Snapshot, err)
		}
		recipeHashes, err := parseRecipe(recipeData)
		if err != nil {
			return batchReport{}, fmt.Errorf("parse %s recipe: %w", workload.Snapshot, err)
		}
		wsData, err := store.DownloadObject(workload.Snapshot + "/working_set_pages")
		if err != nil {
			return batchReport{}, fmt.Errorf("download %s working set: %w", workload.Snapshot, err)
		}
		pfns, err := parseWorkingSetPFNs(wsData, len(recipeHashes))
		if err != nil {
			return batchReport{}, fmt.Errorf("parse %s working set: %w", workload.Snapshot, err)
		}
		for _, hash := range recipeHashes {
			allRecipeHashes[hash] = struct{}{}
		}
		for _, pfn := range pfns {
			allWorkingSetHashes[recipeHashes[pfn]] = struct{}{}
		}
		recipeReferences += len(recipeHashes)
		workingSetReferences += len(pfns)
		loaded = append(loaded, loadedBatchRevision{workload: workload, recipeHashes: recipeHashes, pfns: pfns})
	}

	pageObjectKeys, err := store.ListObjects("_chunks/", true)
	if err != nil {
		return batchReport{}, fmt.Errorf("list canonical page store: %w", err)
	}
	allCanonicalHashes, canonicalSet, err := canonicalHashes(pageObjectKeys)
	if err != nil {
		return batchReport{}, err
	}
	for hash := range allRecipeHashes {
		if _, exists := canonicalSet[hash]; !exists {
			return batchReport{}, fmt.Errorf("recipe hash %s is absent from the canonical page store", hash)
		}
	}
	hashesToVerify := allCanonicalHashes
	if verificationScope == "working-set" {
		hashesToVerify = sortedSetKeys(allWorkingSetHashes)
	}
	pageByHash, err := verifyAndCollect(store, hashesToVerify, allWorkingSetHashes, workers)
	if err != nil {
		return batchReport{}, fmt.Errorf("verify canonical page store: %w", err)
	}

	revisions := make([]batchRevisionReport, 0, len(loaded))
	for _, revision := range loaded {
		view, err := validateSplitSnapView(
			reference,
			revision.workload.Snapshot,
			revision.workload.ImageInventory,
			revision.recipeHashes,
			revision.pfns,
			pageByHash,
			workers,
		)
		if err != nil {
			return batchReport{}, fmt.Errorf("validate %s against SplitSnap: %w", revision.workload.Snapshot, err)
		}
		wanted := make(map[string]struct{}, len(revision.pfns))
		for _, pfn := range revision.pfns {
			wanted[revision.recipeHashes[pfn]] = struct{}{}
		}
		revisions = append(revisions, batchRevisionReport{
			Profile:                  revision.workload.Profile,
			Snapshot:                 revision.workload.Snapshot,
			ReferenceImageKey:        revision.workload.ImageInventory,
			RecipeReferences:         len(revision.recipeHashes),
			RecipeUniqueHashes:       len(uniqueSorted(revision.recipeHashes)),
			WorkingSetPages:          len(revision.pfns),
			WorkingSetUniqueHashes:   len(wanted),
			SharedBaseHashes:         view.baseHashes,
			SharedImageHashes:        view.imageHashes,
			PrivateWorkingSetPages:   view.privatePages,
			CoveredWorkingSetPages:   view.coveredPages,
			PrivateRawBytes:          view.privateRawBytes,
			PrivateCompressedBytes:   view.privateCompBytes,
			PrivateCompressedStreams: view.privateStreams,
		})
	}

	// Figure 10's ideal Full Dedup cache holds every exact-content WS page
	// once, but uses the same coalesced eight-stream Zstd representation as the WS
	// and SplitSnap baselines. Hash order makes the accounting deterministic;
	// this payload is an offline oracle and is never installed as a restore
	// object or counted as additional persistent storage.
	orderedWorkingSetHashes := sortedSetKeys(allWorkingSetHashes)
	globalWorkingSet := make([]byte, 0, len(orderedWorkingSetHashes)*pageSize)
	for _, hash := range orderedWorkingSetHashes {
		page, ok := pageByHash[hash]
		if !ok {
			return batchReport{}, fmt.Errorf("working-set hash %s is absent from verified canonical pages", hash)
		}
		globalWorkingSet = append(globalWorkingSet, page...)
	}
	globalManifest, err := zstdstreams.EncodeTo(io.Discard, globalWorkingSet, zstdLevel)
	if err != nil {
		return batchReport{}, fmt.Errorf("compress global unique working-set union: %w", err)
	}

	return batchReport{
		Method:                    "unsafe Full Dedup oracle inside the SplitSnap framework across an explicit workload/revision manifest; persistent pages use global raw-content keys and every accepted SplitSnap transfer view is content-validated",
		TimingBoundary:            "SplitSnap-format transfer-view assembly and temporary footprint are excluded; measured restore starts after the view exists",
		VerificationScope:         verificationScope,
		SemanticRevisions:         len(revisions),
		CanonicalPageObjects:      len(allCanonicalHashes),
		VerifiedUniquePageObjects: len(hashesToVerify),
		RecipeReferences:          recipeReferences,
		RecipeUniqueHashes:        len(allRecipeHashes),
		WorkingSetReferences:      workingSetReferences,
		WorkingSetUniqueHashes:    len(allWorkingSetHashes),
		GlobalWorkingSetRawBytes:  int64(len(globalWorkingSet)),
		GlobalWorkingSetZstdBytes: globalManifest.CompressedSize,
		GlobalWorkingSetStreams:   len(globalManifest.Streams),
		GlobalWorkingSetOrder:     "ascending exact-content MD5 hash; each unique WS page appears once",
		ZstdLevel:                 zstdLevel,
		WSLayout:                  wsLayout,
		Revisions:                 revisions,
	}, nil
}

func main() {
	minioURL := flag.String("minioURL", "127.0.0.1:9000", "MinIO endpoint")
	referenceMinioURL := flag.String("referenceMinioURL", "", "accepted SplitSnap MinIO endpoint used to validate the transient restore view")
	referenceImageKey := flag.String("referenceImageKey", "aes-go", "SplitSnap ws_shared image key")
	batchWorkloadsPath := flag.String("batchWorkloads", "", "optional JSON manifest for validating all workload/revision transfer views in one pass")
	estimateGlobalWSOnly := flag.Bool("estimateGlobalWSOnly", false, "with -batchWorkloads, only compute the coalesced global exact-content WS union")
	verificationScope := flag.String("verifyCanonical", "all", "canonical content verification scope for batch mode: all or working-set")
	accessKey := flag.String("minioAccessKey", "minio", "MinIO access key")
	secretKey := flag.String("minioSecretKey", "minio123", "MinIO secret key")
	bucket := flag.String("bucket", "snapshots", "MinIO bucket")
	snapshot := flag.String("snapshot", "", "snapshot revision to materialize")
	workers := flag.Int("workers", 28, "concurrent page verification workers")
	zstdLevel := flag.Int("zstdLevel", 3, "Zstd compression level")
	wsLayout := flag.String("zstdWSLayout", zstdstreams.Layout, "coalesced working-set layout")
	reportPath := flag.String("report", "", "optional local JSON report path")
	overwrite := flag.Bool("overwrite", false, "replace an existing transient WS view")
	flag.Parse()

	if *verificationScope != "all" && *verificationScope != "working-set" {
		fmt.Fprintln(os.Stderr, "verifyCanonical must be all or working-set")
		os.Exit(2)
	}
	if strings.TrimSpace(*snapshot) == "" && strings.TrimSpace(*batchWorkloadsPath) == "" {
		fmt.Fprintln(os.Stderr, "-snapshot is required")
		os.Exit(2)
	}
	if strings.TrimSpace(*snapshot) != "" && strings.TrimSpace(*batchWorkloadsPath) != "" {
		fmt.Fprintln(os.Stderr, "-snapshot and -batchWorkloads are mutually exclusive")
		os.Exit(2)
	}
	if *workers < 1 || *wsLayout != zstdstreams.Layout {
		fmt.Fprintln(os.Stderr, "workers must be positive and zstdWSLayout must be streams8-v1")
		os.Exit(2)
	}

	client, err := minio.New(*minioURL, &minio.Options{
		Creds:  credentials.NewStaticV4(*accessKey, *secretKey, ""),
		Secure: false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "create MinIO client: %v\n", err)
		os.Exit(1)
	}
	store, err := storage.NewMinioStorage(client, *bucket)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open object store: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*batchWorkloadsPath) != "" {
		if !*estimateGlobalWSOnly && strings.TrimSpace(*referenceMinioURL) == "" {
			fmt.Fprintln(os.Stderr, "-referenceMinioURL is required with -batchWorkloads")
			os.Exit(2)
		}
		if *overwrite {
			fmt.Fprintln(os.Stderr, "-overwrite is invalid in read-only batch validation mode")
			os.Exit(2)
		}
		manifestData, readErr := os.ReadFile(*batchWorkloadsPath)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "read workload manifest: %v\n", readErr)
			os.Exit(1)
		}
		workloads, parseErr := parseBatchWorkloads(manifestData)
		if parseErr != nil {
			fmt.Fprintln(os.Stderr, parseErr)
			os.Exit(1)
		}
		if *estimateGlobalWSOnly {
			result, estimateErr := estimateGlobalWorkingSet(store, workloads, *workers, *zstdLevel, *wsLayout)
			if estimateErr != nil {
				fmt.Fprintln(os.Stderr, estimateErr)
				os.Exit(1)
			}
			if writeErr := writeJSONReport(result, *reportPath); writeErr != nil {
				fmt.Fprintln(os.Stderr, writeErr)
				os.Exit(1)
			}
			return
		}
		referenceClient, clientErr := minio.New(*referenceMinioURL, &minio.Options{
			Creds:  credentials.NewStaticV4(*accessKey, *secretKey, ""),
			Secure: false,
		})
		if clientErr != nil {
			fmt.Fprintf(os.Stderr, "create SplitSnap reference client: %v\n", clientErr)
			os.Exit(1)
		}
		referenceStore, storeErr := storage.NewMinioStorage(referenceClient, *bucket)
		if storeErr != nil {
			fmt.Fprintf(os.Stderr, "open SplitSnap reference store: %v\n", storeErr)
			os.Exit(1)
		}
		result, validateErr := validateBatch(store, referenceStore, workloads, *workers, *verificationScope, *zstdLevel, *wsLayout)
		if validateErr != nil {
			fmt.Fprintln(os.Stderr, validateErr)
			os.Exit(1)
		}
		result.ReferenceEndpoint = *referenceMinioURL
		if writeErr := writeJSONReport(result, *reportPath); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			os.Exit(1)
		}
		return
	}

	payloadObject := fmt.Sprintf("%s/working_set_pages_content.zstd.streams", *snapshot)
	manifestObject := fmt.Sprintf("%s/working_set_pages_content.zstd.streams.json", *snapshot)
	for _, object := range []string{payloadObject, manifestObject} {
		exists, existsErr := store.Exists(object)
		if existsErr != nil {
			fmt.Fprintf(os.Stderr, "check transient object %s: %v\n", object, existsErr)
			os.Exit(1)
		}
		if exists && !*overwrite {
			fmt.Fprintf(os.Stderr, "refusing existing transient object %s without -overwrite\n", object)
			os.Exit(1)
		}
	}

	started := time.Now()
	recipe, err := store.DownloadObject(fmt.Sprintf("%s/recipe_file", *snapshot))
	if err != nil {
		fmt.Fprintf(os.Stderr, "download recipe: %v\n", err)
		os.Exit(1)
	}
	recipeHashes, err := parseRecipe(recipe)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	wsCSV, err := store.DownloadObject(fmt.Sprintf("%s/working_set_pages", *snapshot))
	if err != nil {
		fmt.Fprintf(os.Stderr, "download working set: %v\n", err)
		os.Exit(1)
	}
	pfns, err := parseWorkingSetPFNs(wsCSV, len(recipeHashes))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	wsHashes := make([]string, len(pfns))
	wanted := make(map[string]struct{}, len(pfns))
	for index, pfn := range pfns {
		wsHashes[index] = recipeHashes[pfn]
		wanted[recipeHashes[pfn]] = struct{}{}
	}
	uniqueRecipe := uniqueSorted(recipeHashes)
	pageObjectKeys, err := store.ListObjects("_chunks/", true)
	if err != nil {
		fmt.Fprintf(os.Stderr, "list canonical page store: %v\n", err)
		os.Exit(1)
	}
	allCanonicalHashes, canonicalSet, err := canonicalHashes(pageObjectKeys)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, hash := range uniqueRecipe {
		if _, exists := canonicalSet[hash]; !exists {
			fmt.Fprintf(os.Stderr, "recipe hash %s is absent from the canonical page store\n", hash)
			os.Exit(1)
		}
	}
	pageByHash, err := verifyAndCollect(store, allCanonicalHashes, wanted, *workers)
	if err != nil {
		fmt.Fprintf(os.Stderr, "verify global page store: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*referenceMinioURL) != "" {
		referenceClient, clientErr := minio.New(*referenceMinioURL, &minio.Options{
			Creds:  credentials.NewStaticV4(*accessKey, *secretKey, ""),
			Secure: false,
		})
		if clientErr != nil {
			fmt.Fprintf(os.Stderr, "create SplitSnap reference client: %v\n", clientErr)
			os.Exit(1)
		}
		referenceStore, storeErr := storage.NewMinioStorage(referenceClient, *bucket)
		if storeErr != nil {
			fmt.Fprintf(os.Stderr, "open SplitSnap reference store: %v\n", storeErr)
			os.Exit(1)
		}
		view, viewErr := validateSplitSnapView(referenceStore, *snapshot, *referenceImageKey, recipeHashes, pfns, pageByHash, *workers)
		if viewErr != nil {
			fmt.Fprintf(os.Stderr, "validate SplitSnap-framework Full Dedup view: %v\n", viewErr)
			os.Exit(1)
		}
		result := report{
			Method:                    "unsafe Full Dedup oracle inside the SplitSnap framework; all persistent pages use global raw-content keys while the accepted SplitSnap shared/private coalesced WS view is an uncounted transient restore input",
			TimingBoundary:            "transient SplitSnap-format WS assembly is excluded; measured restore begins after that view exists",
			Snapshot:                  *snapshot,
			PageSize:                  pageSize,
			RecipeReferences:          len(recipeHashes),
			RecipeUniqueHashes:        len(uniqueRecipe),
			CanonicalPageObjects:      len(allCanonicalHashes),
			CanonicalUnreferenced:     len(allCanonicalHashes) - len(uniqueRecipe),
			VerifiedUniquePageObjects: len(allCanonicalHashes),
			WorkingSetPages:           len(pfns),
			WorkingSetUniqueHashes:    len(wanted),
			RawBytes:                  view.privateRawBytes,
			CompressedBytes:           view.privateCompBytes,
			CompressionRatio:          fmt.Sprintf("%.6f", float64(view.privateRawBytes)/float64(view.privateCompBytes)),
			ZstdLevel:                 3,
			WSLayout:                  zstdstreams.Layout,
			ZstdStreams:               view.privateStreams,
			PayloadObject:             *snapshot + "/working_set_pages_content_private.zstd.streams",
			ManifestObject:            *snapshot + "/working_set_pages_content_private.zstd.streams.json",
			AssemblyElapsedUS:         time.Since(started).Microseconds(),
			ReferenceFramework:        "SplitSnap partial/base-snapshot/shared-base/shared-image/coalesced-private-WS",
			ReferenceEndpoint:         *referenceMinioURL,
			ReferenceImageKey:         *referenceImageKey,
			SharedBaseHashes:          view.baseHashes,
			SharedImageHashes:         view.imageHashes,
			PrivateWorkingSetPages:    view.privatePages,
			CoveredWorkingSetPages:    view.coveredPages,
		}
		reportData, marshalErr := json.MarshalIndent(result, "", "  ")
		if marshalErr != nil {
			fmt.Fprintf(os.Stderr, "marshal report: %v\n", marshalErr)
			os.Exit(1)
		}
		if *reportPath != "" {
			if writeErr := os.WriteFile(*reportPath, append(reportData, '\n'), 0644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "write report: %v\n", writeErr)
				os.Exit(1)
			}
		}
		fmt.Println(string(reportData))
		return
	}

	raw := make([]byte, 0, len(pfns)*pageSize)
	for _, hash := range wsHashes {
		raw = append(raw, pageByHash[hash]...)
	}
	payloadPath, manifest, err := encodeTransient(raw, *zstdLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "encode coalesced WS: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(payloadPath)
	manifestData, err := zstdstreams.MarshalManifest(manifest)
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal coalesced WS manifest: %v\n", err)
		os.Exit(1)
	}
	if err := store.UploadFile(payloadObject, payloadPath); err != nil {
		fmt.Fprintf(os.Stderr, "upload transient payload: %v\n", err)
		os.Exit(1)
	}
	if err := store.UploadObject(manifestObject, bytes.NewReader(manifestData), int64(len(manifestData))); err != nil {
		fmt.Fprintf(os.Stderr, "upload transient manifest: %v\n", err)
		os.Exit(1)
	}

	result := report{
		Method:                    "unsafe Full Dedup oracle with zero-cost WS assembly; canonical pages are globally keyed by raw MD5 and the revision-local coalesced object is an uncounted transient transfer view",
		TimingBoundary:            "assembly_elapsed_us_excluded; measured restore begins after the transient coalesced WS view exists",
		Snapshot:                  *snapshot,
		PageSize:                  pageSize,
		RecipeReferences:          len(recipeHashes),
		RecipeUniqueHashes:        len(uniqueRecipe),
		CanonicalPageObjects:      len(allCanonicalHashes),
		CanonicalUnreferenced:     len(allCanonicalHashes) - len(uniqueRecipe),
		VerifiedUniquePageObjects: len(allCanonicalHashes),
		WorkingSetPages:           len(pfns),
		WorkingSetUniqueHashes:    len(wanted),
		RawBytes:                  len(raw),
		CompressedBytes:           int(manifest.CompressedSize),
		CompressionRatio:          fmt.Sprintf("%.6f", float64(len(raw))/float64(manifest.CompressedSize)),
		ZstdLevel:                 *zstdLevel,
		WSLayout:                  *wsLayout,
		ZstdStreams:               len(manifest.Streams),
		PayloadObject:             payloadObject,
		ManifestObject:            manifestObject,
		AssemblyElapsedUS:         time.Since(started).Microseconds(),
	}
	reportData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal report: %v\n", err)
		os.Exit(1)
	}
	if *reportPath != "" {
		if err := os.WriteFile(*reportPath, append(reportData, '\n'), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write report: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println(string(reportData))
}

func encodeTransient(raw []byte, level int) (string, *zstdstreams.Manifest, error) {
	f, err := os.CreateTemp("", "splitsnap-streams-view-*")
	if err != nil {
		return "", nil, err
	}
	m, encodeErr := zstdstreams.EncodeTo(f, raw, level)
	closeErr := f.Close()
	if encodeErr == nil {
		encodeErr = closeErr
	}
	if encodeErr != nil {
		os.Remove(f.Name())
		return "", nil, encodeErr
	}
	return f.Name(), m, nil
}
