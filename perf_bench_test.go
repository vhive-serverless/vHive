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
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ease-lab/vhive/metrics"
	"github.com/ease-lab/vhive/profile"
	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	// shared arguments
	warmUpTime   = flag.Float64("warmUpTime", 5, "The warm up time before profiling in seconds")
	coolDownTime = flag.Float64("coolDownTime", 1, "The cool down time after profiling in seconds")
	profileTime  = flag.Float64("profileTime", 10, "The profiling time in seconds")
	funcNames    = flag.String("funcNames", "helloworld", "Name of the functions to benchmark")
	isBindSocket = flag.Bool("bindSocket", false, "Bind all VMs to socket 1 on CloudLab rs440")

	vmNum           = flag.Int("vm", 2, "TestBenchRequestPerSecond: the number of VMs")
	targetReqPerSec = flag.Int("requestPerSec", 10, "TestBenchRequestPerSecond: the target number of requests per second")

	vmIncrStep = flag.Int("vmIncrStep", 4, "TestBenchMultiVMRequestPerSecond: the increment VM number for throughput benchmark")
	maxVMNum   = flag.Int("maxVMNum", 100, "TestBenchMultiVMRequestPerSecond: The maximum VM number for throughput benchmark")

	// profiler arguments
	profilerLevel    = flag.Int("l", 1, "Profile level (-l flag)")
	profilerInterval = flag.Uint64("I", 500, "Print count deltas every N milliseconds (-I flag)")
	profilerNodes    = flag.String("nodes", "", "Include or exclude nodes (with "+
		"+ to add, "+
		"-|^ to remove, "+
		"comma separated list, wildcards allowed, "+
		"add * to include all children/siblings, "+
		"add /level to specify highest level node to match, "+
		"add ^ to match related siblings and metrics, "+
		"start with ! to only include specified nodes)")
)

func TestBenchMultiVMRequestPerSecond(t *testing.T) {
	var (
		idx           int
		pinnedFuncNum int
		startVMID     int
		servedTh      uint64
		isSyncOffload bool = true
		metrFile           = "benchRPS.csv"
		images             = getImages(t)
		metrics            = make([]map[string]float64, *maxVMNum / *vmIncrStep)
	)
	log.SetLevel(log.InfoLevel)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	flrPid := pullImages(t, images)
	rps := calculateRPS(runtime.NumCPU())
	for vmNum := *vmIncrStep; vmNum <= *maxVMNum; vmNum += *vmIncrStep {
		log.Infof("vmNum: %d, Target RPS: %d", vmNum, rps)

		bootVMs(t, images, startVMID, vmNum, flrPid)
		metrics[idx] = loadAndProfile(t, images, vmNum, rps, isSyncOffload)
		startVMID = vmNum
		idx++
	}

	dumpMetrics(t, metrics, metrFile)
	profile.CSVPlotter(*vmIncrStep, *benchDir, metrFile)

	tearDownVMs(t, images, *maxVMNum, isSyncOffload)
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

	flrPid := pullImages(t, images)

	bootVMs(t, images, 0, *vmNum, flrPid)

	serveMetrics := loadAndProfile(t, images, *vmNum, *targetReqPerSec, isSyncOffload)

	tearDownVMs(t, images, *vmNum, isSyncOffload)

	dumpMetrics(t, []map[string]float64{serveMetrics}, "benchRPS.csv")
}

// Pull a list of images
func pullImages(t *testing.T, images []string) string {
	for _, imageName := range images {
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")
	}

	pidBytes, err := getFirecrackerPid()
	require.NoError(t, err, "Cannot get firecracker pid")
	pid := strings.TrimSpace(strings.Split(string(pidBytes), "\n")[0])
	return pid
}

// Boot a range of VMs with given images
func bootVMs(t *testing.T, images []string, startVMID, endVMID int, omitPid string) {
	for i := startVMID; i < endVMID; i++ {
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[i%len(images)])
		require.NoError(t, err, "Function returned error")
	}

	if *isBindSocket {
		log.Debugf("Binding socket 1")
		err := bindSocket(omitPid)
		require.NoError(t, err, "BindSocket returned error")
	}
}

