// MIT License
//
// # Copyright (c) 2023 Georgiy Lebedev, Dmitrii Ustiugov, Plamen Petrov and vHive team
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
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/containerd/containerd/namespaces"
	ctrdlog "github.com/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/snapshotting"
)

var (
	isUPFEnabled = flag.Bool("upf", false, "Set UPF enabled")
	isLazyMode   = flag.Bool("lazy", false, "Set lazy serving on or off")
	//nolint:unused
	isWithCache       = flag.Bool("withCache", false, "Do not drop the cache before measurements")
	snapshotter       = flag.String("ss", "devmapper", "Snapshotter to use")
	dockerCredentials = flag.String("dockerCredentials", "", "Docker credentials for pulling images from inside a microVM")
	testImage         = flag.String("img", testImageName, "Test image")
)

func TestMain(m *testing.M) {
	flag.Parse()

	os.Exit(m.Run())
}

func TestValidateUPFMode(t *testing.T) {
	tests := []struct {
		name    string
		upf     bool
		lazy    bool
		wantErr bool
	}{
		{name: "disabled", upf: false, lazy: false},
		{name: "working set", upf: true, lazy: false},
		{name: "lazy", upf: true, lazy: true},
		{name: "lazy without UPF", upf: false, lazy: true, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch := &Orchestrator{isUPFEnabled: tt.upf, isLazyMode: tt.lazy}
			err := orch.validateUPFMode()
			if tt.wantErr {
				require.ErrorIs(t, err, errLazyModeRequiresUPF)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStartSnapStopLoad(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.DebugLevel)

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

	vmID := "2"
	revision := "myrev-2"

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	snap := snapshotting.NewSnapshot(revision, "/fccd/snapshots", *testImage)
	err = snap.CreateSnapDir()
	require.NoError(t, err, "Failed to create snapshots directory")

	err = orch.CreateSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to create snapshot of VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM")

	_, _, err = orch.LoadSnapshot(ctx, vmID, snap)
	require.NoError(t, err, "Failed to load snapshot of VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM")

	_ = snap.Cleanup()
	orch.Cleanup()
}

func TestUPFWorkingSetRecordReplay(t *testing.T) {
	if !*isUPFEnabled || *isLazyMode {
		t.Skip("requires UPF working set mode")
	}

	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), 5*time.Minute)
	defer cancel()

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(true),
		WithLazyMode(false),
		WithMetricsMode(true),
		WithDockerCredentials(*dockerCredentials),
	)
	t.Cleanup(func() {
		_ = orch.StopActiveVMs()
		orch.Cleanup()
	})

	sourceVMID := "ws-source"
	recordVMID := "ws-record"
	replayVMID := "ws-replay"
	const installWorkingSetMetric = "InstallWS"
	snap := snapshotting.NewSnapshot("myrev-working-set", "/fccd/snapshots", *testImage)

	_, _, err := orch.StartVM(ctx, sourceVMID, *testImage)
	require.NoError(t, err, "Failed to start source VM")
	require.NoError(t, orch.PauseVM(ctx, sourceVMID), "Failed to pause source VM")
	require.NoError(t, snap.CreateSnapDir(), "Failed to create snapshot directory")
	require.NoError(t, orch.CreateSnapshot(ctx, sourceVMID, snap), "Failed to create snapshot")
	require.NoError(t, orch.StopSingleVM(ctx, sourceVMID), "Failed to stop source VM")

	loadResumeStop := func(vmID string) {
		response, _, err := orch.LoadSnapshot(ctx, vmID, snap)
		require.NoError(t, err, "Failed to load snapshot into %s", vmID)
		_, err = orch.ResumeVM(ctx, vmID)
		require.NoError(t, err, "Failed to resume %s", vmID)
		require.Eventually(t, func() bool {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(response.GuestIP, "50051"), 200*time.Millisecond)
			if err != nil {
				return false
			}
			_ = conn.Close()
			return true
		}, 30*time.Second, 100*time.Millisecond, "%s workload did not become ready", vmID)
		require.NoError(t, orch.StopSingleVM(ctx, vmID), "Failed to stop %s", vmID)
	}

	loadResumeStop(recordVMID)
	workingSetBefore, err := os.ReadFile(snap.GetWorkingSetFilePath())
	require.NoError(t, err, "Failed to read recorded working set")
	require.NotEmpty(t, workingSetBefore, "Recorded working set is empty")
	traceBefore, err := os.ReadFile(snap.GetWorkingSetTraceFilePath())
	require.NoError(t, err, "Failed to read recorded working set trace")
	require.NotEmpty(t, traceBefore, "Recorded working set trace is empty")
	workingSetInfoBefore, err := os.Stat(snap.GetWorkingSetFilePath())
	require.NoError(t, err, "Failed to stat recorded working set")
	traceInfoBefore, err := os.Stat(snap.GetWorkingSetTraceFilePath())
	require.NoError(t, err, "Failed to stat recorded working set trace")

	loadResumeStop(replayVMID)
	latencyMetrics, err := orch.GetUPFLatencyStats(replayVMID)
	require.NoError(t, err, "Failed to get replay metrics")
	workingSetInstalled := false
	for _, metric := range latencyMetrics {
		if _, ok := metric.MetricMap[installWorkingSetMetric]; ok {
			workingSetInstalled = true
			break
		}
	}
	require.True(t, workingSetInstalled, "Working set was not installed during replay")

	workingSetInfoAfter, err := os.Stat(snap.GetWorkingSetFilePath())
	require.NoError(t, err, "Failed to restat working set")
	traceInfoAfter, err := os.Stat(snap.GetWorkingSetTraceFilePath())
	require.NoError(t, err, "Failed to restat working set trace")
	require.True(t, os.SameFile(workingSetInfoBefore, workingSetInfoAfter), "Replay replaced working set pages")
	require.True(t, os.SameFile(traceInfoBefore, traceInfoAfter), "Replay replaced working set trace")
	require.Equal(t, workingSetInfoBefore.ModTime(), workingSetInfoAfter.ModTime(), "Replay modified working set pages")
	require.Equal(t, traceInfoBefore.ModTime(), traceInfoAfter.ModTime(), "Replay modified working set trace")
}

