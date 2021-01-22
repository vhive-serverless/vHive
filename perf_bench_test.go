// MIT License
//
// Copyright (c) 2020 Yuchen Niu and EASE lab
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/ease-lab/vhive/profile"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	// shared arguments
	injectDuration = flag.Int("injectTimeDuration", 1, "The total time in seconds for injecting requests each round")
	funcNames      = flag.String("funcNames", "helloworld", "Name of the functions to benchmark")

	vmNum           = flag.Int("vm", 2, "TestBenchRequestPerSecond: the number of VMs")
	targetReqPerSec = flag.Int("requestPerSec", 10, "TestBenchRequestPerSecond: the target number of requests per second")

	vmIncrStep = flag.Int("vmIncrStep", 4, "TestBenchMultiVMRequestPerSecond: the increment VM number for throughput benchmark")
	maxVMNum   = flag.Int("maxVMNum", 100, "TestBenchMultiVMRequestPerSecond: The maximum VM number for throughput benchmark")

	// perf arguments
	perfExecTime = flag.Float64("perfExecTime", 30, "The execution time of perf command in seconds (sleep command)")
	perfInterval = flag.Uint64("perfInterval", 500, "Print count deltas every N milliseconds (-I flag)")
	perfEvents   = flag.String("perfEvents", "", "Perf events (-e flag)")
	perfMetrics  = flag.String("perfMetrics", "", "Perf metrics")
)

func TestBenchMultiVMRequestPerSecond(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		startVMID     int
		isSyncOffload bool = true
		metrFile           = "benchRPS.csv"
		images             = getImages(t)
		baseRPS            = getCPUIntenseRPS()
	)
	log.SetLevel(log.InfoLevel)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	pullImages(t, images)

	for vmNum := *vmIncrStep; vmNum <= *maxVMNum; vmNum += *vmIncrStep {
		rps := vmNum * baseRPS
		log.Infof("vmNum: %d, RPS: %d", vmNum, rps)

		bootVMs(t, images, startVMID, vmNum)
		metr := loadAndProfile(t, images, vmNum, rps, *injectDuration, isSyncOffload)
		dumpMetrics(t, metr, metrFile)
		startVMID = vmNum
	}

	tearDownVMs(t, images, *maxVMNum, isSyncOffload)

	profile.CSVPlotter(*benchDir, metrFile)
}

func TestBenchRequestPerSecond(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		images             = getImages(t)
	)

	log.SetLevel(log.InfoLevel)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	pullImages(t, images)

	bootVMs(t, images, 0, *vmNum)

	serveMetrics := loadAndProfile(t, images, *vmNum, *targetReqPerSec, *injectDuration, isSyncOffload)

	tearDownVMs(t, images, *vmNum, isSyncOffload)

	dumpMetrics(t, serveMetrics, "benchRPS.csv")
}

// Pull a list of images
func pullImages(t *testing.T, images []string) {
	for _, imageName := range images {
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")
	}
}

// Boot a range of VMs with given images
func bootVMs(t *testing.T, images []string, startVMID, endVMID int) {
	for i := startVMID; i < endVMID; i++ {
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[i%len(images)])
		require.NoError(t, err, "Function returned error")
	}
}

// Inject many requests per second to VMs and profile
func loadAndProfile(t *testing.T, images []string, vmNum, targetRPS, injectDuration int, isSyncOffload bool) map[string]float64 {
	var (
		vmID                   int
		execTime, realRequests int64
		vmGroup                sync.WaitGroup
		timeInterval           = time.Duration(time.Second.Nanoseconds() / int64(targetRPS))
		totalRequests          = injectDuration * targetRPS
		remainingRequests      = totalRequests
		ticker                 = time.NewTicker(timeInterval)
		perfStat               = profile.NewPerfStat(*perfExecTime, *perfInterval, *perfEvents, *perfMetrics, "perf-tmp.data")
	)

	const (
		averageExecutionTime  = "average-execution-time"
		realRequestsPerSecond = "real-requests-per-second"
	)

	log.Info("Load starts")
	tStart := time.Now()
	err := perfStat.Run()
	require.NoError(t, err, "Start perf stat returned error")

	for remainingRequests > 0 {
		if tickerT := <-ticker.C; !tickerT.IsZero() {
			vmGroup.Add(1)
			remainingRequests--

			imageName := images[vmID%len(images)]
			vmIDString := strconv.Itoa(vmID)

			go serveVM(t, &vmGroup, vmIDString, imageName, isSyncOffload, &execTime, &realRequests)

			vmID = (vmID + 1) % vmNum
			if vmID == 0 {
				perfStat.SetWarmTime()
			}
		}
	}
	ticker.Stop()
	vmGroup.Wait()
	perfStat.SetTearDownTime()
	log.Infof("All VM returned in %d Milliseconds", time.Since(tStart).Milliseconds())

	// Collect results
	serveMetrics := make(map[string]float64)
	serveMetrics[averageExecutionTime] = float64(execTime) / float64(totalRequests)
	serveMetrics[realRequestsPerSecond] = float64(realRequests) / float64(injectDuration)
	result, err := perfStat.GetResult()
	require.NoError(t, err, "Stop perf stat returned error: %v", err)
	for eventName, value := range result {
		log.Debugf("%s: %f\n", eventName, value)
		serveMetrics[eventName] = value
	}
	log.Debugf("average-execution-time: %f\n", serveMetrics[averageExecutionTime])
	log.Debugf("real-requests-per-second: %f\n", serveMetrics[realRequestsPerSecond])

	return serveMetrics
}

