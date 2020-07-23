// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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
	"os"
	"os/exec"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/containerd/containerd/namespaces"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ustiugov/fccd-orchestrator/metrics"
)

func TestBenchmarkStart(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 250 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1, WithTestModeOn(true))

	vmID := "1"

	benchCount := 10
	startStats := make([]*metrics.StartVMStat, benchCount)

	// Pull image
	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM, "+message)

	for i := 0; i < benchCount; i++ {
		message, stat, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
		require.NoError(t, err, "Failed to start VM, "+message)
		startStats[i] = stat

		message, err = orch.StopSingleVM(ctx, vmID)
		require.NoError(t, err, "Failed to stop VM, "+message)
	}

	metrics.PrintStartVMStats(startStats...)

	orch.Cleanup()
}

func TestBenchmarkLoadSnapshotWithCache(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 250 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1, WithTestModeOn(true))

	vmID := "1"

	benchCount := 10
	loadStats := make([]*metrics.LoadSnapshotStat, benchCount)

	snapshotFile := "/dev/snapshot_file"
	memFile := "/dev/mem_file"

	// Pull image and prepare snapshot
	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+vmID+", "+message)

	message, err = orch.CreateSnapshot(ctx, vmID, snapshotFile, memFile)
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	time.Sleep(300 * time.Millisecond)

	for i := 0; i < benchCount; i++ {
		message, stat, err := orch.LoadSnapshot(ctx, vmID, snapshotFile, memFile)
		require.NoError(t, err, "Failed to load snapshot of VM, "+message)

		loadStats[i] = stat

		message, err = orch.Offload(ctx, vmID)
		require.NoError(t, err, "Failed to offload VM, "+message)

		time.Sleep(300 * time.Millisecond)
	}

	metrics.PrintLoadSnapshotStats(loadStats...)

	orch.Cleanup()
}

func TestBenchmarkLoadSnapshotNoCache(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 250 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	orch := NewOrchestrator("devmapper", 1, WithTestModeOn(true))

	vmID := "2"

	benchCount := 10
	loadStats := make([]*metrics.LoadSnapshotStat, benchCount)

	snapshotFile := "/dev/snapshot_file"
	memFile := "/dev/mem_file"

	// Pull image and prepare snapshot
	message, _, err := orch.StartVM(ctx, vmID, "ustiugov/helloworld:runner_workload")
	require.NoError(t, err, "Failed to start VM, "+message)

	message, err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM, "+vmID+", "+message)

	message, err = orch.CreateSnapshot(ctx, vmID, snapshotFile, memFile)
	require.NoError(t, err, "Failed to create snapshot of VM, "+message)

	message, err = orch.Offload(ctx, vmID)
	require.NoError(t, err, "Failed to offload VM, "+message)

	time.Sleep(300 * time.Millisecond)

	for i := 0; i < benchCount; i++ {
		dropPageCache()

		message, stat, err := orch.LoadSnapshot(ctx, vmID, snapshotFile, memFile)
		require.NoError(t, err, "Failed to load snapshot of VM, "+message)

		loadStats[i] = stat

		message, err = orch.Offload(ctx, vmID)
		require.NoError(t, err, "Failed to offload VM, "+message)

		time.Sleep(300 * time.Millisecond)
	}

	metrics.PrintLoadSnapshotStats(loadStats...)

	orch.Cleanup()
}

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}
