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
	"encoding/csv"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ustiugov/fccd-orchestrator/metrics"
)

const (
	benchDir = "bench_results"
)

var isWithCache = flag.Bool("withCache", false, "Do not drop the cache before measurements")
var parallelNum = flag.Int("parallel", 1, "Number of parallel instances to start")
var iterNum = flag.Int("iter", 1, "Number of iterations to run")

func TestBenchParallelServe(t *testing.T) {
	log.Infof("With cache: %t", *isWithCache)

	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	parallel := *parallelNum
	vmID := 0

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	for funcName, imageName := range images {

		serveMetrics := make([]*metrics.Metric, parallel)

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
		//time.Sleep(10 * time.Second)

		var recordVMGroup sync.WaitGroup

		for i := 0; i < parallel; i++ {
			vmIDString := strconv.Itoa(vmID + i)
			log.Infof("Recording VM %s", vmIDString)

			recordVMGroup.Add(1)

			sem <- true

			go func(vmIDString string) {
				log.Infof("Starting recording GO routine for VM %s", vmIDString)
				defer recordVMGroup.Done()
				defer func() { <-sem }()

				// Record
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "record")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, record_response!")

				log.Infof("Served VM %s", vmIDString)

				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)

				log.Infof("VM %s record done.", vmIDString)
			}(vmIDString)
		}

		for i := 0; i < cap(sem); i++ {
			sem <- true
		}

		log.Info("All records done")
		recordVMGroup.Wait()
		//time.Sleep(10 * time.Second)

		for k := 0; k < *iterNum; k++ {
			var vmGroup sync.WaitGroup

			if !*isWithCache {
				dropPageCache()
			}

			tStart := time.Now()
			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)

				vmGroup.Add(1)

				go func(i int) {
					defer vmGroup.Done()

					resp, metric, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
					require.NoError(t, err, "Function returned error")
					require.Equal(t, resp.Payload, "Hello, replay_response!")

					serveMetrics[i] = metric

				}(i)
			}

			vmGroup.Wait()

			duration := time.Since(tStart).Milliseconds()

			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)
				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}
			//time.Sleep(10 * time.Second)

			log.Printf("Started %d instances in %d milliseconds", parallel, duration)
		}

		vmID += parallel

		outFileName := "serve_" + funcName + "_par_nocache.txt"
		metrics.PrintMeanStd(getOutFile(outFileName), serveMetrics...)
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

	outFileName := "/tmp/upfStat.csv"
	csvFile, err := os.Create(outFileName)
	require.NoError(t, err, "Failed to open stat file")

	writer := csv.NewWriter(csvFile)
	statHeader := []string{"FuncName", "RecPages", "RecRegions", "Served", "StdDev", "Reused", "StdDev", "Unique", "StdDev"}

	err = writer.Write(statHeader)
	require.NoError(t, err, "Failed to open stat file")

	writer.Flush()

	csvFile.Close()

	for funcName, imageName := range images {
		vmIDString := strconv.Itoa(vmID)

		// Pull image
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
			resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, replay_response!")

			message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
			require.NoError(t, err, "Function returned error, "+message)
		}

		vmID++

		err = funcPool.DumpUPFStats(vmIDString, imageName, funcName, outFileName)
		require.NoError(t, err, "Failed to dump stats for"+funcName)
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
	if err := os.MkdirAll(benchDir, 0777); err != nil {
		log.Fatalf("Failed to create results dir: %v", err)
	}
}

func getOutFile(name string) string {
	return filepath.Join(benchDir, name)
}

func getAllImages() map[string]string {
	return map[string]string{
		"helloworld":   "ustiugov/helloworld:var_workload",
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
