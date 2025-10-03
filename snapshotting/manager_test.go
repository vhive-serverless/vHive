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
	"fmt"
	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/snapshotting"
	"os"
	"sync"
	"testing"
)

const snapshotsDir = "/fccd/test/snapshots"

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	os.Exit(m.Run())
}

func testSnapshotManager(t *testing.T, mgr *snapshotting.SnapshotManager, revision, imageName string) {
	// Create snapshot
	snap, err := mgr.InitSnapshot(revision, imageName)
	require.NoError(t, err, fmt.Sprintf("Failed to create snapshot for %s", revision))
	_, err = mgr.InitSnapshot(revision, imageName)
	require.Error(t, err, fmt.Sprintf("Init should fail when a snapshot has already been created for %s", revision))

	err = mgr.CommitSnapshot(snap.GetId())
	require.NoError(t, err, fmt.Sprintf("Failed to commit snapshot for %s", revision))
	err = mgr.CommitSnapshot(snap.GetId())
	require.Error(t, err, fmt.Sprintf("Commit should fail when no snapshots are created for %s", revision))

	// Use snapshot
	_, err = mgr.AcquireSnapshot(snap.GetId())
	require.NoError(t, err, fmt.Sprintf("Failed to acquire snapshot for %s", imageName))
	_, err = mgr.AcquireSnapshot("non-existing-revision")
	require.Error(t, err, fmt.Sprintf("Acquire should fail when no snapshots are available for %s", imageName))
}

func TestSnapshotManagerSingle(t *testing.T) {
	// Create snapshot manager
	mgr := snapshotting.NewSnapshotManager(snapshotsDir, nil, false)

	revision := "myrevision-1" // Snap id = revision
	imageName := "testImage"

	testSnapshotManager(t, mgr, revision, imageName)
}

func TestSnapshotManagerConcurrent(t *testing.T) {
	// Create snapshot manager
	mgr := snapshotting.NewSnapshotManager(snapshotsDir, nil, false)

	var wg sync.WaitGroup
	concurrency := 20
	wg.Add(concurrency)

	for i := 0; i < concurrency; i++ {
		revision := fmt.Sprintf("myrev-%d", i)
		imageName := fmt.Sprintf("testImage-%d", i)
		go func(vmId, imageName string) {
			defer wg.Done()
			testSnapshotManager(t, mgr, vmId, imageName)
		}(revision, imageName)
	}
	wg.Wait()
}
