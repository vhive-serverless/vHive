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
	"fmt"
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
	// arguments for TestProfileSingleConfiguration, TestProfileSingleConfiguration and TestColocateVMsOnSameCPU
	warmUpTime   = flag.Float64("warmUpTime", 5, "The warm up time before profiling in seconds")
	profileTime  = flag.Float64("profileTime", 10, "The profiling time in seconds")
	coolDownTime = flag.Float64("coolDownTime", 1, "The cool down time after profiling in seconds")
	loadStep     = flag.Float64("loadStep", 5, "The percentage of target RPS the benchmark loads at every step")
	funcNames    = flag.String("funcNames", "helloworld", "Names of the functions to benchmark, separated by comma")
	profileCPU   = flag.Int("profileCPU", -1, "Bind only one VM to the CPU and profile the physical core only")
	bindSocket   = flag.Int("bindSocket", -1, "Bind all VMs to socket number apart from the profile CPU")
	latSamples   = flag.Int("latSamples", 100, "The number of latency measurements during one profiling period")

	// arguments work for TestProfileSingleConfiguration only
	vmNum     = flag.Int("vm", 2, "TestProfileSingleConfiguration: The number of VMs")
	targetRPS = flag.Int("rps", 10, "TestProfileSingleConfiguration: The target requests per second")

	// arguments work for TestProfileIncrementConfiguration only
	vmIncrStep = flag.Int("vmIncrStep", 1, "TestProfileIncrementConfiguration: The increment VM number")
	maxVMNum   = flag.Int("maxVMNum", 100, "TestProfileIncrementConfiguration: The maximum VM number")

	// profiler arguments
	profilerLevel    = flag.Int("l", 1, "Profile level")
	profilerInterval = flag.Uint64("I", 500, "Print count deltas every N milliseconds")
	profilerNodes    = flag.String("nodes", "", "Include or exclude nodes (with "+
		"+ to add, "+
		"-|^ to remove, "+
		"comma separated list, wildcards allowed, "+
		"add * to include all children/siblings, "+
		"add /level to specify highest level node to match, "+
		"add ^ to match related siblings and metrics, "+
		"start with ! to only include specified nodes)")
)