// Inject many requests per second to VMs and profile
func loadAndProfile(t *testing.T, images []string, vmNum, targetRPS int, isSyncOffload bool) map[string]float64 {
	var (
		pmuMetric      *metrics.Metric
		vmGroup        sync.WaitGroup
		isProfile      = false
		cores          = runtime.NumCPU()
		threshold      = 5 * getUnloadServiceTime()
		injectDuration = *warmUpTime + *profileTime + *coolDownTime
	)

	const (
		averageExecutionTime  = "average-execution-time"
		realRequestsPerSecond = "real-requests-per-second"
	)

	for i := 1.; i < 9; i++ {
		var (
			vmID, requestID             int
			invokExecTime, realRequests int64
			serveMetric                 = metrics.NewMetric()
			rps                         = 0.1 * i * float64(targetRPS)
			timeInterval                = time.Duration(time.Second.Nanoseconds() / int64(rps))
			ticker                      = time.NewTicker(timeInterval)
			totalRequests               = injectDuration * rps
			remainingRequests           = totalRequests
			profiler                    = profile.NewProfiler(injectDuration, *profilerInterval, vmNum, *profilerLevel, *profilerNodes, "profile", *isBindSocket)
		)

		log.Debugf("RPS: %f", rps)

		tStart := time.Now()
		latencies := make(chan float64, 2)
		go latencyMeasurement(t, 1, images[1%len(images)], &isProfile, latencies)

		err := profiler.Run()
		require.NoError(t, err, "Run profiler returned error")
		go profileControl(vmNum, &isProfile, profiler)

		for remainingRequests > 0 {
			if tickerT := <-ticker.C; !tickerT.IsZero() {
				vmGroup.Add(1)
				remainingRequests--

				imageName := images[vmID%len(images)]

				go serveVM(t, &vmGroup, vmID, requestID, imageName, isSyncOffload, &isProfile, &invokExecTime, &realRequests)
				requestID++

				vmID = (vmID + 1) % vmNum
			}
		}
		ticker.Stop()
		vmGroup.Wait()
		log.Debugf("All VM returned in %d Milliseconds", time.Since(tStart).Milliseconds())
		meanLatency, percentileLatency := <-latencies, <-latencies
		log.Debug(meanLatency, percentileLatency)

		// Collect results
		serveMetric.MetricMap[averageExecutionTime] = float64(invokExecTime) / float64(realRequests)
		serveMetric.MetricMap[realRequestsPerSecond] = float64(realRequests) / (profiler.GetCoolDownTime() - profiler.GetWarmUpTime())
		if cores > vmNum {
			serveMetric.MetricMap[realRequestsPerSecond] /= float64(vmNum)
		} else {
			serveMetric.MetricMap[realRequestsPerSecond] /= float64(cores)
		}
		result, err := profiler.GetResult()
		profiledCores := profiler.GetCores()
		log.Debugf("%d cores are recorded: %v", len(profiledCores), profiledCores)
		require.NoError(t, err, "Stopping profiler returned error: %v", err)
		for eventName, value := range result {
			log.Debugf("%s: %f\n", eventName, value)
			serveMetric.MetricMap[eventName] = value
		}
		log.Debugf("average-execution-time: %f\n", serveMetric.MetricMap[averageExecutionTime])
		log.Debugf("real-requests-per-second: %f\n", serveMetric.MetricMap[realRequestsPerSecond])
		profiler.PrintBottlenecks()

		if percentileLatency > float64(threshold) {
			if pmuMetric.MetricMap == nil {
				return serveMetric.MetricMap
			}
			return pmuMetric.MetricMap
		}
		pmuMetric = serveMetric
	}

	return pmuMetric.MetricMap
}

// Goroutine function: serve VM, record real RPS and exection time
func serveVM(t *testing.T, vmGroup *sync.WaitGroup, vmID, requestID int, imageName string, isSyncOffload bool, isProfile *bool, execTime, realRequests *int64) {
	defer vmGroup.Done()

	vmIDString := strconv.Itoa(vmID)
	log.Debugf("VM %s: requestID %d", vmIDString, requestID)
	tStart := time.Now()
	resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
	require.Equal(t, resp.IsColdStart, false)
	if err == nil {
		if resp.Payload != "Hello, replay_response!" {
			log.Debugf("Function returned invalid: %s", resp.Payload)
		}
		if *isProfile {
			atomic.AddInt64(realRequests, 1)
			atomic.AddInt64(execTime, time.Since(tStart).Milliseconds())
			log.Debugf("VM %s: requestID %d completed in %d milliseconds", vmIDString, requestID, time.Since(tStart).Milliseconds())
		}
	} else {
		log.Debugf("VM %s: Function returned error %v", vmIDString, err)
	}
}

// Goroutine function: Control start and end of profiling
func profileControl(vmNum int, isProfile *bool, profiler *profile.Profiler) {
	time.Sleep(time.Duration(*warmUpTime) * time.Second)
	*isProfile = true
	profiler.SetWarmUpTime()
	log.Info("Profile started")
	time.Sleep(time.Duration(*profileTime) * time.Second)
	*isProfile = false
	profiler.SetCoolDownTime()
	log.Info("Profile finished")
}

