package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
)

type ksvcList struct {
	Items []struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	} `json:"items"`
}

func functionType(revision string) string {
	idx := strings.LastIndex(revision, "-")
	if idx <= 0 || idx == len(revision)-1 {
		return revision
	}
	return revision[:idx]
}

func chooseRepresentativeByType(revisions map[string]bool) map[string]string {
	typeToRep := make(map[string]string)
	for rev := range revisions {
		fnType := functionType(rev)
		current, exists := typeToRep[fnType]
		if !exists || rev < current {
			typeToRep[fnType] = rev
		}
	}
	return typeToRep
}

func selectRevisionsForProcessing(revisions map[string]bool, allRevisions bool) []string {
	selected := make([]string, 0, len(revisions))
	if allRevisions {
		for revision := range revisions {
			selected = append(selected, revision)
		}
	} else {
		for _, representative := range chooseRepresentativeByType(revisions) {
			selected = append(selected, representative)
		}
	}
	sort.Strings(selected)
	return selected
}

// A lazy DownloadSnapshot deliberately skips the memory recipe because the
// runtime normally fetches it later through GetUffdMemoryContent.  Conversion
// is different: UploadWorkingSet immediately reconstructs WS pages from the
// chunk recipe, so materialize that input synchronously before conversion.
func materializeLazyRecipe(st storage.ObjectStorage, snap *snapshotting.Snapshot) error {
	recipePath := snap.GetRecipeFilePath()
	objectKey := filepath.Join(snap.GetId(), filepath.Base(recipePath))
	if err := st.DownloadFile(objectKey, recipePath); err != nil {
		return fmt.Errorf("materialize lazy recipe %s: %w", objectKey, err)
	}
	return nil
}