// Goroutine function: serve VM, record real RPS and exection time
func serveVM(t *testing.T, vmGroup *sync.WaitGroup, vmIDString, imageName string, isSyncOffload bool, execTime, realRequests *int64) {
	defer vmGroup.Done()

	tStart := time.Now()
	resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
	require.Equal(t, resp.IsColdStart, false)
	if err != nil {
		log.Warnf("VM %s: Function returned error %v", vmIDString, err)
	} else {
		if resp.Payload != "Hello, replay_response!" {
			log.Warnf("Function returned invalid: %s", resp.Payload)
		}
		atomic.AddInt64(realRequests, 1)
	}

	atomic.AddInt64(execTime, time.Since(tStart).Milliseconds())
	log.Debugf("VM %s: returned in %d milliseconds", vmIDString, execTime)
}

// Tear down VMs
func tearDownVMs(t *testing.T, images []string, vmNum int, isSyncOffload bool) {
	for i := 0; i < vmNum; i++ {
		vmIDString := strconv.Itoa(i)
		message, err := funcPool.RemoveInstance(vmIDString, images[i%len(images)], isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
	}
}

// Returns a list of image names
func getImages(t *testing.T) []string {
	var (
		images = map[string]string{
			"helloworld":   "vhiveease/helloworld:var_workload",
			"chameleon":    "vhiveease/chameleon:var_workload",
			"pyaes":        "vhiveease/pyaes:var_workload",
			"image_rotate": "vhiveease/image_rotate:var_workload",
			"json_serdes":  "vhiveease/json_serdes:var_workload",
			"lr_serving":   "vhiveease/lr_serving:var_workload",
			"cnn_serving":  "vhiveease/cnn_serving:var_workload",
			"rnn_serving":  "vhiveease/rnn_serving:var_workload",
		}
		funcs  = strings.Split(*funcNames, ",")
		result []string
	)

	for _, funcName := range funcs {
		imageName, isPresent := images[funcName]
		require.True(t, isPresent, "Function %s is not supported", funcName)
		result = append(result, imageName)
	}

	return result
}

func dumpMetrics(t *testing.T, metrics map[string]float64, outfile string) {
	outFile := getOutFile(outfile)

	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0666)
	require.NoError(t, err, "Failed opening file")
	defer f.Close()

	reader := csv.NewReader(f)
	headers, err := reader.Read()
	require.True(t, err == nil || err.Error() == "EOF", "Failed reading file")

	writer := csv.NewWriter(f)
	if headers == nil {
		for k := range metrics {
			headers = append(headers, k)
		}
		err = writer.Write(headers)
		require.NoError(t, err, "Failed writting file")
		writer.Flush()
	}

	var data []string
	for _, header := range headers {
		for k, v := range metrics {
			if k == header {
				vStr := strconv.FormatFloat(v, 'f', -1, 64)
				data = append(data, vStr)
			}
		}
	}
	err = writer.Write(data)
	require.NoError(t, err, "Failed writting file")

	writer.Flush()
}

// getCPUIntenseRPS returns the number of requests per second that stress CPU for each image.
func getCPUIntenseRPS() int {
	var (
		sum, result int
		values      []int
		funcs       = strings.Split(*funcNames, ",")
		reqsPerSec  = map[string]int{
			"helloworld":   10000,
			"chameleon":    400,
			"pyaes":        2500,
			"image_rotate": 600,
			"json_serdes":  600,
			"lr_serving":   5000,
			"cnn_serving":  200,
			"rnn_serving":  600,
		}
	)

	for _, funcName := range funcs {
		values = append(values, reqsPerSec[funcName])
		sum += reqsPerSec[funcName]
	}

	for _, rps := range values {
		result += rps * rps / sum
	}

	return result
}
