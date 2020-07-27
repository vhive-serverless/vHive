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

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ustiugov/fccd-orchestrator/metrics"
)

const (
	benchDir = "bench_results"
)

func TestBenchmarkServeWithCache(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	benchCount := 10
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {
		vmIDString := strconv.Itoa(vmID)
		serveStats := make([]*metrics.Metric, benchCount)

		// Pull image
		resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, replay_response!")

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
		// -----------------------------------------------------------------------

		// Warm up loadsnapshot
		resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, replay_response!")

		message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		for i := 0; i < benchCount; i++ {
			resp, stat, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			serveStats[i] = stat

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		vmID++

		outFileName := "serve_" + funcName + "_cache.txt"
		metrics.PrintMeanStd(getOutFile(outFileName), serveStats...)
	}
}

func TestBenchmarkServeNoCache(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	benchCount := 10
	vmID := 10

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {
		vmIDString := strconv.Itoa(vmID)
		serveStats := make([]*metrics.Metric, benchCount)

		// Pull image
		resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, replay_response!")

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
		// -----------------------------------------------------------------------

		// Warm up loadsnapshot
		resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, replay_response!")

		message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		for i := 0; i < benchCount; i++ {
			dropPageCache()

			resp, stat, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			serveStats[i] = stat

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		vmID++

		outFileName := "serve_" + funcName + "_nocache.txt"
		metrics.PrintMeanStd(getOutFile(outFileName), serveStats...)
	}
}

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}

func createResultsDir() {
	if err := os.MkdirAll(benchDir, 0666); err != nil {
		log.Fatalf("Failed to create results dir: %v", err)
	}
}

func getOutFile(name string) string {
	return filepath.Join(benchDir, name)
}

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld":   "ustiugov/helloworld:var_workload",
		"chameleon":    "ustiugov/chameleon:var_workload",
		"pyaes":        "ustiugov/pyaes:var_workload",
		"image_rotate": "ustiugov/image_rotate:var_workload",
		"json_serdes":  "ustiugov/json_serdes:var_workload",
		//"lr_serving" : "ustiugov/lr_serving:var_workload", Issue#15
		//"cnn_serving" "ustiugov/cnn_serving:var_workload",
		"rnn_serving": "ustiugov/rnn_serving:var_workload",
		//"lr_training" : "ustiugov/lr_training:var_workload",
	}
}

func TestBenchParallelServeWithCache(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	parallel := 4
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {

		serveMetrics := make([]*metrics.Metric, parallel)

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)

			// Pull image and create VM
			resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
			// -----------------------------------------------------------------------

			// Warm up loadsnapshot
			resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		var vmGroup sync.WaitGroup
		start := make(chan struct{})

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)

			vmGroup.Add(1)

			go func(i int) {
				<-start
				defer vmGroup.Done()

				resp, metric, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, replay_response!")

				serveMetrics[i] = metric

				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}(i)
		}

		close(start)
		vmGroup.Wait()

		vmID += parallel

		outFileName := "serve_" + funcName + "_par_cache.txt"
		metrics.PrintMeanStd(getOutFile(outFileName), serveMetrics...)
	}
}

func TestBenchParallelServeNoCache(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	parallel := 4
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {

		serveMetrics := make([]*metrics.Metric, parallel)

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)

			// Pull image and create VM
			resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		var vmGroup sync.WaitGroup
		start := make(chan struct{})

		dropPageCache()

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)

			vmGroup.Add(1)

			go func(i int) {
				<-start
				defer vmGroup.Done()

				resp, metric, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, replay_response!")

				serveMetrics[i] = metric

				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}(i)
		}

		close(start)
		vmGroup.Wait()

		vmID += parallel

		outFileName := "serve_" + funcName + "_par_nocache.txt"
		metrics.PrintMeanStd(getOutFile(outFileName), serveMetrics...)
	}
}