// TestProfileIncrementConfiguration issues requests to VMs and increments VM number after loads
// start to violate time latency threshold. It also profile counters and RPS at each step. After
// iteration finishes, it saves results in bench.csv under *benchDir folder and plots each
// counters which are also saved under *benchDir folder
func TestProfileIncrementConfiguration(t *testing.T) {
	var (
		idx, rps      int
		pinnedFuncNum int
		startVMID     int
		servedTh      uint64
		isSyncOffload bool = true
		metrFile           = "bench.csv"
		images             = getImages(t)
		cores              = runtime.NumCPU()
		metrics            = make([]map[string]float64, *maxVMNum / *vmIncrStep)
	)
	log.SetLevel(log.InfoLevel)

	validateRuntimeArguments(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for vmNum := *vmIncrStep; vmNum <= *maxVMNum; vmNum += *vmIncrStep {
		if vmNum < cores {
			rps = calculateRPS(vmNum)
		} else {
			rps = calculateRPS(cores)
		}

		log.Infof("vmNum: %d, Target RPS: %d", vmNum, rps)

		bootVMs(t, images, startVMID, vmNum)
		metrics[idx] = loadAndProfile(t, images, vmNum, rps, isSyncOffload)
		startVMID = vmNum
		idx++
	}

	dumpMetrics(t, metrics, metrFile)
	profile.PlotCVS(*vmIncrStep, *benchDir, metrFile, "the number of VM")

	tearDownVMs(t, images, startVMID, isSyncOffload)
}

// TestProfileSingleConfiguration issues requests to a fixed number of VMs until requests start
// to violate tail latency threshold and then saves the results in bench.csv under *benchDir folder
func TestProfileSingleConfiguration(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		images             = getImages(t)
	)
	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	log.SetLevel(log.InfoLevel)

	validateRuntimeArguments(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	bootVMs(t, images, 0, *vmNum)

	serveMetrics := loadAndProfile(t, images, *vmNum, *targetRPS, isSyncOffload)

	tearDownVMs(t, images, *vmNum, isSyncOffload)

	dumpMetrics(t, []map[string]float64{serveMetrics}, "bench.csv")
}

// TestColocateVMsOnSameCPU measures the differences between 2 VMs on the same core and 2VMs on
// different cores. Only campatible with 45-cores machine or more cores.
func TestColocateVMsOnSameCPU(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		metrFile           = "bench.csv"
		cpuList            = []int{13, 45}
		images             = getImages(t)
		metrics            = make([]map[string]float64, 2)
	)

	log.SetLevel(log.InfoLevel)

	validateRuntimeArguments(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	bootVMs(t, images, 0, 2)

	*profileCPU = 13
	for i := 0; i < 2; i++ {
		pidBytes, err := getFirecrackerPid()
		require.NoError(t, err, "Cannot get Firecracker PID")
		vmPidList := strings.Split(string(pidBytes), " ")
		for i, vm := range vmPidList {
			vm = strings.TrimSpace(vm)
			err := bindProcessToCPU(strconv.Itoa(cpuList[i]), vm)
			require.NoError(t, err, "Cannot run taskset")
		}

		metrics[i] = loadAndProfile(t, images, 2, calculateRPS(2), isSyncOffload)
		cpuList[1] = 15
	}

	dumpMetrics(t, metrics, metrFile)

	profile.PlotCVS(1, *benchDir, metrFile, "the number of cores")

	tearDownVMs(t, images, 2, isSyncOffload)
}

func TestBindSocket(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		testImage          = []string{"vhiveease/helloworld:var_workload"}
	)

	type testCase struct {
		vmNum    int
		expected []string
	}

	log.SetLevel(log.InfoLevel)

	*profileCPU = 0
	*bindSocket = 0

	socketCPUs, err := getSocketCPUs()
	require.NoError(t, err, "Cannot get CPUs of the socket")
	subCPUs, err := deleteProfileCore(socketCPUs)
	require.NoError(t, err, "Cannot delete profile core from the CPU list")

	cases := []testCase{
		{vmNum: 1, expected: []string{strconv.Itoa(*profileCPU)}},
		{vmNum: 4, expected: []string{strconv.Itoa(*profileCPU), strings.Join(subCPUs, ",")}},
	}

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	for _, tCase := range cases {
		testName := fmt.Sprintf("vmNum=%d", tCase.vmNum)
		t.Run(testName, func(t *testing.T) {
			bootVMs(t, testImage, 0, tCase.vmNum)

			pidBytes, err := getFirecrackerPid()
			require.NoError(t, err, "Cannot get Firecracker PID")
			vmPidList := strings.Split(string(pidBytes), " ")

			cpuBytes, err := exec.Command("taskset", "-cp", strings.TrimSpace(vmPidList[0])).Output()
			require.NoError(t, err, "Cannot get CPU affinity")
			cpuAffinity := strings.TrimSpace(strings.Split(string(cpuBytes), ":")[1])
			require.Equal(t, tCase.expected[0], cpuAffinity, "VM was not binded correctly")
			for _, vm := range vmPidList[1:] {
				vm = strings.TrimSpace(vm)
				cpuBytes, err := exec.Command("taskset", "-cp", vm).Output()
				require.NoError(t, err, "Cannot get CPU affinity")
				cpuAffinity := strings.TrimSpace(strings.Split(string(cpuBytes), ":")[1])
				require.Equal(t, tCase.expected[1], cpuAffinity, "VM was not binded correctly")
			}

			tearDownVMs(t, testImage, tCase.vmNum, isSyncOffload)
		})
	}
}

// bootVMs boots a range of VMs with given images
func bootVMs(t *testing.T, images []string, startVMID, endVMID int) {
	for i := startVMID; i < endVMID; i++ {
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[i%len(images)])
		require.NoError(t, err, "Function returned error")
	}

	if *profileCPU > -1 || *bindSocket > -1 {
		log.Debugf("Binding VMs")
		err := bindVMsToSocket()
		require.NoError(t, err, "Bind Socket returned error")
	}
}

