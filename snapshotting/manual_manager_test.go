// MIT License
//
// Copyright (c) 2020 Plamen Petrov, Amory Hoste and EASE lab
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

package snapshotting_test

import (
	"context"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
	"os"
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

func TestSnapshotMinioUploadDownload(t *testing.T) {
	revision := "myrevision-2"
	image := "testImage"

	storage := setupMinioClient(t)
	manager := snapshotting.NewSnapshotManager(snapshotsDir, storage, false)

	// Test snapshot doesn't exist remotely before upload
	exists, err := manager.SnapshotExists(revision)
	require.NoError(t, err)
	require.False(t, exists, "snapshot should not exist remotely before upload")

	// Create and commit a snapshot
	snap, err := manager.InitSnapshot(revision, image)
	require.NoError(t, err)

	err = snap.SerializeSnapInfo()
	require.NoError(t, err)

	// Create dummy snap and mem files, since this test does not involve actual VM snapshots
	err = os.WriteFile(snap.GetSnapshotFilePath(), []byte("dummy snapshot data"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(snap.GetMemFilePath(), []byte("dummy mem data"), 0644)
	require.NoError(t, err)

	err = manager.CommitSnapshot(revision)
	require.NoError(t, err)

	// Upload snapshot
	err = manager.UploadSnapshot(revision)
	require.NoError(t, err)

	// Delete snapshot from local manager
	err = manager.DeleteSnapshot(revision)
	require.NoError(t, err, "Failed to delete snapshot after upload")

	// Test snapshot exists remotely after upload
	exists, err = manager.SnapshotExists(revision)
	require.NoError(t, err)
	require.True(t, exists, "snapshot should exist remotely after upload")

	// Download snapshot again
	downloadedSnap, err := manager.DownloadSnapshot(revision)
	require.NoError(t, err)

	// Validate files are downloaded
	_, err = os.Stat(downloadedSnap.GetMemFilePath())
	require.NoError(t, err, "mem file missing after download")
	_, err = os.Stat(downloadedSnap.GetSnapshotFilePath())
	require.NoError(t, err, "snap file missing after download")
	_, err = os.Stat(downloadedSnap.GetInfoFilePath())
	require.NoError(t, err, "info file missing after download")
}
