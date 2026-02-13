package main

import (
	"bytes"
	"crypto/md5"
	"encoding/gob"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
)

func main() {
	minioURL := flag.String("minioURL", "localhost:9000", "MinIO URL")
	minioAccessKey := flag.String("minioAccessKey", "minio", "MinIO Access Key")
	minioSecretKey := flag.String("minioSecretKey", "minio123", "MinIO Secret Key")
	bucketName := flag.String("bucket", "xsnapshots", "MinIO bucket name")
	targetMode := flag.String("mode", "full", "Target security mode (full, partial)")
	encryption := flag.Bool("encryption", false, "Use encryption")
	baseDir := flag.String("baseDir", "/tmp", "Base directory containing images folder")
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

	// Initialize SnapshotManager to load chunk info
	smBase := filepath.Join(*baseDir, "vhive", "snapshots")
	mgr := snapshotting.NewSnapshotManager(smBase, st, true, false, false, false, false, 4096, 128*1024*1024, *targetMode, 1, *encryption, false)
	log.Info("Waiting for snapshot manager to initialize chunks...")
	mgr.WaitForInit()

	log.Infof("Preparing base snapshot chunks...")
	if _, err := mgr.DownloadSnapshot("base"); err != nil {
		log.Warnf("Failed to download base snapshot: %v", err)
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
			if rev != "_chunks" && rev != "" && rev != "base" {
				snapshots[rev] = true
			}
		}
	}

	log.Infof("Found %d snapshots to process", len(snapshots))

	numWorkers := runtime.NumCPU()
	log.Infof("Starting processing with %d workers", numWorkers)

	var wg sync.WaitGroup
	snapChan := make(chan string, len(snapshots))

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for snapID := range snapChan {
				processSnapshot(st, mgr, snapID, *targetMode)
			}
		}()
	}

	for snapID := range snapshots {
		snapChan <- snapID
	}
	close(snapChan)
	wg.Wait()
}

func processSnapshot(st storage.ObjectStorage, mgr *snapshotting.SnapshotManager, snapID, targetMode string) {
	log.Infof("Processing snapshot: %s", snapID)

	// 1. Get Image Name
	infoKey := fmt.Sprintf("%s/info_file", snapID)
	infoReader, err := st.DownloadObject(infoKey)
	if err != nil {
		log.Warnf("Skipping %s: no info_file: %v", snapID, err)
		return
	}
	defer infoReader.Close()

	var tempSnap snapshotting.Snapshot
	dec := gob.NewDecoder(infoReader)
	if err := dec.Decode(&tempSnap); err != nil {
		log.Warnf("Skipping %s: failed to decode info_file: %v", snapID, err)
		return
	}
	imageName := tempSnap.Image

	// 2. Process Recipe
	recipeKey := fmt.Sprintf("%s/recipe_file", snapID)
	recipeReader, err := st.DownloadObject(recipeKey)
	if err != nil {
		log.Warnf("Skipping %s: no recipe_file: %v", snapID, err)
		return
	}

	recipeData, err := io.ReadAll(recipeReader)
	recipeReader.Close()
	if err != nil {
		log.Warnf("Skipping %s: failed to read recipe: %v", snapID, err)
		return
	}

	newRecipe := make([]byte, len(recipeData))
	copy(newRecipe, recipeData)

	modified := false

	for i := 0; i < len(recipeData); i += 16 {
		if i+16 > len(recipeData) {
			break
		}

		var currentHash [16]byte
		copy(currentHash[:], recipeData[i:i+16])

		// Determine sensitivity
		isSensitive := false
		if targetMode == "full" {
			isSensitive = true
		} else if targetMode == "partial" {
			isSensitive = mgr.IsChunkSensitive(currentHash, imageName)
		}

		if isSensitive {
			// New Hash (mix ID)
			newHashBytes := md5.Sum(append(currentHash[:], []byte(snapID)...))

			currentHashStr := hex.EncodeToString(currentHash[:])
			newHashStr := hex.EncodeToString(newHashBytes[:])

			// Check if chunk exists with new hash
			chunkPath := getChunkPath(newHashStr)
			exists, err := st.Exists(chunkPath)
			if err != nil {
				log.Warnf("Error checking chunk existence %s: %v", newHashStr, err)
				continue
			}

			if !exists {
				// Assumes old chunk is stored with plain hash
				oldChunkPath := getChunkPath(currentHashStr)

				// log.Debugf("Migrating chunk %s -> %s", currentHashStr, newHashStr)

				obj, err := st.DownloadObject(oldChunkPath)
				if err != nil {
					log.Warnf("Failed to download old chunk %s: %v", oldChunkPath, err)
					continue
				}

				data, err := io.ReadAll(obj)
				obj.Close()
				if err != nil {
					log.Warnf("Failed to read old chunk %s: %v", oldChunkPath, err)
					continue
				}

				err = st.UploadObject(chunkPath, bytes.NewReader(data), int64(len(data)))
				if err != nil {
					log.Errorf("Failed to upload new chunk %s: %v", chunkPath, err)
					continue
				}
			}

			// Update recipe with new hash
			copy(newRecipe[i:i+16], newHashBytes[:])
			modified = true
		}
	}

	if modified {
		log.Infof("Updating recipe for %s", snapID)
		err = st.UploadObject(recipeKey, bytes.NewReader(newRecipe), int64(len(newRecipe)))
		if err != nil {
			log.Errorf("Failed to upload updated recipe for %s: %v", snapID, err)
		}
	} else {
		log.Infof("No changes for %s", snapID)
	}
}

func getChunkPath(hash string) string {
	return fmt.Sprintf("_chunks/%s/%s", hash[:2], hash)
}