// loadAndProfile issues requests at B% to 100% of the maximum RPS (measured separately), where B is *loadStep/100
// and collects counters iteratively
func loadAndProfile(t *testing.T, images []string, vmNum, targetRPS int, isSyncOffload bool) map[string]float64 {
	var (
		pmuMetric      *metrics.Metric
		vmGroup        sync.WaitGroup
		isProfile      = false
		cores          = runtime.NumCPU()
		stepSize       = *loadStep / 100.
		threshold      = 10 * getUnloadedServiceTime() // for the constraint of tail latency
		injectDuration = *warmUpTime + *profileTime + *coolDownTime
	)

	// the constants for metric names
	const (
		avgExecTime = "average-execution-time"
		rpsPerCore  = "RPS-per-Core"
	)

	for step := stepSize; step < 1+stepSize; step += stepSize {
		var (
			vmID, requestID             int
			invokExecTime, realRequests int64
			serveMetric                 = metrics.NewMetric()
			rps                         = int64(step * float64(targetRPS))
			totalRequests               = injectDuration * float64(rps)
			remainingRequests           = totalRequests
		)

		if rps <= 0 {
			continue
		}
		profiler, err := profile.NewProfiler(injectDuration, *profilerInterval, vmNum, *profilerLevel,
			*profilerNodes, "profile", *profileCPU)
		require.NoError(t, err, "Cannot create a profiler instance")
		ticker := time.NewTicker(time.Duration(time.Second.Nanoseconds() / rps))

		log.Infof("Current RPS: %d", rps)

		tStart := time.Now()
		latencyCh := make(chan LatencyStat)
		go measureTailLatency(t, vmNum, images, &isProfile, latencyCh)

		err = profiler.Run()
		require.NoError(t, err, "Run profiler returned error")
		go configureProfiler(&isProfile, profiler)

		for remainingRequests > 0 {
			if tickerT := <-ticker.C; !tickerT.IsZero() {
				vmGroup.Add(1)
				remainingRequests--

				imageName := images[vmID%len(images)]

				go loadVMs(t, &vmGroup, vmID, requestID, imageName, isSyncOffload, &isProfile, &invokExecTime, &realRequests)
				requestID++

				vmID = (vmID + 1) % vmNum
			}
		}
		ticker.Stop()
		vmGroup.Wait()
		log.Debugf("All VM returned in %d Milliseconds", time.Since(tStart).Milliseconds())
		latencies := <-latencyCh
		log.Debugf("Mean Latency: %.2f, Tail Latency: %.2f", latencies.meanLatency, latencies.tailLatency)

		// Collect results
		serveMetric.MetricMap[avgExecTime] = float64(invokExecTime) / float64(realRequests)
		serveMetric.MetricMap[rpsPerCore] = float64(realRequests) / (profiler.GetCoolDownTime() - profiler.GetWarmUpTime())
		if cores > vmNum {
			serveMetric.MetricMap[rpsPerCore] /= float64(vmNum)
		} else {
			serveMetric.MetricMap[rpsPerCore] /= float64(cores)
		}
		result, err := profiler.GetResult()
		profiledCores := profiler.GetCores()
		log.Debugf("%d cores are recorded: %v", len(profiledCores), profiledCores)
		require.NoError(t, err, "Stopping profiler returned error: %v", err)
		for eventName, value := range result {
			log.Debugf("%s: %.2f", eventName, value)
			serveMetric.MetricMap[eventName] = value
		}
		log.Debugf("%s: %.2f", avgExecTime, serveMetric.MetricMap[avgExecTime])
		log.Debugf("%s: %.2f", rpsPerCore, serveMetric.MetricMap[rpsPerCore])
		profiler.PrintBottlenecks()

		// if tail latency violates the threshold that it should be less than 10x service time,
		// it returns the metric before tail latency violation.
		if latencies.tailLatency > threshold {
			require.NotNil(t, pmuMetric, "The tail latency of first round is larger than the constraint")
			return pmuMetric.MetricMap
		}

		pmuMetric = serveMetric
	}

	return pmuMetric.MetricMap
}

