package snapshotting

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOArtifactStoreConfig configures the optional remote artifact store.
// It is inert until supplied to the orchestrator through WithArtifactStoreConfig.
type MinIOArtifactStoreConfig struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Secure    bool
}

func (c MinIOArtifactStoreConfig) validate() error {
	if c.Endpoint == "" || c.AccessKey == "" || c.SecretKey == "" || c.Bucket == "" {
		return fmt.Errorf("MinIO endpoint, access key, secret key, and bucket are required")
	}
	return nil
}

// MinIOArtifactStore adapts a MinIO bucket to ArtifactStore.
type MinIOArtifactStore struct {
	client *minio.Client
	bucket string
}

func NewMinIOArtifactStore(config MinIOArtifactStoreConfig) (*MinIOArtifactStore, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.Secure,
	})
	if err != nil {
		return nil, fmt.Errorf("create MinIO client: %w", err)
	}
	return NewMinIOArtifactStoreWithClient(client, config.Bucket)
}

// NewMinIOArtifactStoreWithClient is useful for integration tests and callers
// that manage MinIO client authentication themselves. It ensures the bucket
// exists before publishing artifacts.
func NewMinIOArtifactStoreWithClient(client *minio.Client, bucket string) (*MinIOArtifactStore, error) {
	if client == nil || bucket == "" {
		return nil, fmt.Errorf("MinIO client and bucket are required")
	}
	ctx := context.Background()
	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check MinIO bucket %q: %w", bucket, err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create MinIO bucket %q: %w", bucket, err)
		}
	}
	return &MinIOArtifactStore{client: client, bucket: bucket}, nil
}

func (s *MinIOArtifactStore) Put(ctx context.Context, key ArtifactKey, reader io.Reader, size int64) error {
	_, err := s.client.PutObject(ctx, s.bucket, string(key), reader, size, minio.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("put artifact %q: %w", key, err)
	}
	return nil
}

func (s *MinIOArtifactStore) Get(ctx context.Context, key ArtifactKey) (io.ReadCloser, error) {
	object, err := s.client.GetObject(ctx, s.bucket, string(key), minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("get artifact %q: %w", key, err)
	}
	// GetObject performs the request lazily. Stat makes missing-key errors
	// observable at Get, matching the ArtifactStore contract.
	if _, err := object.Stat(); err != nil {
		object.Close()
		return nil, minIOArtifactError(key, err)
	}
	return object, nil
}

func (s *MinIOArtifactStore) Stat(ctx context.Context, key ArtifactKey) (ArtifactInfo, error) {
	info, err := s.client.StatObject(ctx, s.bucket, string(key), minio.StatObjectOptions{})
	if err != nil {
		return ArtifactInfo{}, minIOArtifactError(key, err)
	}
	return ArtifactInfo{Key: key, Size: info.Size, ModTime: info.LastModified}, nil
}

func (s *MinIOArtifactStore) List(ctx context.Context, prefix ArtifactKey) ([]ArtifactInfo, error) {
	objects := s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{Prefix: string(prefix), Recursive: true})
	result := make([]ArtifactInfo, 0)
	for object := range objects {
		if object.Err != nil {
			return nil, fmt.Errorf("list artifacts with prefix %q: %w", prefix, object.Err)
		}
		result = append(result, ArtifactInfo{Key: ArtifactKey(object.Key), Size: object.Size, ModTime: object.LastModified})
	}
	return result, nil
}

func minIOArtifactError(key ArtifactKey, err error) error {
	if response := minio.ToErrorResponse(err); response.Code == "NoSuchKey" || response.Code == "NoSuchObject" || response.Code == "NoSuchBucket" {
		return fmt.Errorf("%w: %s", ErrArtifactNotFound, key)
	}
	return fmt.Errorf("access artifact %q: %w", key, err)
}
