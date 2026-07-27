package snapshotting

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
)

var benchmarkBlobSizes = []struct {
	name string
	size int
}{
	{name: "4KiB", size: 4 * 1024},
	{name: "128KiB", size: 128 * 1024},
	{name: "512KiB", size: 512 * 1024},
	{name: "10MiB", size: 10 * 1024 * 1024},
	{name: "512MiB", size: 512 * 1024 * 1024},
}

// BenchmarkChunkCacheFetch measures complete cache-hit reads. The file-backed
// variant includes opening and reading the cached file; the in-memory variant
// uses the same public ChunkHandle API to make the two results comparable.
func BenchmarkChunkCacheFetch(b *testing.B) {
	for _, tc := range benchmarkBlobSizes {
		b.Run("memory/"+tc.name, func(b *testing.B) {
			data := benchmarkData(tc.size)
			cache := newMemoryChunkCache()
			id := chunkID(data)
			insertBenchmarkChunk(b, cache, id, data)
			benchmarkCacheFetch(b, cache, id, len(data))
		})

		b.Run("file/"+tc.name, func(b *testing.B) {
			data := benchmarkData(tc.size)
			cache, err := NewFileChunkCache(b.TempDir())
			if err != nil {
				b.Fatal(err)
			}
			id := chunkID(data)
			insertBenchmarkChunk(b, cache, id, data)
			benchmarkCacheFetch(b, cache, id, len(data))
		})
	}
}

// BenchmarkBlobStorageDownload measures complete blob downloads from a local
// in-memory ArtifactStore and a real remote MinIO ArtifactStore. The remote
// sub-benchmarks are opt-in: set VHIVE_MINIO_ENDPOINT (and optionally
// VHIVE_MINIO_ACCESS_KEY and VHIVE_MINIO_SECRET_KEY) to enable them.
func BenchmarkBlobStorageDownload(b *testing.B) {
	endpoint := os.Getenv("VHIVE_MINIO_ENDPOINT")
	accessKey := os.Getenv("VHIVE_MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minio"
	}
	secretKey := os.Getenv("VHIVE_MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minio123"
	}

	for _, tc := range benchmarkBlobSizes {
		b.Run("local-memory/"+tc.name, func(b *testing.B) {
			data := benchmarkData(tc.size)
			store := NewMemoryArtifactStore()
			key := ArtifactKey("benchmark/blob")
			if err := store.Put(context.Background(), key, bytes.NewReader(data), int64(len(data))); err != nil {
				b.Fatal(err)
			}

			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader, err := store.Get(context.Background(), key)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, reader); err != nil {
					reader.Close()
					b.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})

		b.Run("remote-minio/"+tc.name, func(b *testing.B) {
			if endpoint == "" {
				b.Skip("set VHIVE_MINIO_ENDPOINT to run remote MinIO download benchmarks")
			}
			data := benchmarkData(tc.size)
			store, err := NewMinIOArtifactStore(MinIOArtifactStoreConfig{
				Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey,
				Bucket: "benchmark-" + uuid.NewString(),
			})
			if err != nil {
				b.Fatal(err)
			}
			key := ArtifactKey("benchmark/blob")
			if err := store.Put(context.Background(), key, bytes.NewReader(data), int64(len(data))); err != nil {
				b.Fatal(err)
			}
			b.Cleanup(func() {
				_ = store.client.RemoveObject(context.Background(), store.bucket, string(key), minio.RemoveObjectOptions{})
				_ = store.client.RemoveBucket(context.Background(), store.bucket)
			})

			b.SetBytes(int64(len(data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				reader, err := store.Get(context.Background(), key)
				if err != nil {
					b.Fatal(err)
				}
				if _, err := io.Copy(io.Discard, reader); err != nil {
					reader.Close()
					b.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func benchmarkData(size int) []byte {
	return bytes.Repeat([]byte{0xa5}, size)
}

func insertBenchmarkChunk(b *testing.B, cache ChunkCache, id ChunkID, data []byte) {
	b.Helper()
	handle, err := cache.Insert(context.Background(), id, data)
	if err != nil {
		b.Fatal(err)
	}
	if err := handle.Release(); err != nil {
		b.Fatal(err)
	}
}

func benchmarkCacheFetch(b *testing.B, cache ChunkCache, id ChunkID, size int) {
	b.Helper()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handle, err := cache.Acquire(context.Background(), id)
		if err != nil {
			b.Fatal(err)
		}
		reader, err := handle.Open()
		if err != nil {
			handle.Release()
			b.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, reader); err != nil {
			reader.Close()
			handle.Release()
			b.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			handle.Release()
			b.Fatal(err)
		}
		if err := handle.Release(); err != nil {
			b.Fatal(err)
		}
	}
}