// loadVMs load requests to VMs every second and records completed requests and exection time
func loadVMs(t *testing.T, vmGroup *sync.WaitGroup, vmID, requestID int, imageName string,
	isSyncOffload bool, isProfile *bool, execTime, realRequests *int64) {
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

// configureProfiler controls the time duration of profiling for the loadVMs function and the profiler
func configureProfiler(isProfile *bool, profiler *profile.Profiler) {
	time.Sleep(time.Duration(*warmUpTime) * time.Second)
	*isProfile = true
	profiler.SetWarmUpTime()
	log.Debug("Profile started")
	time.Sleep(time.Duration(*profileTime) * time.Second)
	*isProfile = false
	profiler.SetCoolDownTime()
	log.Debug("Profile finished")
}

type LatencyStat struct {
	meanLatency float64
	tailLatency float64
}

// measureTailLatency measures tail latency by sampling at least every 500ms for a VM
func measureTailLatency(t *testing.T, vmNum int, images []string, isProfile *bool, latencyCh chan LatencyStat) {
	var (
		vmID         int
		serviceTimes []float64
		vmGroup      sync.WaitGroup
		duraInMs     = *profileTime * 1000 / float64(*latSamples)
	)
	if duraInMs*float64(vmNum) < 500 {
		duraInMs = 500
		log.Warnf("Too many latency samples for %d VMs, measure %.0f samples instead.", vmNum, *profileTime*1000/duraInMs)
	}
	duration := time.Duration(duraInMs)
	ticker := time.NewTicker(duration * time.Millisecond)

	for {
		if tickerT := <-ticker.C; *isProfile && !tickerT.IsZero() {
			vmGroup.Add(1)
			go func(vmID int) {
				defer vmGroup.Done()
				var (
					tStart     = time.Now()
					vmIDString = strconv.Itoa(vmID)
				)
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, images[vmID%len(images)], "replay")
				require.Equal(t, resp.IsColdStart, false)
				if err != nil {
					log.Debugf("VM %s: Function returned error %v", vmIDString, err)
				} else if resp.Payload != "Hello, replay_response!" {
					log.Debugf("Function returned invalid: %s", resp.Payload)
				} else {
					serviceTimes = append(serviceTimes, float64(time.Since(tStart).Milliseconds()))
				}
			}(vmID)
			vmID = (vmID + 1) % vmNum
		} else if !*isProfile && len(serviceTimes) > 0 {
			vmGroup.Wait()
			data := stats.LoadRawData(serviceTimes)
			mean, err := stats.Mean(data)
			require.NoError(t, err, "Compute mean returned error")
			percentile, err := stats.Percentile(data, 90)
			require.NoError(t, err, "Compute 90 percentile returned error")
			latencyCh <- LatencyStat{
				meanLatency: mean,
				tailLatency: percentile,
			}
			return
		}
	}
}

