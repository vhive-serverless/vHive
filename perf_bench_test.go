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
	"math"
	"os"
	"os/exec"
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
	loadStep     = flag.Int("loadStep", 5, "The percentage of target RPS the benchmark loads at every step")
	funcNames    = flag.String("funcNames", "helloworld", "Names of the functions to benchmark, separated by comma")
	// *profileCPUID allocates only one VM to the core and profiler only collects counters from the core.
	profileCPUID = flag.Int("profileCPUID", -1, "Bind one VM to the core of the CPU and profile the core only")
	// *bindSocket binds all VMs to a socket. If *profileCPUID is set at the same time, the *profileCPUID
	// must be in the socket. Then, only one VM runs on the *profileCPUID and others run on other cores.
	bindSocket = flag.Int("bindSocket", -1, "Bind all VMs to socket number apart from the profile CPU")
	latSamples = flag.Int("latSamples", 100, "The number of latency measurements during one profiling period")
	isTest     = flag.Bool("test", false, "Tail latency threshold is ignored if test is true")

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
		"comma separated list, wildcards allowed [+Fetch_Latency,-Backend_Bound], "+
		"add * to include all children/siblings [+Frontend_Bound*], "+
		"add /level to specify highest level node to match [+Frontend_Bound*/2], "+
		"add ^ to match related siblings and metrics [+Frontend_Bound^], "+
		"start with ! to only include specified nodes ['!Frontend_Bound'])")
)

// TestProfileIncrementConfiguration issues requests to VMs and increments VM number after requests
// start to violate time latency threshold. It also profile counters and RPS at each step. After
// iteration finishes, it saves results in bench.csv under *benchDir folder and plots each
// counters which are also saved under *benchDir folder
func TestProfileIncrementConfiguration(t *testing.T) {

	//t.Skip("Skipping TestProfileIncrementConfiguration")

	var (
		idx, rps      int
		pinnedFuncNum int
		startVMID     int
		servedTh      uint64
		isSyncOffload bool = true
		metrFile           = "bench.csv"
		images             = getImages(t)
		metrics            = make([]map[string]float64, *maxVMNum / *vmIncrStep)
	)
	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	cores, err := cpuNum()
	require.NoError(t, err, "Cannot get the number of CPU")
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
	profile.PlotLineCharts(*vmIncrStep, *benchDir, metrFile, "the number of tenants")
	profile.PlotStackCharts(*vmIncrStep, "profile/toplev_metrics.json", *benchDir, metrFile, "the number of tenants")

	tearDownVMs(t, images, startVMID, isSyncOffload)
}

// TestProfileSingleConfiguration issues requests to a fixed number of VMs until requests start
// to violate tail latency threshold and then saves the results in bench.csv under *benchDir folder
func TestProfileSingleConfiguration(t *testing.T) {

	//t.Skip("Skipping TestProfileSingleConfiguration")

	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		images             = getImages(t)
	)

	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	bootVMs(t, images, 0, *vmNum)

	serveMetrics := loadAndProfile(t, images, *vmNum, *targetRPS, isSyncOffload)

	tearDownVMs(t, images, *vmNum, isSyncOffload)

	dumpMetrics(t, []map[string]float64{serveMetrics}, "bench.csv")
}

// TestColocateVMsOnSameCPU measures the differences between 2 VMs on the same core and 2VMs on different cores,
// controlled by *profileCoreID
func TestColocateVMsOnSameCPU(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		metrFile           = "bench.csv"
		images             = getImages(t)
		metrics            = make([]map[string]float64, 2)
	)

	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	bootVMs(t, images, 0, 2)

	metrics[0] = loadAndProfile(t, images, 2, calculateRPS(2), isSyncOffload)

	cpuInfo, err := profile.GetCPUInfo()
	require.NoError(t, err, "Cannot get CPU info")
	sibling, err := cpuInfo.GetSibling(*profileCPUID)
	require.EqualErrorf(t, err, "processor does not have a sibling", "Invalid input processor ID")
	// SMT is off, set sibling to its own
	if err != nil {
		sibling = *profileCPUID
	}
	cpuList := []int{*profileCPUID, sibling}
	pidBytes, err := getFirecrackerPid()
	require.NoError(t, err, "Cannot get Firecracker PID")
	vmPidList := strings.Split(string(pidBytes), " ")
	for i, vm := range vmPidList {
		vm = strings.TrimSpace(vm)
		err := bindProcessToCPU(vm, cpuList[i])
		require.NoError(t, err, "Cannot run taskset")
	}
	metrics[1] = loadAndProfile(t, images, 2, calculateRPS(2), isSyncOffload)

	dumpMetrics(t, metrics, metrFile)

	profile.PlotLineCharts(1, *benchDir, metrFile, "Different Cores -> Same Core")

	tearDownVMs(t, images, 2, isSyncOffload)
}