func TestPauseSnapResume(t *testing.T) {
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

	vmID := "4"
	revision := "myrev-4"

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
	require.NoError(t, err, "Failed to stop VM")

	_ = snap.Cleanup()
	orch.Cleanup()
}

func TestStartStopSerial(t *testing.T) {
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

	vmID := "5"

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM")

	orch.Cleanup()
}

func TestPauseResumeSerial(t *testing.T) {
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

	vmID := "6"

	_, _, err := orch.StartVM(ctx, vmID, *testImage)
	require.NoError(t, err, "Failed to start VM")

	err = orch.PauseVM(ctx, vmID)
	require.NoError(t, err, "Failed to pause VM")

	_, err = orch.ResumeVM(ctx, vmID)
	require.NoError(t, err, "Failed to resume VM")

	err = orch.StopSingleVM(ctx, vmID)
	require.NoError(t, err, "Failed to stop VM")

	orch.Cleanup()
}

func TestStartStopParallel(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	testTimeout := 360 * time.Second
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), namespaceName), testTimeout)
	defer cancel()

	vmNum := 10
	vmIDBase := 7

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	if *snapshotter != "proxy" {
		// Pull image (with remote snapshotters you can't pull the image before starting the VM)
		_, err := orch.getImage(ctx, *testImage)
		require.NoError(t, err, "Failed to pull image "+*testImage)
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				_, _, err := orch.StartVM(ctx, vmID, *testImage)
				require.NoError(t, err, "Failed to start VM "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				err := orch.StopSingleVM(ctx, vmID)
				require.NoError(t, err, "Failed to stop VM "+vmID)
			}(i)
		}
		vmGroup.Wait()
	}

	orch.Cleanup()
}

func TestPauseResumeParallel(t *testing.T) {
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
	vmIDBase := 17

	orch := NewOrchestrator(
		*snapshotter,
		"",
		WithTestModeOn(true),
		WithUPF(*isUPFEnabled),
		WithLazyMode(*isLazyMode),
		WithDockerCredentials(*dockerCredentials),
	)

	if *snapshotter != "proxy" {
		// Pull image (with remote snapshotters you can't pull the image before starting the VM)
		_, err := orch.getImage(ctx, *testImage)
		require.NoError(t, err, "Failed to pull image "+*testImage)
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				_, _, err := orch.StartVM(ctx, vmID, *testImage)
				require.NoError(t, err, "Failed to start VM")
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				err := orch.PauseVM(ctx, vmID)
				require.NoError(t, err, "Failed to pause VM")
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				_, err := orch.ResumeVM(ctx, vmID)
				require.NoError(t, err, "Failed to resume VM")
			}(i)
		}
		vmGroup.Wait()
	}

	{
		var vmGroup sync.WaitGroup
		for i := vmIDBase; i < vmNum; i++ {
			vmGroup.Add(1)
			go func(i int) {
				defer vmGroup.Done()
				vmID := fmt.Sprintf("%d", i)
				err := orch.StopSingleVM(ctx, vmID)
				require.NoError(t, err, "Failed to stop VM")
			}(i)
		}
		vmGroup.Wait()
	}

	orch.Cleanup()
}
