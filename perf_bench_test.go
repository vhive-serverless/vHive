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
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	isColdStart     = flag.Bool("coldStart", false, "Profile cold starts (default is false)")
	vmNum           = flag.Int("vm", 2, "The number of VMs")
	targetReqPerSec = flag.Int("requestPerSec", 10, "The target number of requests per second")
	executionTime   = flag.Int("executionTime", 1, "The execution time of the benchmark in seconds")
	funcNames       = flag.String("funcNames", "helloworld", "Name of the functions to benchmark")
	perfEvents      = flag.String("perfEvents", "", "Perf events (run `perf stat` if not empty)")

	envPath = "PATH=" + os.Getenv("PATH")
)

const (
	AveExecTime = "AveExecTime"
	RealRPS     = "RealRPS"
)

// TODO: Change this from test function to normal function, and move to another file
func TestBenchMultiVMRequestPerSecond(t *testing.T) {
	log.SetLevel(log.InfoLevel)

	for i := 4; i <= 4; i += 4 {
		err := setupBenchRPS()
		require.NoError(t, err, "Setup TestBenchRequestPerSecond error")

		//Subtest
		testName := fmt.Sprintf("Test_%dVM", i)
		*targetReqPerSec = 10
		*perfEvents = "instructions,LLC-loads,LLC-load-misses"
		*vmNum = i
		*funcNames = "rnn_serving"

		testResult := t.Run(testName, TestBenchRequestPerSecond)
		require.True(t, testResult)

		err = teardownBenchRPS()
		require.NoError(t, err, "Teardown TestBenchRequestPerSecond error")
	}
}

func setupBenchRPS() error {
	// sudo mkdir -m777 -p /tmp/ctrd-logs && sudo env "PATH=$PATH" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &
	var CTRDLOGDIR = "/tmp/ctrd-logs"

	if err := os.MkdirAll(CTRDLOGDIR, 0777); err != nil {
		return err
	}

	commandString := "sudo env " + envPath + " /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &"
	cmd := exec.Command("sudo", "/bin/sh", "-c", commandString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func teardownBenchRPS() error {
	// ./scripts/clean_fcctr.sh
	cmd := exec.Command("sudo", "/bin/sh", "-c", "./scripts/clean_fcctr.sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

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
		perfStat           = NewPerfStat(AllCPUs, Event, *perfEvents, Output, "perf-tmp.data")
	)

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
		perfStat.RunPerfStat()
		log.Info("Perf starts")
	}

	var vmGroup sync.WaitGroup
	ticker := time.NewTicker(timeInterval)
	tickerDone := make(chan bool, 1)

	serveMetrics := make(map[string]float64)
	serveMetrics[RealRPS] = 0
	serveMetrics[AveExecTime] = 0

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
		result := perfStat.StopPerfStat()
		log.Info("Perf stops")
		for eventName, value := range result {
			serveMetrics[eventName] = value
		}
	}

	serveMetrics[AveExecTime] /= float64(totalRequests)
	log.Debugf("RESULTS: %f, %f, %f, %f, %f", serveMetrics[AveExecTime], float64(totalRequests), serveMetrics[RealRPS], serveMetrics["LLC-loads"], serveMetrics["LLC-load-misses"])
}

func serveVM(t *testing.T, vmIDString, imageName string, vmGroup *sync.WaitGroup, isSyncOffload bool, serveMetrics map[string]float64) {
	defer vmGroup.Done()

	tStart := time.Now()
	resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.IsColdStart, false)

	execTime := time.Since(tStart).Milliseconds()
	serveMetrics[AveExecTime] += float64(execTime)
	log.Debugf("VM %s: returned in %d milliseconds", vmIDString, execTime)

	if resp.Payload == "Hello, replay_response!" {
		serveMetrics[RealRPS]++
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

func saveResult() {

}
