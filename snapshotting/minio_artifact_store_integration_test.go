package snapshotting

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/stretchr/testify/require"
)

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