// tearDownVMs removes instances from 0 to input VM number
func tearDownVMs(t *testing.T, images []string, vmNum int, isSyncOffload bool) {
	for i := 0; i < vmNum; i++ {
		log.Infof("Shutting down VM %d, images: %s", i, images[i%len(images)])
		vmIDString := strconv.Itoa(i)
		message, err := funcPool.RemoveInstance(vmIDString, images[i%len(images)], isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
	}
}

// getImages returns of the supported images' names
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

// dumpMetrics writes metrics to a file
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
func getCloseLoopRPS() int {
	var (
		sum, result int
		values      []int
		funcs       = strings.Split(*funcNames, ",")
		reqsPerSec  = map[string]int{
			"helloworld":   1000,
			"chameleon":    85,
			"pyaes":        1000,
			"image_rotate": 333,
			"json_serdes":  167,
			"lr_serving":   1000,
			"cnn_serving":  20,
			"rnn_serving":  100,
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

func getUnloadedServiceTime() float64 {
	var (
		sum         float64
		funcs       = strings.Split(*funcNames, ",")
		serviceTime = map[string]float64{
			"helloworld":   1,
			"chameleon":    12,
			"pyaes":        1,
			"image_rotate": 3,
			"json_serdes":  6,
			"lr_serving":   1,
			"cnn_serving":  60,
			"rnn_serving":  10,
		}
	)

	for _, funcName := range funcs {
		sum += serviceTime[funcName]
	}

	return sum / float64(len(funcs))
}

func calculateRPS(vmNum int) int {
	baseRPS := getCloseLoopRPS()
	return vmNum * baseRPS
}

func bindVMsToSocket() error {
	var (
		coreFree = true
		cores    = runtime.NumCPU()
	)
	pidBytes, err := getFirecrackerPid()
	if err != nil {
		return err
	}

	socketCPUs, err := getSocketCPUs()
	if err != nil {
		return err
	}

	subSocketCPUs, err := deleteProfileCore(socketCPUs)
	if err != nil {
		return err
	}

	vmPidList := strings.Split(string(pidBytes), " ")
	for _, vm := range vmPidList {
		vm = strings.TrimSpace(vm)
		if cores > len(vmPidList) {
			if coreFree && *profileCPU > -1 {
				if err := bindProcessToCPU(strconv.Itoa(*profileCPU), vm); err != nil {
					return err
				}
				coreFree = false
			} else {
				if err := bindProcessToCPU(strings.Join(subSocketCPUs, ","), vm); err != nil {
					return err
				}
			}
		} else {
			if err := bindProcessToCPU(strings.Join(socketCPUs, ","), vm); err != nil {
				return err
			}
		}
	}

	return nil
}

func bindProcessToCPU(processors, pid string) error {
	log.Debugf("binding pid %s to processor %s", pid, processors)
	if err := exec.Command("taskset", "--all-tasks", "-cp", processors, pid).Run(); err != nil {
		return err
	}

	return nil
}

func getSocketCPUs() ([]string, error) {
	var cpus []string
	allCPUs, err := profile.GetAllCPUs()
	if err != nil {
		return nil, err
	}

	for _, cpu := range allCPUs {
		if cpu.Socket == strconv.Itoa(*bindSocket) {
			cpus = append(cpus, cpu.Processor)
		}
	}

	return cpus, nil
}

func deleteProfileCore(cpus []string) ([]string, error) {
	var subList []string
	if *profileCPU > -1 {
		profileCore, err := profile.GetPhysicalCore(*profileCPU)
		if err != nil {
			return nil, err
		}

		for _, cpuStr := range cpus {
			cpuID, err := strconv.Atoi(cpuStr)
			physicalCore, err := profile.GetPhysicalCore(cpuID)
			if err != nil {
				return nil, err
			}
			if physicalCore != profileCore {
				subList = append(subList, cpuStr)
			}
		}
	} else {
		return cpus, nil
	}

	return subList, nil
}

func validateRuntimeArguments(t *testing.T) {
	var cores = runtime.NumCPU()
	require.Truef(t, *profileTime >= 0, "Profile time = %.2f must be no less than 0s", *profileTime)
	require.Truef(t, *warmUpTime >= 0, "Warm-up time = %.2f must be no less than 0s", *warmUpTime)
	require.Truef(t, *coolDownTime >= 0, "Cool-down time = %.2f must be no less than 0s", *coolDownTime)
	require.Truef(t, *profilerInterval >= 10, "Profiler print interval = %d must be no less than 10ms", *profilerInterval)
	require.Truef(t, *profilerLevel > 0, "Profiler level = %d must be more than 0", *profilerLevel)
	require.Truef(t, *vmNum > 0, "VM number = %d must be more than 0", *vmNum)
	require.Truef(t, *targetRPS >= 0, "RPS = %d must be no less than 0", *targetRPS)
	require.Truef(t, *vmIncrStep >= 0, "Increment step of VM number = %d must be no less than 0", *vmIncrStep)
	require.Truef(t, *maxVMNum >= 0, "Maximum VM number = %d must be no less than 0", *maxVMNum)
	require.Truef(t, *maxVMNum >= *vmIncrStep, "Maximum VM number = %d must be no less than increment step = %d", *maxVMNum, *vmIncrStep)
	require.Truef(t, *loadStep > 0 && *loadStep <= 100, "Load step = %.2f must be between 0 and 1", *loadStep)
	require.Truef(t, *profileCPU < cores, "profile CPUID = %d must be smaller than the number of cores %d", *profileCPU, cores)
	sockets, err := getAvailableSocketNum()
	require.NoError(t, err, "Cannot get the number of available nodes")
	require.Truef(t, *bindSocket < sockets, "Socket %d must be smaller than the number of nodes %d", *bindSocket, sockets)
	require.Truef(t, *latSamples > 0 && *latSamples < 101, "Latency samples = %d must be between 0 and 100", *latSamples)
}

func getAvailableSocketNum() (int, error) {
	sockets := make(map[string]bool)
	allCPUs, err := profile.GetAllCPUs()
	if err != nil {
		return 0, err
	}

	for _, cpu := range allCPUs {
		sockets[cpu.Socket] = true
	}

	return len(sockets), nil
}
