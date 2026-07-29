package snapshotting

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

const minIOChunkUploadPages = 24 * 1024

// TestMinIOArtifactStoreIntegration is intentionally opt-in so unit tests do
// not require a running object store. Set VHIVE_MINIO_ENDPOINT (and optionally
// VHIVE_MINIO_ACCESS_KEY and VHIVE_MINIO_SECRET_KEY) to run it.
func TestMinIOArtifactStoreIntegration(t *testing.T) {
	endpoint := os.Getenv("VHIVE_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set VHIVE_MINIO_ENDPOINT to run the MinIO integration test")
	}
	accessKey := os.Getenv("VHIVE_MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minio"
	}
	secretKey := os.Getenv("VHIVE_MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minio123"
	}
	store, err := NewMinIOArtifactStore(MinIOArtifactStoreConfig{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey,
		Bucket: "test-" + uuid.NewString(),
	})
	require.NoError(t, err)
	defer func() {
		objects, listErr := store.List(context.Background(), "")
		if listErr == nil {
			for _, object := range objects {
				_ = store.client.RemoveObject(context.Background(), store.bucket, string(object.Key), minio.RemoveObjectOptions{})
			}
		}
		_ = store.client.RemoveBucket(context.Background(), store.bucket)
	}()

	key, err := RevisionArtifactKey("integration-revision", "mem_file")
	require.NoError(t, err)
	payload := []byte("byte-exact MinIO payload")
	require.NoError(t, store.Put(context.Background(), key, bytes.NewReader(payload), int64(len(payload))))
	reader, err := store.Get(context.Background(), key)
	require.NoError(t, err)
	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.Equal(t, payload, got)

	objects, err := store.List(context.Background(), "revisions/integration-revision/")
	require.NoError(t, err)
	require.Len(t, objects, 1)
	_, err = store.Get(context.Background(), ArtifactKey("revisions/integration-revision/missing"))
	require.True(t, errors.Is(err, ErrArtifactNotFound), "missing object error = %v", err)
}

// TestMinIOChunkedMemoryUploadIntegration exercises a realistic number of
// unique 4 KiB pages. It is separately opt-in because it creates 24,576
// objects, which is intentionally large enough to expose per-page request
// overhead in an object store.
func TestMinIOChunkedMemoryUploadIntegration(t *testing.T) {
	if os.Getenv("VHIVE_MINIO_CHUNK_STRESS_TEST") == "" {
		t.Skip("set VHIVE_MINIO_CHUNK_STRESS_TEST to run the 4 KiB chunk upload test")
	}
	endpoint := os.Getenv("VHIVE_MINIO_ENDPOINT")
	if endpoint == "" {
		t.Skip("set VHIVE_MINIO_ENDPOINT to run the MinIO integration test")
	}
	accessKey := os.Getenv("VHIVE_MINIO_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "minio"
	}
	secretKey := os.Getenv("VHIVE_MINIO_SECRET_KEY")
	if secretKey == "" {
		secretKey = "minio123"
	}
	store, err := NewMinIOArtifactStore(MinIOArtifactStoreConfig{Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: "test-" + uuid.NewString()})
	require.NoError(t, err)
	defer func() {
		objects, listErr := store.List(context.Background(), "")
		if listErr == nil {
			keys := make(chan minio.ObjectInfo)
			go func() {
				defer close(keys)
				for _, object := range objects {
					keys <- minio.ObjectInfo{Key: string(object.Key)}
				}
			}()
			for range store.client.RemoveObjects(context.Background(), store.bucket, keys, minio.RemoveObjectsOptions{}) {
			}
		}
		_ = store.client.RemoveBucket(context.Background(), store.bucket)
	}()

	file, err := os.CreateTemp(t.TempDir(), "memory-*")
	require.NoError(t, err)
	page := make([]byte, 4096)
	for index := 0; index < minIOChunkUploadPages; index++ {
		binary.LittleEndian.PutUint64(page, uint64(index))
		_, err := file.Write(page)
		require.NoError(t, err)
	}
	require.NoError(t, file.Close())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	recipe, err := uploadChunkedMemory(ctx, store, file.Name(), len(page))
	require.NoError(t, err)
	require.Len(t, recipe.Chunks, minIOChunkUploadPages)
	objects, err := store.List(context.Background(), "shared/chunks/")
	require.NoError(t, err)
	require.Len(t, objects, minIOChunkUploadPages)
}
