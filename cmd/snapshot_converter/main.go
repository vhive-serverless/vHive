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
	targetMode := flag.String("mode", "none", "Target security mode (full, partial, none)")
	encryption := flag.Bool("encryption", false, "Use encryption")
	baseDir := flag.String("baseDir", "/tmp", "Base directory containing images folder")
	wsCoalescing := flag.Bool("wsCoalescing", false, "Enable WS coalescing")
	wsRecording := flag.Bool("wsRecording", false, "Enable WS recording")
	chunkSize := flag.Uint64("chunkSize", 4096, "Chunk size for chunking")
	workers := flag.Int("workers", runtime.NumCPU(), "Number of concurrent snapshot workers")
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

	*targetMode = strings.ToLower(strings.TrimSpace(*targetMode))
	if *targetMode != "full" && *targetMode != "partial" && *targetMode != "none" {
		log.Fatalf("Invalid mode %q. Expected one of: full, partial, none", *targetMode)
	}
	if *chunkSize == 0 {
		log.Fatalf("chunkSize must be greater than 0")
	}
	if *workers < 1 {
		*workers = 1
	}

	// Initialize SnapshotManager to load chunk info
	smBase := filepath.Join(*baseDir, "snapshots")
	mgr := snapshotting.NewSnapshotManager(smBase, st, true, false, false, false, *wsCoalescing, *wsRecording, *chunkSize, 128*1024*1024, *targetMode, 1, *encryption, false)
	log.Info("Waiting for snapshot manager to initialize chunks...")
	mgr.WaitForInit()

	log.Infof("Preparing base snapshot chunks...")
	if err := mgr.EnsureRemoteSnapshotChunked("base"); err != nil {
		log.Warnf("Failed to convert base snapshot: %v", err)
	}
	if _, err := mgr.DownloadSnapshot("base"); err != nil {
		log.Warnf("Failed to download base snapshot metadata: %v", err)
	}
	mgr.PrepareBaseSnapshotChunks()

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
			if rev != "_chunks" && rev != "ws_shared" && rev != "" && rev != "base" {
				snapshots[rev] = true
			}
		}
	}

	log.Infof("Found %d snapshots to process", len(snapshots))
	typeToRep := chooseRepresentativeByType(snapshots)
	representatives := make([]string, 0, len(typeToRep))
	for _, rep := range typeToRep {
		representatives = append(representatives, rep)
	}
	sort.Strings(representatives)
	log.Infof("Processing %d representative snapshots (one per function type)", len(representatives))

	numWorkers := *workers
	log.Infof("Starting processing with %d workers", numWorkers)

	var wg sync.WaitGroup
	snapChan := make(chan string, len(representatives))
	processed := make(map[string]bool, len(representatives))
	processedMu := sync.Mutex{}

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for snapID := range snapChan {
				if err := mgr.EnsureRemoteSnapshotChunked(snapID); err != nil {
					log.Errorf("Failed processing snapshot %s: %v", snapID, err)
				}
				snap, err := mgr.DownloadSnapshot(snapID)
				if err != nil {
					log.Errorf("Failed downloading snapshot %s: %v", snapID, err)
					continue
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

	for _, snapID := range representatives {
		snapChan <- snapID
	}
	close(snapChan)
	wg.Wait()

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