func listKnativeServices() ([]string, error) {
	cmd := exec.Command("kubectl", "get", "ksvc", "-A", "-o", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var parsed ksvcList
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse ksvc list: %w", err)
	}

	names := make([]string, 0, len(parsed.Items))
	for _, item := range parsed.Items {
		name := strings.TrimSpace(item.Metadata.Name)
		nameComponents := strings.Split(name, "-")
		name = strings.Join(nameComponents[:len(nameComponents)-1], "-") // replace dots with dashes to match revision naming
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

func copyRevisionObjects(st storage.ObjectStorage, srcRevision, dstRevision string) error {
	prefix := srcRevision + "/"
	objects, err := st.ListObjects(prefix, true)
	if err != nil {
		return fmt.Errorf("list objects for %s: %w", srcRevision, err)
	}

	for _, srcKey := range objects {
		if !strings.HasPrefix(srcKey, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(srcKey, prefix)
		if suffix == "" {
			continue
		}

		dstKey := dstRevision + "/" + suffix
		data, dlErr := st.DownloadObject(srcKey)
		if dlErr != nil {
			return fmt.Errorf("download source object %s: %w", srcKey, dlErr)
		}
		if upErr := st.UploadObject(dstKey, bytes.NewReader(data), int64(len(data))); upErr != nil {
			return fmt.Errorf("upload copied object %s: %w", dstKey, upErr)
		}
	}

	return nil
}

func main() {
	minioURL := flag.String("minioURL", "10.0.1.1:9000", "MinIO URL")
	minioAccessKey := flag.String("minioAccessKey", "minio", "MinIO Access Key")
	minioSecretKey := flag.String("minioSecretKey", "minio123", "MinIO Secret Key")
	bucketName := flag.String("bucket", "snapshots", "MinIO bucket name")
	targetMode := flag.String("mode", snapshotting.SecurityModeNone,
		"Target security mode (full, partial, no-image-sharing, full-dedup, none)")
	encryption := flag.Bool("encryption", false, "Use encryption")
	baseDir := flag.String("baseDir", "/tmp", "Base directory containing images folder")
	wsCoalescing := flag.Bool("wsCoalescing", false, "Enable WS coalescing")
	wsCompression := flag.Bool("wsCompression", false, "Store coalesced private/full working sets as eight independent long Zstd streams")
	chunkCompression := flag.Bool("chunkCompression", false, "Store each converted snapshot chunk as an independent Zstd frame")
	preserveRecipe := flag.Bool("preserveRecipe", false, "Preserve an existing chunk recipe while materializing the configured chunk representation")
	zstdLevel := flag.Int("zstdLevel", snapshotting.DefaultZstdLevel, "Zstd compression level")
	zstdWSLayout := flag.String("zstdWSLayout", "streams8-v1", "Coalesced WS layout (streams8-v1: eight long Range GETs and eight decoders)")
	zstdFetchers := flag.Int("zstdFetchers", snapshotting.DefaultZstdFetchers, "WS stream range GET/decoder count (must be 8)")
	wsRecording := flag.Bool("wsRecording", false, "Enable WS recording")
	lazy := flag.Bool("lazy", false, "Skip reconstructing complete memory files while converting chunked snapshots")
	chunkSize := flag.Uint64("chunkSize", 4096, "Chunk size for chunking")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of concurrent snapshot workers")
	allRevisions := flag.Bool("allRevisions", false, "Process every snapshot revision instead of one representative per function type")
	serviceFanOut := flag.Bool("serviceFanOut", true, "Copy converted representative snapshots to matching Knative services")
	flag.Parse()

	log.SetLevel(log.DebugLevel)

	minioClient, err := minio.New(*minioURL, &minio.Options{
		Creds:  credentials.NewStaticV4(*minioAccessKey, *minioSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		log.Fatalf("Failed to initialize MinIO client: %v", err)
	}

	st, err := storage.NewMinioStorage(minioClient, *bucketName)
	if err != nil {
		log.Fatalf("Failed to initialize storage: %v", err)
	}

	*targetMode = snapshotting.NormalizeSecurityMode(*targetMode)
	if !snapshotting.IsValidSecurityMode(*targetMode) {
		log.Fatalf("Invalid mode %q. Expected one of: full, partial, no-image-sharing, full-dedup, none", *targetMode)
	}
	if *targetMode == snapshotting.SecurityModeFullDedup && *wsCoalescing {
		log.Fatal("full-dedup requires -wsCoalescing=false: coalesced private working-set objects are revision-scoped and would not implement full deduplication")
	}
	if *wsCompression && !*wsCoalescing {
		log.Fatal("-wsCompression requires -wsCoalescing")
	}
	if *preserveRecipe && !*chunkCompression {
		log.Fatal("-preserveRecipe requires -chunkCompression")
	}
	if *chunkSize == 0 {
		log.Fatalf("chunkSize must be greater than 0")
	}
	if *workers < 1 {
		*workers = 1
	}

	// Initialize SnapshotManager to load chunk info
	smBase := filepath.Join(*baseDir, "snapshots")
	mgr := snapshotting.NewSnapshotManager(smBase, st, true, false, *lazy, false, *wsCoalescing, *wsRecording, *chunkSize, 128*1024*1024, *targetMode, 1, *encryption, false)
	if err := mgr.ConfigureCompression(snapshotting.CompressionConfig{
		WorkingSet: *wsCompression,
		Chunks:     *chunkCompression,
		Codec:      snapshotting.CompressionCodecZstd,
		Level:      *zstdLevel,
		WSLayout:   *zstdWSLayout,
		Fetchers:   *zstdFetchers,
	}); err != nil {
		log.Fatalf("Invalid compression configuration: %v", err)
	}
	log.Infof("SNAPSHOT_CONVERTER_COMPRESSION_CONFIG ws=%t chunks=%t codec=zstd level=%d layout=%s fetchers=%d",
		*wsCompression, *chunkCompression, *zstdLevel, *zstdWSLayout, *zstdFetchers)
	log.Info("Waiting for snapshot manager to initialize chunks...")
	mgr.WaitForInit()

	log.Infof("Preparing base snapshot chunks...")
	var baseEnsureErr error
	if *preserveRecipe {
		// The base recipe is already provenance-classified by the raw pass, just
		// like every revision recipe below.  Re-running the security rewrite here
		// would salt private base hashes a second time and make the Raw and Zstd
		// corpora semantically different.
		baseEnsureErr = mgr.EnsureRemoteSnapshotChunkRepresentation("base")
	} else {
		baseEnsureErr = mgr.EnsureRemoteSnapshotChunked("base")
	}
	if baseEnsureErr != nil {
		log.Fatalf("Failed to convert base snapshot: %v", baseEnsureErr)
	}
	if _, err := mgr.DownloadSnapshot("base"); err != nil {
		log.Warnf("Failed to download base snapshot metadata: %v", err)
		log.Warn("Continuing without base-snapshot chunk classification")
	} else {
		mgr.PrepareBaseSnapshotChunks()
	}

	log.Info("Snapshot manager initialized.")

	// List all snapshots
	objects, err := st.ListObjects("", false)
	if err != nil {
		log.Fatalf("Failed to list objects: %v", err)
	}

	snapshots := make(map[string]bool, len(objects))
	for _, path := range objects {
		parts := strings.Split(path, "/")
		if len(parts) > 1 {
			rev := parts[0]
			if !strings.HasPrefix(rev, "_chunks") && rev != "ws_shared" && rev != "" && rev != "base" {
				snapshots[rev] = true
			}
		}
	}

	log.Infof("Found %d snapshots to process", len(snapshots))
	selectedRevisions := selectRevisionsForProcessing(snapshots, *allRevisions)
	if *allRevisions {
		log.Infof("Processing all %d snapshot revisions", len(selectedRevisions))
	} else {
		log.Infof("Processing %d representative snapshots (one per function type)", len(selectedRevisions))
	}

	numWorkers := *workers
	log.Infof("Starting processing with %d workers", numWorkers)

	var wg sync.WaitGroup
	snapChan := make(chan string, len(selectedRevisions))
	processed := make(map[string]bool, len(selectedRevisions))
	processedMu := sync.Mutex{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for snapID := range snapChan {
				var ensureErr error
				if *preserveRecipe {
					ensureErr = mgr.EnsureRemoteSnapshotChunkRepresentation(snapID)
				} else {
					ensureErr = mgr.EnsureRemoteSnapshotChunked(snapID)
				}
				if ensureErr != nil {
					log.Errorf("Failed processing snapshot %s: %v", snapID, ensureErr)
					continue
				}
				snap, err := mgr.DownloadSnapshot(snapID)
				if err != nil {
					log.Errorf("Failed downloading snapshot %s: %v", snapID, err)
					continue
				}
				if *lazy {
					if err := materializeLazyRecipe(st, snap); err != nil {
						log.Errorf("Failed materializing lazy recipe for snapshot %s: %v", snapID, err)
						continue
					}
				}
				_, _ = mgr.GetWorkingSetPages(snap)
				time.Sleep(100 * time.Millisecond)
				if err := mgr.UploadWorkingSet(snapID); err != nil {
					log.Errorf("Failed uploading working set for snapshot %s: %v", snapID, err)
					continue
				}

				processedMu.Lock()
				processed[snapID] = true
				processedMu.Unlock()
			}
		}()
	}

	for _, snapID := range selectedRevisions {
		snapChan <- snapID
	}
	close(snapChan)
	wg.Wait()
	if len(processed) != len(selectedRevisions) {
		log.Fatalf("Conversion incomplete: processed %d of %d selected snapshot revisions", len(processed), len(selectedRevisions))
	}

	if !*serviceFanOut {
		log.Info("Knative service fan-out disabled")
		return
	}

	processedByType := make(map[string]string, len(processed))
	for rep := range processed {
		processedByType[functionType(rep)] = rep
	}

	services, err := listKnativeServices()
	if err != nil {
		log.Warnf("Failed to list knative services via kubectl, skipping service fan-out: %v", err)
		return
	}
	log.Infof("Discovered %d knative services", len(services))

	targets := make([]string, 0)
	for _, service := range services {
		if processed[service] {
			continue
		}
		targets = append(targets, service)
	}

	if len(targets) == 0 {
		log.Info("No additional services require snapshot fan-out")
		return
	}

	log.Infof("Fanning out snapshots to %d additional services", len(targets))
	targetChan := make(chan string, len(targets))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for targetRevision := range targetChan {
				fnType := functionType(targetRevision)
				sourceRevision, ok := processedByType[fnType]
				if !ok {
					log.Warnf("No processed source snapshot for service %s (function type %s)", targetRevision, fnType)
					continue
				}
				if sourceRevision == targetRevision {
					continue
				}

				if err := copyRevisionObjects(st, sourceRevision, targetRevision); err != nil {
					log.Errorf("Failed copying snapshot objects from %s to %s: %v", sourceRevision, targetRevision, err)
					continue
				}

				if err := mgr.EnsureRemoteSnapshotChunked(targetRevision); err != nil {
					log.Errorf("Failed updating recipe/private chunks for %s after copy: %v", targetRevision, err)
					continue
				}

				log.Infof("Replicated snapshot from %s to %s and rewrote recipe for target-private chunks", sourceRevision, targetRevision)
			}
		}()
	}

	for _, target := range targets {
		targetChan <- target
	}
	close(targetChan)
	wg.Wait()
}
