package main

import (
	"flag"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
)

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

	numWorkers := *workers
	log.Infof("Starting processing with %d workers", numWorkers)

	var wg sync.WaitGroup
	snapChan := make(chan string, len(snapshots))

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
				}
			}
		}()
	}

	for snapID := range snapshots {
		snapChan <- snapID
	}
	close(snapChan)
	wg.Wait()
}
