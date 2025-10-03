// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
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

package ctriface

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/snapshotting"
	"github.com/vhive-serverless/vhive/storage"
)

const (
	remoteSnapshotsDir = "/tmp/vhive/remote-snapshots"
)

var (
	remoteSnaps = flag.Bool("remote-snaps", false, "Run tests with remote snapshots (upload/download to/from MinIO)")
)

func TestSnapLoad(t *testing.T) {
	// Need to clean up manually after this test because StopVM does not
	// work for stopping machines which are loaded from snapshots yet
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	vmID := "1"
	revision := "myrev-1"

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap := snapshotting.NewSnapshot(revision, "/fccd/snapshots", *testImage)
	err = snap.CreateSnapDir()
	require.NoError(t, err, "Failed to create snapshots directory")

	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	vmID = "2"

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM")

	_ = snap.Cleanup()
	orch.Cleanup()
}

func TestSnapLoadMultiple(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	vmID := "3"
	revision := "myrev-3"

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap := snapshotting.NewSnapshot(revision, "/fccd/snapshots", *testImage)
	err = snap.CreateSnapDir()
	require.NoError(t, err, "Failed to create snapshots directory")

	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	vmID = "4"

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	vmID = "5"

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, ")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	_ = snap.Cleanup()
	orch.Cleanup()
}

func TestParallelSnapLoad(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmNum := 5
	vmIDBase := 6

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	// Pull image
	_, err := orch.getImage(ctx, *testImage)
	require.NoError(t, err, "Failed to pull image "+*testImage)

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			vmID := fmt.Sprintf("%d", i+vmIDBase)
			revision := fmt.Sprintf("myrev-%d", i+vmIDBase)

			_, _, err := orch.StartVM(ctx, vmID, *testImage)
			require.NoError(t, err, "Failed to start VM, "+vmID)

			err = orch.PauseVM(ctx, vmID)
			require.NoError(t, err, "Failed to pause VM, "+vmID)

			snap := snapshotting.NewSnapshot(revision, "/fccd/snapshots", *testImage)
			err = snap.CreateSnapDir()
			require.NoError(t, err, "Failed to create snapshots directory")

			err = orch.CreateSnapshot(ctx, vmID, snap)
			require.NoError(t, err, "Failed to create snapshot of VM, "+vmID)

			_, err = orch.ResumeVM(ctx, vmID)
			require.NoError(t, err, "Failed to resume VM")

			err = orch.StopSingleVM(ctx, vmID)
			require.NoError(t, err, "Failed to offload VM, "+vmID)

			vmIDInt, _ := strconv.Atoi(vmID)
			vmID = strconv.Itoa(vmIDInt + 1)

			_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
			require.NoError(t, err, "Failed to load snapshot of VM, "+vmID)

			_, err = orch.ResumeVM(ctx, vmID)
			require.NoError(t, err, "Failed to resume VM, "+vmID)

			err = orch.StopSingleVM(ctx, vmID)
			require.NoError(t, err, "Failed to offload VM, "+vmID)

			_ = snap.Cleanup()
		}(i)
	}
	vmGroup.Wait()

	orch.Cleanup()
}

func TestParallelPhasedSnapLoad(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmNum := 10
	vmIDBase := 16

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	// Pull image
	_, err := orch.getImage(ctx, *testImage)
	require.NoError(t, err, "Failed to pull image "+*testImage)

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				_, _, err := orch.StartVM(ctx, vmID, *testImage)
				require.NoError(t, err, "Failed to start VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				err := orch.PauseVM(ctx, vmID)
				require.NoError(t, err, "Failed to pause VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				revision := fmt.Sprintf("myrev-%d", i+vmIDBase)
				snap := snapshotting.NewSnapshot(revision, "/fccd/snapshots", *testImage)
				err = snap.CreateSnapDir()
				require.NoError(t, err, "Failed to create snapshots directory")

				err := orch.CreateSnapshot(ctx, vmID, snap)
				require.NoError(t, err, "Failed to create snapshot of VM, "+vmID)

				_, err = orch.ResumeVM(ctx, vmID)
				require.NoError(t, err, "Failed to resume VM")
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				err := orch.StopSingleVM(ctx, vmID)
				require.NoError(t, err, "Failed to offload VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				snap := snapshotting.NewSnapshot(vmID, "/fccd/snapshots", *testImage)
				vmIDInt, _ := strconv.Atoi(vmID)
				vmID = strconv.Itoa(vmIDInt + 1)
				_, _, err := orch.LoadSnapshot(ctx, vmID, snap)
				require.NoError(t, err, "Failed to load snapshot of VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				_, err := orch.ResumeVM(ctx, vmID)
				require.NoError(t, err, "Failed to resume VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				err := orch.StopSingleVM(ctx, vmID)
				require.NoError(t, err, "Failed to stop VM, "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	orch.Cleanup()
}

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

func TestRemoteSnapCreate(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmID := "37"
	revision := "myrev-37"

	var snapshotManager *snapshotting.SnapshotManager

	if *remoteSnaps {
		// Initialize MinIO storage for remote snapshots
		storage := setupMinioClient(t)
		snapshotManager = snapshotting.NewSnapshotManager(remoteSnapshotsDir, storage, false)
	} else {
		snapshotManager = snapshotting.NewSnapshotManager(remoteSnapshotsDir, nil, false)
	}

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap, err := snapshotManager.InitSnapshot(revision, *testImage)
	require.NoError(t, err, "Failed to initialize snapshot")

	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = snap.SerializeSnapInfo()
	require.NoError(t, err, "Failed to serialize snapshot info")

	err = snapshotManager.CommitSnapshot(revision)
	require.NoError(t, err, "Failed to commit snapshot")

	if *remoteSnaps {
		// Upload snapshot to MinIO
		err = snapshotManager.UploadSnapshot(revision)
		require.NoError(t, err, "Failed to upload snapshot to MinIO")
	}

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	orch.Cleanup()
}

func TestRemoteSnapLoad(t *testing.T) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmID := "37"
	revision := "myrev-37"

	var snapshotManager *snapshotting.SnapshotManager

	if *remoteSnaps {
		// Initialize MinIO storage for remote snapshots
		storage := setupMinioClient(t)
		snapshotManager = snapshotting.NewSnapshotManager(remoteSnapshotsDir, storage, false)
	} else {
		snapshotManager = snapshotting.NewSnapshotManager(remoteSnapshotsDir, nil, true)
	}

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	var snap *snapshotting.Snapshot
	var err error
	if *remoteSnaps {
		// Download snapshot from MinIO
		_, err = snapshotManager.DownloadSnapshot(revision)
		require.NoError(t, err, "Failed to download snapshot from MinIO")

		snap, err = snapshotManager.AcquireSnapshot(revision)
		require.NoError(t, err, "Failed to acquire snapshot")
	} else {
		snap = snapshotting.NewSnapshot(revision, remoteSnapshotsDir, *testImage)
	}

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load remote snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM")

	_ = snap.Cleanup()
	orch.Cleanup()
}
