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
	"os"
	"strconv"
	"sync"
	"testing"

	ctrdlog "github.com/containerd/log"
	"github.com/google/uuid"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ctriface "github.com/vhive-serverless/vhive/ctriface"
	"github.com/vhive-serverless/vhive/snapshotting"
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
	isRemoteSnapshotsTest  = flag.Bool("remoteSnapshotsTest", false, "Store snapshots remotely during tests")
	wsCoalesceTest         = flag.Bool("wsCoalesceTest", false, "Coalesce working sets into a single file")
	chunkedMemorySizeTest  = flag.Int("chunkedMemorySizeTest", 0, "Remote snapshot memory chunk size in bytes (0 disables chunking)")
	isWithCache            = flag.Bool("withCache", false, "Do not drop the cache before measurements")
	benchDir               = flag.String("benchDirTest", "bench_results", "Directory where stats should be saved")
	snapshotterTest        = flag.String("ss", "devmapper", "Snapshotter to use")
	testImage              = flag.String("img", testImageName, "Test image")
	dockerCredentialsTest  = flag.String("dockerCredentials", "", "Docker credentials for pulling images from inside a microVM")
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
	// Keep the existing tests' image references while allowing integration runs
	// to select an eStargz image with -img.
	testImageName = *testImage

	log.Infof("Orchestrator snapshots enabled: %t", *isSnapshotsEnabledTest)
	log.Infof("Orchestrator UPF enabled: %t", *isUPFEnabledTest)
	log.Infof("Orchestrator lazy serving mode enabled: %t", *isLazyModeTest)
	log.Infof("Working-set coalescing enabled: %t", *wsCoalesceTest)
	log.Infof("Remote snapshots enabled: %t", *isRemoteSnapshotsTest)
	log.Infof("Remote snapshot memory chunk size: %d", *chunkedMemorySizeTest)
	log.Infof("Orchestrator UPF metrics enabled: %t", *isMetricsModeTest)
	log.Infof("Drop cache: %t", !*isWithCache)
	log.Infof("Bench dir: %s", *benchDir)
	log.Infof("Snapshotter: %s", *snapshotterTest)
	log.Infof("Test image: %s", testImageName)

	if *chunkedMemorySizeTest < 0 {
		log.Error("Remote snapshot memory chunk size must not be negative")
		os.Exit(-1)
	}
	if *chunkedMemorySizeTest > 0 && !*isRemoteSnapshotsTest {
		log.Error("Chunked snapshot memory requires remote snapshots")
		os.Exit(-1)
	}

	orchOptions := []ctriface.OrchestratorOption{
		ctriface.WithTestModeOn(true),
		ctriface.WithSnapshots(*isSnapshotsEnabledTest),
		ctriface.WithUPF(*isUPFEnabledTest),
		ctriface.WithMetricsMode(*isMetricsModeTest),
		ctriface.WithLazyMode(*isLazyModeTest),
		ctriface.WithWSCoalescing(*wsCoalesceTest),
		ctriface.WithDockerCredentials(*dockerCredentialsTest),
	}
	if *isRemoteSnapshotsTest {
		// The in-memory store exercises the remote publication/download path
		// without making the default benchmark target depend on MinIO.
		endpoint := os.Getenv("VHIVE_MINIO_ENDPOINT")
		accessKey := os.Getenv("VHIVE_MINIO_ACCESS_KEY")
		if accessKey == "" {
			accessKey = "minio"
		}
		secretKey := os.Getenv("VHIVE_MINIO_SECRET_KEY")
		if secretKey == "" {
			secretKey = "minio123"
		}
		orchOptions = append(orchOptions, ctriface.WithChunkedMemory(*chunkedMemorySizeTest))
		if endpoint != "" {
			orchOptions = append(orchOptions,
				ctriface.WithArtifactStoreConfig(snapshotting.MinIOArtifactStoreConfig{
					Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey,
					Bucket: "test-" + uuid.NewString(),
				}),
			)
			log.Infof("Using real minio at %s", endpoint)
		} else {
			orchOptions = append(orchOptions, ctriface.WithArtifactStore(snapshotting.NewMemoryArtifactStore()))
		}
	}

	orch = ctriface.NewOrchestrator(
		*snapshotterTest,
		"",
		orchOptions...,
	)

	ret := m.Run()

	err := orch.StopActiveVMs()
	if err != nil {
		log.Printf("Failed to stop VMs, err: %v\n", err)
	}

	orch.Cleanup()

	os.Exit(ret)
}

func TestSendToFunctionSerial(t *testing.T) {
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

// TestSnapshotRoundTripServesIndependentVMs is the application-level local
// snapshot control test. It is intentionally opt-in because it needs the
// Firecracker integration environment; unit tests must not require it.
func TestSnapshotRoundTripServesIndependentVMs(t *testing.T) {
	if !*isSnapshotsEnabledTest {
		t.Skip("requires -snapshotsTest and the Firecracker integration environment")
	}

	const (
		functionID = "snap-rt"
		request    = "world"
		response   = "Hello, world!"
	)

	pool := NewFuncPool(true, 1, 0, isTestModeConst)
	function := pool.getFunction(functionID, testImageName)

	first, _, err := pool.Serve(context.Background(), functionID, testImageName, request)
	require.NoError(t, err)
	require.Equal(t, response, first.Payload)
	firstVMID := function.vmID
	require.NotEmpty(t, firstVMID)

	message, err := pool.RemoveInstance(functionID, testImageName, true)
	require.NoError(t, err, message)

	second, _, err := pool.Serve(context.Background(), functionID, testImageName, request)
	require.NoError(t, err)
	require.Equal(t, response, second.Payload)
	secondVMID := function.vmID
	require.NotEmpty(t, secondVMID)
	require.NotEqual(t, firstVMID, secondVMID)

	message, err = pool.RemoveInstance(functionID, testImageName, true)
	require.NoError(t, err, message)

	third, _, err := pool.Serve(context.Background(), functionID, testImageName, request)
	require.NoError(t, err)
	require.Equal(t, response, third.Payload)
	thirdVMID := function.vmID
	require.NotEmpty(t, thirdVMID)
	require.NotEqual(t, firstVMID, thirdVMID)
	require.NotEqual(t, secondVMID, thirdVMID)

	message, err = pool.RemoveInstance(functionID, testImageName, true)
	require.NoError(t, err, message)
}

func TestSendToFunctionParallel(t *testing.T) {
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
	fID := "3"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum        = 2
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
	fID := "not-cld"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum        = 2
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
	fID := "4"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum        = 4
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
	fID := "5"
	var (
		servedTh      uint64 = 40
		pinnedFuncNum        = 2
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
	fID := "6"
	var (
		servedTh      uint64 = 40
		pinnedFuncNum        = 2
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