func TestBindSocket(t *testing.T) {
	var (
		procStr, sep  string
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		testImage          = []string{"ghcr.io/ease-lab/helloworld:var_workload"}
	)

	type testCase struct {
		vmNum    int
		expected []string
	}

	log.SetLevel(log.InfoLevel)

	*profileCPUID = 0
	*bindSocket = 0

	cpuInfo, err := profile.GetCPUInfo()
	require.NoError(t, err, "Cannot get CPU info")
	socketCPUs, err := cpuInfo.SocketCPUs(*bindSocket)
	require.NoError(t, err, "Cannot get CPUs of the socket")
	for _, proc := range socketCPUs {
		procStr += sep + strconv.Itoa(proc)
		sep = ","
	}
	cases := []testCase{
		{vmNum: 1, expected: []string{strconv.Itoa(*profileCPUID)}},
		{vmNum: 4, expected: []string{strconv.Itoa(*profileCPUID), procStr}},
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
				var result, sep string
				if strings.ContainsAny(cpuAffinity, "-") {
					subSets := strings.Split(cpuAffinity, ",")
					for _, t := range subSets {
						set := strings.Split(t, "-")
						start, _ := strconv.Atoi(set[0])
						end, _ := strconv.Atoi(set[1])
						for i := start; i < end+1; i++ {
							result += sep + strconv.Itoa(i)
							sep = ","
						}
					}
				} else {
					result = cpuAffinity
				}
				require.Equal(t, tCase.expected[1], result, "VM was not binded correctly")
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

	if *profileCPUID > -1 || *bindSocket > -1 {
		log.Debugf("Binding VMs")
		err := bindVMs(*bindSocket, *profileCPUID)
		require.NoError(t, err, "Bind Socket returned error")
	}
}

// loadAndProfile issues requests at B% to 100% of the maximum RPS (measured separately), where B is *loadStep/100
// and collects counters iteratively
func loadAndProfile(t *testing.T, images []string, vmNum, targetRPS int, isSyncOffload bool) map[string]float64 {
	var (
		pmuMetric      *metrics.Metric
		vmGroup        sync.WaitGroup
		threshold      float64 // for the constraint of tail latency
		isProfile      int32   = 0
		stepSize               = float64(*loadStep) / 100
		injectDuration         = *warmUpTime + *profileTime + *coolDownTime
	)

	// the constants for metric names
	const (
		avgExecTime = "Average_execution_time"
		rpsPerCPU   = "RPS_per_CPU"
		rpsHost     = "Overall_RPS"
	)

	if *isTest {
		threshold = math.MaxInt64 * getUnloadedServiceTime()
	} else {
		threshold = 10 * getUnloadedServiceTime()
	}

	cpus, err := cpuNum()
	require.NoError(t, err, "Cannot get the number of CPU")
	log.Debugf("CPU number is %d", cpus)
	for step := stepSize; step < 1+stepSize; step += stepSize {
		var (
			vmID, requestID             int
			invokExecTime, realRequests int64
			serveMetric                 = metrics.NewMetric()
			rps                         = int64(step * float64(targetRPS))
			totalRequests               = injectDuration * float64(rps)
			remainingRequests           = totalRequests
			latencyCh                   = make(chan LatencyStat)
			profileCh                   = make(chan bool)
		)
		if rps <= 0 {
			log.Debugf("Current RPS %d is less than 0. Skip this step", rps)
			continue
		}
		profiler, err := profile.NewProfiler(injectDuration, *profilerInterval, *profilerLevel,
			*profilerNodes, "profile", *bindSocket, *profileCPUID)
		require.NoError(t, err, "Cannot create a profiler instance")
		ticker := time.NewTicker(time.Duration(time.Second.Nanoseconds() / rps))

		log.Infof("Current RPS: %d", rps)

		tStart := time.Now()
		go measureTailLatency(t, vmNum, images, latencyCh)

		err = profiler.Run()
		require.NoError(t, err, "Run profiler returned error")
		go configureProfiler(&isProfile, profiler, profileCh)

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
		<-profileCh

		// Collect results
		serveMetric.MetricMap[avgExecTime] = float64(invokExecTime) / float64(realRequests)
		serveMetric.MetricMap[rpsHost] = float64(realRequests) / (profiler.GetCoolDownTime() - profiler.GetWarmUpTime())
		if cpus > vmNum {
			serveMetric.MetricMap[rpsPerCPU] = serveMetric.MetricMap[rpsHost] / float64(vmNum)
		} else {
			serveMetric.MetricMap[rpsPerCPU] = serveMetric.MetricMap[rpsHost] / float64(cpus)
		}
		result, err := profiler.GetResult()
		require.NoError(t, err, "Stopping profiler returned error: %v", err)
		profiledCores := profiler.GetCores()
		log.Debugf("%d cores are recorded: %v", len(profiledCores), profiledCores)
		for eventName, value := range result {
			log.Debugf("%s: %.2f", eventName, value)
			serveMetric.MetricMap[eventName] = value
		}
		log.Debugf("%s: %.2f", avgExecTime, serveMetric.MetricMap[avgExecTime])
		log.Debugf("%s: %.2f", rpsHost, serveMetric.MetricMap[rpsHost])
		log.Debugf("%s: %.2f", rpsPerCPU, serveMetric.MetricMap[rpsPerCPU])
		profiler.PrintBottlenecks()

		// if tail latency violates the threshold that it should be less than 10x service time,
		// it returns the metric before tail latency violation.
		if latencies.tailLatency > threshold {
			require.NotNil(t, pmuMetric, "The tail latency of first round %.0f is larger than the threshold %.0f", latencies.tailLatency, threshold)
			return pmuMetric.MetricMap
		}

		pmuMetric = serveMetric
	}

	return pmuMetric.MetricMap
}

// loadVMs load requests to VMs every second and records completed requests and exection time
func loadVMs(t *testing.T, vmGroup *sync.WaitGroup, vmID, requestID int, imageName string,
	isSyncOffload bool, isProfile *int32, totalTime, realRequests *int64) {
	defer vmGroup.Done()

	vmIDString := strconv.Itoa(vmID)
	log.Debugf("VM %s: requestID %d", vmIDString, requestID)
	tStart := time.Now()
	resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
	execTime := time.Since(tStart).Milliseconds()
	require.Equal(t, resp.IsColdStart, false)
	if err == nil {
		if resp.Payload != "Hello, replay_response!" {
			log.Debugf("Function returned invalid: %s", resp.Payload)
		}
		if atomic.LoadInt32(isProfile) != 0 {
			atomic.AddInt64(realRequests, 1)
			atomic.AddInt64(totalTime, execTime)
			log.Debugf("VM %s: requestID %d completed in %d milliseconds", vmIDString, requestID, execTime)
		}
	} else {
		log.Debugf("VM %s: Function returned error %v", vmIDString, err)
	}
}

// configureProfiler controls the time duration of profiling for the loadVMs function and the profiler
func configureProfiler(isProfile *int32, profiler *profile.Profiler, ch chan bool) {
	time.Sleep(time.Duration(*warmUpTime) * time.Second)
	atomic.StoreInt32(isProfile, 1)
	profiler.SetWarmUpTime()
	log.Debug("Profile started")
	time.Sleep(time.Duration(*profileTime) * time.Second)
	atomic.StoreInt32(isProfile, 0)
	profiler.SetCoolDownTime()
	log.Debug("Profile finished")
	ch <- true
}

type LatencyStat struct {
	meanLatency float64
	tailLatency float64
}

// measureTailLatency measures tail latency by sampling at least every 500ms for a VM
func measureTailLatency(t *testing.T, vmNum int, images []string, latencyCh chan LatencyStat) {
	var (
		idx      int
		vmGroup  sync.WaitGroup
		times    = make([]float64, *latSamples)
		done     = make(chan bool)
		duraInMs = *profileTime * 1000 / float64(*latSamples)
	)
	if duraInMs*float64(vmNum) < 500 {
		duraInMs = 500
		log.Warnf("Too many latency samples for %d VM, measure %.0f samples instead.", vmNum, *profileTime*1000/duraInMs)
		times = times[:int(*profileTime*1000/duraInMs)]
	}
	duration := time.Duration(duraInMs)

	time.Sleep(time.Duration(*warmUpTime) * time.Second)
	ticker := time.NewTicker(duration * time.Millisecond)
	go func() {
		for {
			select {
			case <-ticker.C:
				vmGroup.Add(1)
				go func(idx int) {
					defer vmGroup.Done()
					var (
						tStart     = time.Now()
						vmID       = idx % vmNum
						vmIDString = strconv.Itoa(vmID)
					)
					resp, _, err := funcPool.Serve(context.Background(), vmIDString, images[vmID%len(images)], "replay")
					require.Equal(t, resp.IsColdStart, false)
					if err != nil {
						log.Debugf("VM %s: Function returned error %v", vmIDString, err)
					} else if resp.Payload != "Hello, replay_response!" {
						log.Debugf("Function returned invalid: %s", resp.Payload)
					} else {
						times[idx] = float64(time.Since(tStart).Milliseconds())
					}
				}(idx)
				idx = idx + 1
			case <-done:
				ticker.Stop()
				return
			}
		}
	}()
	time.Sleep(time.Duration(*profileTime) * time.Second)
	done <- true
	vmGroup.Wait()
	data := stats.LoadRawData(times)
	mean, err := stats.Mean(data)
	require.NoError(t, err, "Compute mean returned error")
	percentile, err := stats.Percentile(data, 90)
	require.NoError(t, err, "Compute 90 percentile returned error")
	latencyCh <- LatencyStat{
		meanLatency: mean,
		tailLatency: percentile,
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
			"helloworld":   "ghcr.io/ease-lab/helloworld:var_workload",
			"chameleon":    "ghcr.io/ease-lab/chameleon:var_workload",
			"pyaes":        "ghcr.io/ease-lab/pyaes:var_workload",
			"image_rotate": "ghcr.io/ease-lab/image_rotate:var_workload",
			"json_serdes":  "ghcr.io/ease-lab/json_serdes:var_workload",
			"lr_serving":   "ghcr.io/ease-lab/lr_serving:var_workload",
			"cnn_serving":  "ghcr.io/ease-lab/cnn_serving:var_workload",
			"rnn_serving":  "ghcr.io/ease-lab/rnn_serving:var_workload",
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

// getCloseLoopRPS returns the number of requests per second that stress CPU for each image.
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

// bindVMs can bind VMs to cores.
// If socket is set to a socket ID, it binds all VMs to the socket.
// If profileCPU is set to a CPU ID, it binds one VM to the CPU for profiling.
// If both are set, it binds one VM to profile CPU in the socket and other VMs to the socket
func bindVMs(socket, profileCPU int) error {
	var cpus []int
	pidBytes, err := getFirecrackerPid()
	if err != nil {
		return err
	}

	cpuInfo, err := profile.GetCPUInfo()
	if err != nil {
		return err
	}

	// If socket ID is not negative, it collects CPUID on the socket,
	// else it collects CPUID of the host.
	if socket > -1 {
		cpus, err = cpuInfo.SocketCPUs(socket)
		if err != nil {
			return err
		}
	} else {
		cpus = cpuInfo.AllCPUs()
	}

	vmPidList := strings.Split(string(pidBytes), " ")

	// bind the first firecracker process to profile CPU
	profileVM := strings.TrimSpace(vmPidList[0])
	if err := bindProcessToCPU(profileVM, profileCPU); err != nil {
		return err
	}
	// loop over rest pids of firecracker processes
	for _, vm := range vmPidList[1:] {
		vm = strings.TrimSpace(vm)
		if err := bindProcessToCPU(vm, cpus...); err != nil {
			return err
		}
	}

	return nil
}

// bindProcessToCPU changes the CPU affinity of a process to input cpus
func bindProcessToCPU(pid string, cpus ...int) error {
	var procStr, sep string
	for _, proc := range cpus {
		procStr += sep + strconv.Itoa(proc)
		sep = ","
	}
	log.Debugf("binding pid %s to processor %v", pid, cpus)
	if err := exec.Command("taskset", "--all-tasks", "-cp", procStr, pid).Run(); err != nil {
		return err
	}

	return nil
}

// cpuNum returns the total number of CPUs of the host if *bindSocket is not set,
// otherwise, it returns the number of CPUs of the socket
func cpuNum() (int, error) {
	cpuInfo, err := profile.GetCPUInfo()
	if err != nil {
		return 0, err
	}
	if *bindSocket > -1 {
		cores, err := cpuInfo.SocketCPUs(*bindSocket)
		if err != nil {
			return 0, err
		}
		return len(cores), nil
	}
	cores := cpuInfo.AllCPUs()
	return len(cores), nil
}

func checkInputValidation(t *testing.T) {
	cpuInfo, err := profile.GetCPUInfo()
	require.NoError(t, err, "Cannot get CPU info")
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
	require.Truef(t, *loadStep > 0 && *loadStep <= 100, "Load step = %d must be between 0% and 100%", *loadStep)
	sockets := cpuInfo.NumSocket()
	require.Truef(t, *bindSocket < sockets, "Socket %d must be smaller than the number of nodes %d", *bindSocket, sockets)
	var cpus []int
	if *bindSocket > -1 {
		cpus, err = cpuInfo.SocketCPUs(*bindSocket)
		require.NoError(t, err, "Cannot get CPU list of the socket")
	} else {
		cpus = cpuInfo.AllCPUs()
	}
	cpus = append(cpus, -1)
	require.Containsf(t, cpus, *profileCPUID, "profile CPU ID = %d must be in CPUs %v", *profileCPUID, cpus)
	require.Truef(t, *latSamples > 0 && *latSamples < 101, "Latency samples = %d must be between 0 and 100", *latSamples)
}
