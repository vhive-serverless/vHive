// MIT License
//
// Copyright (c) 2025 André Jesus and vHive team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package storage_test

import (
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/storage"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func setupMinioClient(t *testing.T) *storage.MinioStorage {
	endpoint := "localhost:9000"
	accessKey := "minio"
	secretKey := "minio123"
	bucket := "test-bucket"

	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: false,
	})
	require.NoError(t, err)

	// Ensure test bucket exists
	exists, err := client.BucketExists(context.Background(), bucket)
	require.NoError(t, err)
	if !exists {
		err = client.MakeBucket(context.Background(), bucket, minio.MakeBucketOptions{})
		require.NoError(t, err)
	}

	store, err := storage.NewMinioStorage(client, bucket)
	require.NoError(t, err)

	return store
}

func TestMinioStorage_UploadAndDownload(t *testing.T) {
	storage := setupMinioClient(t)

	// Create a temp file
	tmpFile := filepath.Join(os.TempDir(), "test_upload.txt")
	content := []byte("hello world")
	err := os.WriteFile(tmpFile, content, 0644)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	// Open file for reading
	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	objectKey := "unit/test_upload.txt"
	err = storage.UploadObject(objectKey, file, stat.Size())
	require.NoError(t, err)

	// Download
	reader, err := storage.DownloadObject(objectKey)
	require.NoError(t, err)
	defer reader.Close()

	downloaded, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, content, downloaded)
}

func TestMinioStorage_Exists(t *testing.T) {
	storage := setupMinioClient(t)

	objectKey := "unit/test_exists.txt"

	// Test that object doesn't exist initially
	exists, err := storage.Exists(objectKey)
	require.NoError(t, err)
	require.False(t, exists)

	// Create and upload a test file
	tmpFile := filepath.Join(os.TempDir(), "test_exists.txt")
	content := []byte("test content for exists check")
	err = os.WriteFile(tmpFile, content, 0644)
	require.NoError(t, err)
	defer os.Remove(tmpFile)

	file, err := os.Open(tmpFile)
	require.NoError(t, err)
	defer file.Close()

	stat, err := file.Stat()
	require.NoError(t, err)

	// Upload the object
	err = storage.UploadObject(objectKey, file, stat.Size())
	require.NoError(t, err)

	// Test that object now exists
	exists, err = storage.Exists(objectKey)
	require.NoError(t, err)
	require.True(t, exists)

	// Test with non-existent object
	nonExistentKey := "unit/does_not_exist.txt"
	exists, err = storage.Exists(nonExistentKey)
	require.NoError(t, err)
	require.False(t, exists)
}
