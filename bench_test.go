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
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ustiugov/fccd-orchestrator/metrics"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	isWithCache = flag.Bool("withCache", false, "Do not drop the cache before measurements")
	parallelNum = flag.Int("parallel", 1, "Number of parallel instances to start")
	iterNum     = flag.Int("iter", 1, "Number of iterations to run")
	benchDir    = flag.String("benchDirTest", "bench_results", "Directory where stats should be saved")
)

func TestBenchParallelServe(t *testing.T) {
	log.Infof("With cache: %t", *isWithCache)

	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		serveMetrics       = make([]*metrics.Metric, *parallelNum)
		upfMetrics         = make([]*metrics.Metric, *parallelNum)
	)

	images := getAllImages()
	parallel := *parallelNum
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {
		// Pull image
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		var startVMGroup sync.WaitGroup
		concurrency := 2
		sem := make(chan bool, concurrency)

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)

			startVMGroup.Add(1)

			sem <- true

			go func(vmIDString string) {
				defer startVMGroup.Done()
				defer func() { <-sem }()

				// Create VM (and snapshot)
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "record")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, record_response!")

				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}(vmIDString)
		}

		for i := 0; i < cap(sem); i++ {
			sem <- true
		}
		sem = make(chan bool, concurrency)
		startVMGroup.Wait()
		log.Info("All snapshots created")

		var recordVMGroup sync.WaitGroup

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)
			//log.Infof("Recording VM %s", vmIDString)

			recordVMGroup.Add(1)

			sem <- true

			go func(vmIDString string) {
				//log.Infof("Starting recording GO routine for VM %s", vmIDString)
				defer recordVMGroup.Done()
				defer func() { <-sem }()

				// Record
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "record")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, record_response!")

				//log.Infof("Served VM %s", vmIDString)

				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)

				//log.Infof("VM %s record done.", vmIDString)
			}(vmIDString)
		}

		for i := 0; i < cap(sem); i++ {
			sem <- true
		}

		log.Info("All records done")
		recordVMGroup.Wait()

		for k := 0; k < *iterNum; k++ {
			var vmGroup sync.WaitGroup
			semLoad := make(chan bool, 1000)

			if !*isWithCache {
				dropPageCache()
			}

			tStart := time.Now()
			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)

				semLoad <- true

				vmGroup.Add(1)

				go func(i int) {
					defer vmGroup.Done()
					defer func() { <-semLoad }()

					resp, metr, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
					require.NoError(t, err, "Function returned error")
					require.Equal(t, resp.Payload, "Hello, replay_response!")

					serveMetrics[i] = metr
				}(i)
			}

			for i := 0; i < cap(semLoad); i++ {
				semLoad <- true
			}
			vmGroup.Wait()

			duration := time.Since(tStart).Milliseconds()

			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)
				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)

				if *isUPFEnabledTest {
					memManagerMetrics, err := orch.GetUPFLatencyStats(vmIDString + "_0")
					require.NoError(t, err, "Failed to ge tupf metrics")
					require.Equal(t, len(memManagerMetrics), 1, "wrong length")
					upfMetrics[i] = memManagerMetrics[0]
				}
			}

			if *isUPFEnabledTest {
				for i, metr := range serveMetrics {
					for k, v := range upfMetrics[i].MetricMap {
						metr.MetricMap[k] = v
					}
				}
			}
			for _, metr := range serveMetrics {
				err := metrics.PrintMeanStd(getOutFile("parallelServe.csv"), funcName, metr)
				require.NoError(t, err, "Failed to dump stats")
			}

			err = metrics.PrintMeanStd(getOutFile("parallelServe.csv"), funcName, serveMetrics...)
			require.NoError(t, err, "Failed to dump stats")

			log.Printf("Started %d instances in %d milliseconds", parallel, duration)
		}

		vmID += parallel
	}
}

func TestBenchUPFStats(t *testing.T) {
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

		// Pull image and create snapshot
		resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
		// -----------------------------------------------------------------------

		// Record
		resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		// Replay and gather stats
		for i := 0; i < benchCount; i++ {
			if !*isWithCache {
				dropPageCache()
			}

			resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		vmID++

		err = funcPool.DumpUPFPageStats(vmIDString, imageName, funcName, getOutFile("pageStats.csv"))
		require.NoError(t, err, "Failed to dump page stats for"+funcName)

		err = funcPool.DumpUPFLatencyStats(vmIDString, imageName, funcName, getOutFile("latencyStats.csv"))
		require.NoError(t, err, "Failed to dump latency stats for"+funcName)
	}
}

///////////////////////////////////////////////////////////////////////////////
////////////////////////// Auxialiary functions below /////////////////////////
///////////////////////////////////////////////////////////////////////////////

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}

func createResultsDir() {
	if err := os.MkdirAll(*benchDir, 0777); err != nil {
		log.Fatalf("Failed to create results dir: %v", err)
	}
}

func getOutFile(name string) string {
	return filepath.Join(*benchDir, name)
}

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld": "ustiugov/helloworld:var_workload",
		//"chameleon":    "ustiugov/chameleon:var_workload",
		//"pyaes":        "ustiugov/pyaes:var_workload",
		//"image_rotate": "ustiugov/image_rotate:var_workload",
		//"json_serdes":  "ustiugov/json_serdes:var_workload",
		//"lr_serving":   "ustiugov/lr_serving:var_workload",
		//"cnn_serving":  "ustiugov/cnn_serving:var_workload",
		//"rnn_serving":  "ustiugov/rnn_serving:var_workload",
		//"lr_training":  "ustiugov/lr_training:var_workload",
	}
}

func TestBenchServe(t *testing.T) {
	log.Infof("With cache: %t", *isWithCache)

	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {
		// Pull image
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		vmIDString := strconv.Itoa(vmID)

		// Create VM (and snapshot)
		resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		// Record
		resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)

		serveMetrics := make([]*metrics.Metric, *iterNum)

		for k := 0; k < *iterNum; k++ {
			if !*isWithCache {
				dropPageCache()
			}

			resp, met, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			serveMetrics[k] = met

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		// FUSE
		memManagerMetrics, err := orch.GetUPFLatencyStats(vmIDString + "_0")
		require.NoError(t, err, "Failed to dump get stats for "+funcName)
		require.Equal(t, len(serveMetrics), len(memManagerMetrics), "different metrics lengths")

		for i, metr := range serveMetrics {
			for k, v := range memManagerMetrics[i].MetricMap {
				metr.MetricMap[k] = v
			}
		}

		// ##########################

		vmID++

		// Print individual
		for _, metr := range serveMetrics {
			err := metrics.PrintMeanStd(getOutFile("serve.txt"), funcName, metr)
			require.NoError(t, err, "Failed to dump stats")
		}

		// Print aggregate
		err = metrics.PrintMeanStd(getOutFile("serve.txt"), funcName, serveMetrics...)
		require.NoError(t, err, "Printing stats returned error")
	}
}
