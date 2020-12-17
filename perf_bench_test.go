// MIT License
//
// Copyright (c) 2020 Yuchen Niu
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
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ease-lab/vhive/profile"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	vmNum           = flag.Int("vm", 2, "The number of VMs")
	targetReqPerSec = flag.Int("requestPerSec", 10, "The target number of requests per second")
	executionTime   = flag.Int("executionTime", 1, "The execution time of the benchmark in seconds")
	funcNames       = flag.String("funcNames", "helloworld", "Name of the functions to benchmark")
	perfEvents      = flag.String("perfEvents", "", "Perf events (run `perf stat` if not empty)")
)

const (
	avgExecTime = "average-execution-time"
	realRPS     = "real-requests-per-second"
)

func TestBenchRequestPerSecond(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		vmID          int
		isSyncOffload bool = true
		isPerf             = len(*perfEvents) > 0
		images             = getImages(t)
		timeInterval       = time.Duration(time.Second.Nanoseconds() / int64(*targetReqPerSec))
		totalRequests      = *executionTime * *targetReqPerSec
		perfStat           = profile.NewPerfStat(profile.AllCPUs, profile.Event, *perfEvents, profile.OutFile, "perf-tmp.data")
	)
	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	log.SetLevel(log.InfoLevel)
	bootStart := time.Now()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	createResultsDir()

	// Pull images
	for _, imageName := range images {
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")
	}

	imagesLen := len(images)

	// Boot VMs
	for i := 0; i < *vmNum; i++ {
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[i%imagesLen])
		require.NoError(t, err, "Function returned error")
	}
	log.Debugf("All VMs booted in %d ms", time.Since(bootStart).Milliseconds())

	if isPerf {
		err := perfStat.Run()
		require.NoError(t, err, "Start perf stat returned error")
	}

	var vmGroup sync.WaitGroup
	ticker := time.NewTicker(timeInterval)
	tickerDone := make(chan bool, 1)

	serveMetrics := make(map[string]float64)
	serveMetrics[avgExecTime] = 0
	serveMetrics[realRPS] = 0

	remainRequests := totalRequests
	for remainRequests > 0 {
		select {
		case <-ticker.C:
			remainRequests--
			vmGroup.Add(1)

			imageName := images[vmID%imagesLen]
			vmIDString := strconv.Itoa(vmID)

			go serveVM(t, vmIDString, imageName, &vmGroup, isSyncOffload, serveMetrics)

			vmID = (vmID + 1) % *vmNum
		case <-tickerDone:
			ticker.Stop()
		}
	}

	tickerDone <- true
	vmGroup.Wait()

	if isPerf {
		result, err := perfStat.Stop()
		require.NoError(t, err, "Stop perf stat returned error")
		for eventName, value := range result {
			log.Infof("%s: %f\n", eventName, value)
			serveMetrics[eventName] = value
		}
	}

	serveMetrics[avgExecTime] /= float64(totalRequests)

	writeResultToCSV(t, serveMetrics, "benchRPS.csv")
}

func serveVM(t *testing.T, vmIDString, imageName string, vmGroup *sync.WaitGroup, isSyncOffload bool, serveMetrics map[string]float64) {
	defer vmGroup.Done()

	tStart := time.Now()
	resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.IsColdStart, false)

	execTime := time.Since(tStart).Milliseconds()
	serveMetrics[avgExecTime] += float64(execTime)
	log.Infof("VM %s: returned in %d milliseconds", vmIDString, execTime)

	if resp.Payload == "Hello, replay_response!" {
		serveMetrics[realRPS]++
	}
}

// Returns a list of image names
func getImages(t *testing.T) []string {
	var (
		images = getAllImages()
		funcs  = strings.Split(*funcNames, ",")
		result []string
	)

	for _, funcName := range funcs {
		imageName, isPresent := images[funcName]
		require.True(t, isPresent, "Function is not supported")
		result = append(result, imageName)
	}

	return result
}

func writeResultToCSV(t *testing.T, metrics map[string]float64, filePath string) {
	f, err := os.OpenFile(filePath, os.O_RDWR|os.O_APPEND, 0644)
	require.NoError(t, err, "Failed opening file")
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		log.Fatalf("Failed reading file: %v", err)
	}

	titles := records[0]
	writer := csv.NewWriter(f)

	var data []string
	for _, title := range titles {
		for k, v := range metrics {
			if k == title {
				vStr := strconv.FormatFloat(v, 'E', -1, 64)
				data = append(data, vStr)
			}
		}
	}
	err = writer.Write(data)
	require.NoError(t, err, "Failed writting file")

	writer.Flush()
}
