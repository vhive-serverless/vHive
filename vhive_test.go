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

package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"testing"
	"time"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/vhive-serverless/vhive/ctriface"
)

const (
	isTestModeConst   = true
	isSaveMemoryConst = true
)

var (
	isUPFEnabledTest       = flag.Bool("upfTest", false, "Enable user-level page faults guest memory management")
	isSnapshotsEnabledTest = flag.Bool("snapshotsTest", false, "Use VM snapshots when adding function instances")
	isMetricsModeTest      = flag.Bool("metricsTest", false, "Calculate UPF metrics")
	isLazyModeTest         = flag.Bool("lazyTest", false, "Enable lazy serving mode when UPFs are enabled")
	isWithCache            = flag.Bool("withCache", false, "Do not drop the cache before measurements")
	benchDir               = flag.String("benchDirTest", "bench_results", "Directory where stats should be saved")
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	flag.Parse()

	if *isUPFEnabledTest {
		log.Error("User-level page faults are temporarily disabled (gh-807)")
		os.Exit(-1)
	}

	log.Infof("Orchestrator snapshots enabled: %t", *isSnapshotsEnabledTest)
	log.Infof("Orchestrator UPF enabled: %t", *isUPFEnabledTest)
	log.Infof("Orchestrator lazy serving mode enabled: %t", *isLazyModeTest)
	log.Infof("Orchestrator UPF metrics enabled: %t", *isMetricsModeTest)
	log.Infof("Drop cache: %t", !*isWithCache)
	log.Infof("Bench dir: %s", *benchDir)

	ret := m.Run()
	os.Exit(ret)
}

func TestSendToFunctionSerial(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "1"
	var (
		servedTh      uint64
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for i := 0; i < 2; i++ {
		resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
		require.NoError(t, err, "Function returned error")
		if i == 0 {
			require.Equal(t, resp.IsColdStart, true)
		}

		require.Equal(t, resp.Payload, "Hello, world!")
	}

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSendToFunctionParallel(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "2"
	var (
		servedTh      uint64
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()
			resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)

	}
	vmGroup.Wait()

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestStartSendStopTwice(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "3"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum int    = 2
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for i := 0; i < 2; i++ {
		for k := 0; k < 2; k++ {
			resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}

		message, err := funcPool.RemoveInstance(fID, testImageName, true)
		require.NoError(t, err, "Function returned error, "+message)
	}

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 4, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 2, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotNumericFunction(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "not-cld"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum int    = 2
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 1, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 1, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotColdFunction(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "4"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum int    = 4
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 1, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 1, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestSaveMemorySerial(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "5"
	var (
		servedTh      uint64 = 40
		pinnedFuncNum int    = 2
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for i := 0; i < 100; i++ {
		resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, world!")
	}

	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSaveMemoryParallel(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "6"
	var (
		servedTh      uint64 = 40
		pinnedFuncNum int    = 2
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()

			resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)

	}
	vmGroup.Wait()

	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	message, err := funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestDirectStartStopVM(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	fID := "7"
	var (
		servedTh      uint64
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	message, err := funcPool.AddInstance(fID, testImageName)
	require.NoError(t, err, "This error should never happen (addInstance())"+message)

	resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err = funcPool.RemoveInstance(fID, testImageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestAllFunctions(t *testing.T) {
	if err := initFirecrackerContainerd(); err != nil {
		t.Fatalf("Failed to initialize firecracker containerd: %v", err)
	}
	defer cleanup()

	if testing.Short() {
		t.Skip("skipping TestAllFunctions in non-nightly runs.")
	}

	images := []string{
		"ghcr.io/ease-lab/helloworld:var_workload",
		"ghcr.io/ease-lab/chameleon:var_workload",
		"ghcr.io/ease-lab/pyaes:var_workload",
		"ghcr.io/ease-lab/image_rotate:var_workload",
		"ghcr.io/ease-lab/json_serdes:var_workload",
		"ghcr.io/ease-lab/lr_serving:var_workload",
		"ghcr.io/ease-lab/cnn_serving:var_workload",
		"ghcr.io/ease-lab/rnn_serving:var_workload",
		"ghcr.io/ease-lab/lr_training:var_workload",
		"ghcr.io/ease-lab/springboot:var_workload",
	}
	var (
		servedTh      uint64
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for i := 0; i < 2; i++ {
		var vmGroup sync.WaitGroup
		for fID, imageName := range images {
			reqs := []string{"world", "record", "replay"}
			resps := []string{"world", "record_response", "replay_response"}
			for k := 0; k < 3; k++ {
				vmGroup.Add(1)
				go func(fID int, imageName, request, response string) {
					defer vmGroup.Done()

					resp, _, err := funcPool.Serve(context.Background(), strconv.Itoa(8+fID), imageName, request)
					require.NoError(t, err, "Function returned error")

					require.Equal(t, resp.Payload, "Hello, "+response+"!")
				}(fID, imageName, reqs[k], resps[k])
			}
			vmGroup.Wait()
		}
	}

	var vmGroup sync.WaitGroup
	for fID, imageName := range images {
		vmGroup.Add(1)
		go func(fID int, imageName string) {
			defer vmGroup.Done()

			message, err := funcPool.RemoveInstance(strconv.Itoa(8+fID), imageName, true)
			require.NoError(t, err, "Function returned error, "+message)
		}(fID, imageName)
	}
	vmGroup.Wait()
}

func initFirecrackerContainerd() error {
	firecrackerBin := os.Getenv("FIRECRACKER_BIN")
	firecrackerConfigPath := os.Getenv("FIRECRACKER_CONFIG_PATH")
	logOut := os.Getenv("LOG_OUT")
	logErr := os.Getenv("LOG_ERR")

	logOutFile, err := os.OpenFile(logOut, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", logOut, err)
	}
	defer logOutFile.Close()

	logErrFile, err := os.OpenFile(logErr, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open error log file %s: %w", logErr, err)
	}
	defer logErrFile.Close()

	cmd := exec.Command(firecrackerBin, "--config", firecrackerConfigPath)
	cmd.Stdout = logOutFile
	cmd.Stderr = logErrFile

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start firecracker containerd: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := waitForContainerd(ctx); err != nil {
		return fmt.Errorf("failed to connect to containerd: %w", err)
	}

	orch = ctriface.NewOrchestrator(
		"devmapper",
		"",
		ctriface.WithTestModeOn(true),
		ctriface.WithSnapshots(*isSnapshotsEnabledTest),
		ctriface.WithUPF(*isUPFEnabledTest),
		ctriface.WithMetricsMode(*isMetricsModeTest),
		ctriface.WithLazyMode(*isLazyModeTest),
	)

	return nil
}

func waitForContainerd(ctx context.Context) error {
	containerdAddress := "/run/firecracker-containerd/containerd.sock"

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if conn, err := net.DialTimeout("unix", containerdAddress, 200*time.Millisecond); err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

func cleanup() {
	if err := orch.StopActiveVMs(); err != nil {
		log.Printf("Failed to stop VMs, err: %v\n", err)
	}
	orch.Cleanup()
	err := exec.Command("bash", "scripts/clean_fcctr.sh").Run()
	if err != nil {
		log.Printf("Failed to clean up firecracker containerd, err: %v\n", err)
	}
}