func latencyMeasurement(t *testing.T, vmID int, imageName string, isProfile *bool, results chan float64) {
	var (
		latencies  []float64
		vmGroup    sync.WaitGroup
		ticker     = time.NewTicker(500 * time.Millisecond)
		vmIDString = strconv.Itoa(vmID)
	)

	for {
		if tickerT := <-ticker.C; *isProfile && !tickerT.IsZero() {
			vmGroup.Add(1)
			go func() {
				defer vmGroup.Done()
				tStart := time.Now()
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
				require.Equal(t, resp.IsColdStart, false)
				if err != nil {
					log.Debugf("VM %s: Function returned error %v", vmIDString, err)
				} else if resp.Payload != "Hello, replay_response!" {
					log.Debugf("Function returned invalid: %s", resp.Payload)
				}
				latencies = append(latencies, float64(time.Since(tStart).Milliseconds()))
			}()
		} else if !*isProfile && len(latencies) > 0 {
			vmGroup.Wait()
			data := stats.LoadRawData(latencies)
			mean, err := stats.Mean(data)
			require.NoError(t, err, "Compute mean returned error")
			percentile, err := stats.Percentile(data, 90)
			require.NoError(t, err, "Compute percentile returned error")
			results <- mean
			results <- percentile
			return
		}
	}
}

// Tear down VMs
func tearDownVMs(t *testing.T, images []string, vmNum int, isSyncOffload bool) {
	for i := 0; i < vmNum; i++ {
		log.Debugf("Shutting down VM %d, images: %s", i, images[i%len(images)])
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

func dumpMetrics(t *testing.T, metrics []map[string]float64, outfile string) {
	outFile := getOutFile(outfile)

	f, err := os.OpenFile(outFile, os.O_CREATE|os.O_WRONLY, 0666)
	require.NoError(t, err, "Failed opening file")
	defer f.Close()

	headerSet := make(map[string]bool)
	for _, metric := range metrics {
		for area := range metric {
			headerSet[area] = true
		}
	}

	var headers []string
	for area := range headerSet {
		headers = append(headers, area)
	}

	writer := csv.NewWriter(f)

	err = writer.Write(headers)
	require.NoError(t, err, "Failed writting file")
	writer.Flush()

	for _, metric := range metrics {
		var data []string
		for _, header := range headers {
			value, isPresent := metric[header]
			if isPresent {
				vStr := strconv.FormatFloat(value, 'f', -1, 64)
				data = append(data, vStr)
			} else {
				data = append(data, "")
			}
		}
		err = writer.Write(data)
		require.NoError(t, err, "Failed writting file")
		writer.Flush()
	}
}

// getCPUIntenseRPS returns the number of requests per second that stress CPU for each image.
func getCPUIntenseRPS() int {
	var (
		sum, result int
		values      []int
		funcs       = strings.Split(*funcNames, ",")
		reqsPerSec  = map[string]int{
			"helloworld":   10000,
			"chameleon":    700,
			"pyaes":        1000,
			"image_rotate": 600,
			"json_serdes":  600,
			"lr_serving":   5000,
			"cnn_serving":  30,
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

func getUnloadServiceTime() float64 {
	var (
		sum         float64
		funcs       = strings.Split(*funcNames, ",")
		serviceTime = map[string]float64{
			"helloworld":   1,
			"chameleon":    23,
			"pyaes":        1,
			"image_rotate": 0,
			"json_serdes":  0,
			"lr_serving":   0,
			"cnn_serving":  70,
			"rnn_serving":  0,
		}
	)

	for _, funcName := range funcs {
		sum += serviceTime[funcName]
	}

	return sum / float64(len(funcs))
}

func calculateRPS(vmNum int) int {
	baseRPS := getCPUIntenseRPS()
	return vmNum * baseRPS
}

func bindSocket(omitPid string) error {
	pidBytes, err := getFirecrackerPid()
	if err != nil {
		return err
	}

	vmPidList := strings.Split(string(pidBytes), " ")
	for _, vm := range vmPidList {
		vm = strings.TrimSpace(vm)
		if vm != omitPid {
			log.Debugf("binding pid: %s", vm)
			cmd := exec.Command("taskset", "--all-tasks", "-cp", "1-63:2", vm)
			if err := cmd.Run(); err != nil {
				return err
			}
		}
	}

	return nil
}
