package snapshotting

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/vhive-serverless/vhive/memory/manager"
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

const (
	benchmarkWorkingSetPageSize  = 4 * 1024
	benchmarkWorkingSetChunkSize = 512 * 1024
	benchmarkWorkingSetChunks    = 400
	benchmarkWorkingSetPages     = 6500
)

// BenchmarkWorkingSetRestore compares the local work needed to replay a
// 6,500-page working set from 400 remote recipe chunks against the same pages
// already coalesced in memory. It does not use UFFD, disk, or network access.
//
// Run the cold sub-benchmark with -benchtime=1x: each iteration intentionally
// creates a fresh page source and downloads 400 512KiB chunks (200MiB total)
// to model a clean restore.
func BenchmarkWorkingSetRestore(b *testing.B) {
	store, recipe, pageOffsets, coalesced := benchmarkWorkingSetFixture(b)
	pageBytes := int64(benchmarkWorkingSetPages * benchmarkWorkingSetPageSize)

	b.Run("coalesced", func(b *testing.B) {
		b.SetBytes(pageBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			for page := 0; page < benchmarkWorkingSetPages; page++ {
				data := coalesced[page*benchmarkWorkingSetPageSize : (page+1)*benchmarkWorkingSetPageSize]
				benchmarkWorkingSetSink ^= data[0]
			}
		}
	})

	b.Run("chunked/cold", func(b *testing.B) {
		b.SetBytes(pageBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			source, err := NewRecipePageSource(store, nil, recipe)
			if err != nil {
				b.Fatal(err)
			}
			for _, offset := range pageOffsets {
				page, err := source.ReadAt(context.Background(), offset, benchmarkWorkingSetPageSize)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkWorkingSetSink ^= page.Bytes[0]
			}
			if err := source.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("chunked/cold/page-server", func(b *testing.B) {
		b.SetBytes(pageBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			source, err := NewRecipePageSource(store, nil, recipe)
			if err != nil {
				b.Fatal(err)
			}
			server, err := manager.NewPageServer(source)
			if err != nil {
				b.Fatal(err)
			}
			for _, offset := range pageOffsets {
				page, err := server.Read(offset, benchmarkWorkingSetPageSize)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkWorkingSetSink ^= page.Bytes[0]
			}
			if err := server.Close(); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("chunked/warm", func(b *testing.B) {
		source, err := NewRecipePageSource(store, nil, recipe)
		if err != nil {
			b.Fatal(err)
		}
		defer source.Close()
		for _, offset := range pageOffsets {
			if _, err := source.ReadAt(context.Background(), offset, benchmarkWorkingSetPageSize); err != nil {
				b.Fatal(err)
			}
		}
		b.SetBytes(pageBytes)
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			for _, offset := range pageOffsets {
				page, err := source.ReadAt(context.Background(), offset, benchmarkWorkingSetPageSize)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkWorkingSetSink ^= page.Bytes[0]
			}
		}
	})

	b.Run("chunk-downloads-only", func(b *testing.B) {
		b.SetBytes(int64(benchmarkWorkingSetChunks * benchmarkWorkingSetChunkSize))
		b.ReportAllocs()
		b.ResetTimer()
		for iteration := 0; iteration < b.N; iteration++ {
			for _, chunk := range recipe.Chunks {
				data, err := readRemoteChunk(context.Background(), store, chunk.ID, benchmarkWorkingSetChunkSize)
				if err != nil {
					b.Fatal(err)
				}
				benchmarkWorkingSetSink ^= data[0]
			}
		}
	})
}

var benchmarkWorkingSetSink byte

func benchmarkWorkingSetFixture(b *testing.B) (*MemoryArtifactStore, MemoryRecipe, []uint64, []byte) {
	b.Helper()
	store := NewMemoryArtifactStore()
	recipe := MemoryRecipe{
		Version:   memoryRecipeVersion,
		ChunkSize: benchmarkWorkingSetChunkSize,
		Chunks:    make([]RecipeChunk, benchmarkWorkingSetChunks),
	}
	for index := range recipe.Chunks {
		data := make([]byte, benchmarkWorkingSetChunkSize)
		binary.LittleEndian.PutUint64(data, uint64(index+1))
		for page := 0; page < benchmarkWorkingSetChunkSize/benchmarkWorkingSetPageSize; page++ {
			data[page*benchmarkWorkingSetPageSize] = byte(index + 1)
		}
		id := chunkID(data)
		if err := store.PutIfAbsent(context.Background(), id, data); err != nil {
			b.Fatal(err)
		}
		recipe.Chunks[index] = RecipeChunk{ID: id}
	}

	pageOffsets := make([]uint64, benchmarkWorkingSetPages)
	coalesced := make([]byte, benchmarkWorkingSetPages*benchmarkWorkingSetPageSize)
	for page := range pageOffsets {
		chunkIndex := page % benchmarkWorkingSetChunks
		pageIndex := page / benchmarkWorkingSetChunks
		pageOffsets[page] = uint64(chunkIndex*benchmarkWorkingSetChunkSize + pageIndex*benchmarkWorkingSetPageSize)
		coalesced[page*benchmarkWorkingSetPageSize] = byte(chunkIndex + 1)
	}
	return store, recipe, pageOffsets, coalesced
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
