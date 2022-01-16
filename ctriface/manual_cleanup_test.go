// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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
	"fmt"
	"github.com/ease-lab/vhive/ctriface/regular"
	"github.com/ease-lab/vhive/snapshotting"
	"os"
	"sync"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestSnapLoad(t *testing.T) {
	orch := NewOrchestrator(regular.NewRegOrchestrator(
		"devmapper",
		"",
		"fc-dev-thinpool",
		"",
		10,
		regular.WithTestModeOn(true),
		regular.WithUPF(*isUPFEnabled),
		regular.WithLazyMode(*isLazyMode),
	))

	vmID := "1"
	revisionID := "myrev-1"

	snapLoad(t, orch, vmID, revisionID)
}

func snapLoad(t *testing.T, orch *Orchestrator, vmID string, revisionID string) {
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
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), regular.NamespaceName), testTimeout)
	defer cancel()

	_, _, err := orch.StartVM(ctx, vmID, regular.TestImageName, 256, 1, false)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap := snapshotting.NewSnapshot(revisionID, "/fccd/snapshots", regular.TestImageName, 0, 0, false)
	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	orch.Cleanup()
}

func TestSnapLoadMultiple(t *testing.T) {
	orch := NewOrchestrator(regular.NewRegOrchestrator(
		"devmapper",
		"",
		"fc-dev-thinpool",
		"",
		10,
		regular.WithTestModeOn(true),
		regular.WithUPF(*isUPFEnabled),
		regular.WithLazyMode(*isLazyMode),
	))

	vmID := "3"
	revisionID := "myrev-3"

	snapLoadMultiple(t, orch, vmID, revisionID)
}

func snapLoadMultiple(t *testing.T, orch *Orchestrator, vmID string, revisionID string) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), regular.NamespaceName), testTimeout)
	defer cancel()

	_, _, err := orch.StartVM(ctx, vmID, regular.TestImageName, 256, 1, false)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap := snapshotting.NewSnapshot(revisionID, "/fccd/snapshots", regular.TestImageName, 0, 0,false)
	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM, ")

	orch.Cleanup()
}

func TestParallelSnapLoad(t *testing.T) {
	vmNum := 5
	vmIDBase := 6

	orch := NewOrchestrator(regular.NewRegOrchestrator(
		"devmapper",
		"",
		"fc-dev-thinpool",
		"",
		10,
		regular.WithTestModeOn(true),
		regular.WithUPF(*isUPFEnabled),
		regular.WithLazyMode(*isLazyMode),
	))

	parallelSnapLoad(t, orch, vmNum, vmIDBase)
}

func parallelSnapLoad(t *testing.T, orch *Orchestrator, vmNum int, vmIDBase int) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), regular.NamespaceName), testTimeout)
	defer cancel()

	// Pull image
	_, err := orch.GetImage(ctx, regular.TestImageName)
	require.NoError(t, err, "Failed to pull image "+regular.TestImageName)

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			vmID := fmt.Sprintf("%d", i+vmIDBase)
			revisionID := fmt.Sprintf("myrev-%d", i+vmIDBase)

			_, _, err := orch.StartVM(ctx, vmID, regular.TestImageName, 256, 1, false)
			require.NoError(t, err, "Failed to start VM, "+vmID)

			err = orch.PauseVM(ctx, vmID)
			require.NoError(t, err, "Failed to pause VM, "+vmID)

			snap := snapshotting.NewSnapshot(revisionID, "/fccd/snapshots", regular.TestImageName, 0, 0, false)
			err = orch.CreateSnapshot(ctx, vmID, snap)
			require.NoError(t, err, "Failed to create snapshot of VM, "+vmID)

			_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
			require.NoError(t, err, "Failed to load snapshot of VM, "+vmID)

			_, err = orch.ResumeVM(ctx, vmID)
			require.NoError(t, err, "Failed to resume VM, "+vmID)
		}(i)
	}
	vmGroup.Wait()

	orch.Cleanup()
}

func TestParallelPhasedSnapLoad(t *testing.T) {
	vmNum := 10
	vmIDBase := 11

	orch := NewOrchestrator(regular.NewRegOrchestrator(
		"devmapper",
		"",
		"fc-dev-thinpool",
		"",
		10,
		regular.WithTestModeOn(true),
		regular.WithUPF(*isUPFEnabled),
		regular.WithLazyMode(*isLazyMode),
	))

	parallelPhasedSnapLoad(t, orch, vmNum, vmIDBase)
}

func parallelPhasedSnapLoad(t *testing.T, orch *Orchestrator, vmNum int, vmIDBase int) {
	// Needs to be cleaned up manually.
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 120 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), regular.NamespaceName), testTimeout)
	defer cancel()

	// Pull image
	_, err := orch.GetImage(ctx, regular.TestImageName)
	require.NoError(t, err, "Failed to pull image "+regular.TestImageName)

	{
		var vmGroup sync.WaitGroup
		for i := 0; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i+vmIDBase)
				_, _, err := orch.StartVM(ctx, vmID, regular.TestImageName, 256, 1, false)
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
				revisionID := fmt.Sprintf("myrev-%d", i+vmIDBase)
				snap := snapshotting.NewSnapshot(revisionID, "/fccd/snapshots", regular.TestImageName, 0, 0, false)
				err = orch.CreateSnapshot(ctx, vmID, snap)
				require.NoError(t, err, "Failed to create snapshot of VM, "+vmID)
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
				revisionID := fmt.Sprintf("myrev-%d", i+vmIDBase)
				snap := snapshotting.NewSnapshot(revisionID, "/fccd/snapshots", regular.TestImageName, 0, 0, false)
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

	orch.Cleanup()
}
