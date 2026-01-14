package main

import (
	"flag"
	"fmt"
	"sync"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"

	log "github.com/sirupsen/logrus"
)

const (
	homeDir = "/users/lkondras"
	// snapDir = "/tmp/snapshots"
	snapDir  = homeDir + "/cache_test"
	vhiveDir = homeDir + "/vhive"
)

func main() {
	chunkSize := flag.Uint64("chunkSize", 4*1024, "Chunk size in bytes for memory file uploads and downloads when chunking is enabled")
	cacheSize := flag.Uint64("cacheSize", 1000, "Size of the cache for memory file chunks when chunking is enabled")
	threads := flag.Int("j", 28, "How many concurrent uploads/downloads to run when transferring snapshots")
	encryption := flag.Bool("encryption", false, "Enable snapshot encryption")
	chunkCount := flag.Int("chunkCount", 1, "Number of chunks to upload/download during the benchmark")
	flag.Parse()

	var err error
	minioClient, err := minio.New("10.0.1.1:9000", &minio.Options{
		Creds:  credentials.NewStaticV4("minio", "minio123", ""),
		Secure: false,
	})
	if err != nil || minioClient == nil {
		log.WithError(err).Fatal("failed to create MinIO client")
	}

	objectStore, err := storage.NewMinioStorage(minioClient, "test")
	if err != nil {
		log.WithError(err).Fatalf("failed to create MinIO storage for snapshots in bucket %s", "test")
	}
	cache := snapshotting.NewSnapshotManager(snapDir, objectStore, true, true, true, true, *chunkSize, *cacheSize, "none", *threads, *encryption)

	for i := *chunkCount - 1; i >= 0; i-- {
		if ok, err := objectStore.Exists(fmt.Sprintf("_chunks/te/test_chunk_%d", i)); err == nil && ok {
			break
		}
		cache.DownloadChunk(fmt.Sprintf("test_chunk_%d", i))
	}

	tasks := make(chan string, *chunkCount)
	for i := 0; i < *chunkCount; i++ {
		tasks <- fmt.Sprintf("test_chunk_%d", i)
	}
	close(tasks)

	log.Infof("Starting %d downloads with %d threads...", *chunkCount, *threads)

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(*threads)
	for range *threads {
		go func() {
			defer wg.Done()
			for t := range tasks {
				// log.Info("Starting upload/download of chunk")
				cache.DownloadAndReturnChunk(t)
			}
		}()
	}
	log.Info("Waiting for downloads to finish...")
	wg.Wait()

	log.Infof("Completed %d downloads in %v", *chunkCount, time.Since(start))

	start = time.Now()

	tasks = make(chan string, *chunkCount)
	for i := 0; i < *chunkCount; i++ {
		tasks <- fmt.Sprintf("test_chunk_%d", i)
	}
	close(tasks)

	wg.Add(*threads)
	for range *threads {
		go func() {
			defer wg.Done()
			for t := range tasks {
				// log.Info("Starting upload/download of chunk")
				cache.DownloadAndReturnChunk(t)
			}
		}()
	}
	log.Info("Waiting for downloads to finish...")
	wg.Wait()

	log.Infof("Completed %d downloads second time in %v", *chunkCount, time.Since(start))
}
