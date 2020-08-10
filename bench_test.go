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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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

var isNoCache = flag.Bool("nocache", false, "Drop the cache before measuring serve performance")

func TestBenchParallelServe(t *testing.T) {
	log.Infof("Dropping cache: %t", *isNoCache)

	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)

	images := getAllImages()
	parallel := 100
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

				// Record
				resp, _, err = funcPool.Serve(context.Background(), vmIDString, imageName, "record")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, record_response!")

				message, err = funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}(vmIDString)
		}

		for i := 0; i < cap(sem); i++ {
			sem <- true
		}

		startVMGroup.Wait()
		time.Sleep(10 * time.Second)

		for k := 0; k < 5; k++ {
			var vmGroup sync.WaitGroup
			//start := make(chan struct{})

			if *isNoCache {
				dropPageCache()
			}

			tStart := time.Now()
			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)

				vmGroup.Add(1)

				go func(i int) {
					//<-start
					defer vmGroup.Done()

					resp, metric, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
					require.NoError(t, err, "Function returned error")
					require.Equal(t, resp.Payload, "Hello, replay_response!")

					serveMetrics[i] = metric

				}(i)
			}

			//close(start)
			vmGroup.Wait()

			duration := time.Since(tStart).Milliseconds()

			for i := 0; i < parallel; i++ {
				vmIDString := strconv.Itoa(vmID + i)
				message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
				require.NoError(t, err, "Function returned error, "+message)
			}
			time.Sleep(10 * time.Second)

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
		"helloworld": "ustiugov/helloworld:var_workload",
		/*
			"chameleon":    "ustiugov/chameleon:var_workload",
			"pyaes":        "ustiugov/pyaes:var_workload",
			"image_rotate": "ustiugov/image_rotate:var_workload",
			"json_serdes":  "ustiugov/json_serdes:var_workload",
			"lr_serving":   "ustiugov/lr_serving:var_workload",
			"cnn_serving":  "ustiugov/cnn_serving:var_workload",
			"rnn_serving":  "ustiugov/rnn_serving:var_workload",
			"lr_training":  "ustiugov/lr_training:var_workload",
		*/
	}
}

// getFirecrackerPid Assumes that only one Firecracker process is running
func getFirecrackerPid() ([]byte, error) {
	pidBytes, err := exec.Command("pidof", "firecracker").Output()
	if err != nil {
		log.Warnf("Failed to get Firecracker PID: %v", err)
	}

	return pidBytes, err
}

func getMemFootprint() (float64, error) {
	pidBytes, err := getFirecrackerPid()
	if err != nil {
		return 0, err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(pidBytes), "\n")[0]))
	if err != nil {
		log.Warnf("Pid conversion failed: %v", err)
	}

	cmd := exec.Command("ps", "-o", "rss", "-p", strconv.Itoa(pid))

	out, err := cmd.Output()
	if err != nil {
		log.Warnf("Failed to run ps command: %v", err)
	}

	infoArr := strings.Split(string(out), "\n")[1]
	stats := strings.Fields(infoArr)

	rss, err := strconv.ParseFloat(stats[0], 64)
	if err != nil {
		log.Warnf("Error in conversion when computing rss: %v", err)
		return 0, err
	}

	rss *= 1024

	return rss, nil
}

func appendMemFootprint(outFileName string, memFootprint float64) {
	f, err := os.OpenFile(outFileName, os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Println(err)
	}
	defer f.Close()
	if _, err := f.WriteString(fmt.Sprintf("MemFootprint\t%12.1f\n", memFootprint)); err != nil {
		log.Println(err)
	}
}
