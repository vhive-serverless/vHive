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
	"flag"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"github.com/ease-lab/vhive/profile"
	"github.com/montanaflynn/stats"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	isColdStart = flag.Bool("coldStart", false, "Profile cold starts")
)

func TestBenchRequestPerSecond(t *testing.T) {

	var (
		servedTh       uint64
		pinnedFuncNum  int
		isSyncOffload  bool = true
		images              = getAllImages()
		funcs               = []string{}
		funcIdx             = 0
		vmID                = 0
		requestsPerSec      = 1
		concurrency         = 1
		totalSeconds        = 5
		duration            = 30 * time.Second
	)
	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	// pull image
	for funcName, imageName := range images {
		resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")

		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, record_response!")

		// For future use
		// createSnapshots(t, concurrency, vmID, imageName, isSyncOffload)
		// log.Info("Snapshot created")

		// createRecords(t, concurrency, vmID, imageName, isSyncOffload)
		// log.Info("Record done")

		funcs = append(funcs, funcName)
	}

	if !*isWithCache && *isColdStart {
		log.Info("Cold invoke")
		dropPageCache()
	}

	ticker := time.NewTicker(time.Second)
	seconds := 0
	sem := make(chan bool, concurrency)

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	plrfncPid := pullImages(t, images)

	bootVMs(t, images, 0, *vmNum, plrfncPid)

	serveMetrics := loadAndProfile(t, images, *vmNum, *targetRPS, isSyncOffload)

	tearDownVMs(t, images, *vmNum, isSyncOffload)

	dumpMetrics(t, []map[string]float64{serveMetrics}, "benchRPS.csv")
}

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
func bootVMs(t *testing.T, images []string, startVMID, endVMID int, omittedPid string) {
	for i := startVMID; i < endVMID; i++ {
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[i%len(images)])
		require.NoError(t, err, "Function returned error")
	}

	if *isBindSocket {
		log.Debugf("Binding socket 1")
		err := bindSocket(omittedPid)
		require.NoError(t, err, "Bind Socket returned error")
	}
}

// Inject many requests per second to VMs and profile
func loadAndProfile(t *testing.T, images []string, vmNum, targetRPS int, isSyncOffload bool) map[string]float64 {
	var (
		pmuMetric      *metrics.Metric
		vmGroup        sync.WaitGroup
		isProfile      = false
		cores          = runtime.NumCPU()
		threshold      = 10 * getUnloadServiceTime() // for the constraint of tail latency
		injectDuration = *warmUpTime + *profileTime + *coolDownTime
	)

	// the constants for metric names
	const (
		avgExecTime = "average-execution-time"
		rpsPerCore  = "RPS-per-Core"
	)

	for step := 0.05; step < 1.05; step += 0.05 {
		var (
			vmID, requestID             int
			invokExecTime, realRequests int64
			serveMetric                 = metrics.NewMetric()
			rps                         = step * float64(targetRPS)
			timeInterval                = time.Duration(time.Second.Nanoseconds() / int64(rps))
			ticker                      = time.NewTicker(timeInterval)
			totalRequests               = injectDuration * rps
			remainingRequests           = totalRequests
			profiler                    = profile.NewProfiler(injectDuration, *profilerInterval, vmNum, *profilerLevel, *profilerNodes, "profile", *isBindSocket)
		)

		log.Infof("Current RPS: %f", rps)

		tStart := time.Now()
		latencyCh := make(chan LatencyStat)
		go latencyMeasurement(t, 0, images[1%len(images)], &isProfile, latencyCh)

		err := profiler.Run()
		require.NoError(t, err, "Run profiler returned error")
		go profileControl(vmNum, &isProfile, profiler)

		for remainingRequests > 0 {
			if tickerT := <-ticker.C; !tickerT.IsZero() {
				vmGroup.Add(1)
				remainingRequests--

				imageName := images[vmID%len(images)]

				go loadVM(t, &vmGroup, vmID, requestID, imageName, isSyncOffload, &isProfile, &invokExecTime, &realRequests)
				requestID++

				vmID = (vmID + 1) % vmNum
			}
		}
		ticker.Stop()
		vmGroup.Wait()
		log.Debugf("All VM returned in %d Milliseconds", time.Since(tStart).Milliseconds())
		latencies := <-latencyCh
		log.Debugf("Mean Latency: %f, Tail Latency: %f", latencies.meanLatency, latencies.tailLatency)

		// Collect results
		serveMetric.MetricMap[avgExecTime] = float64(invokExecTime) / float64(realRequests)
		serveMetric.MetricMap[rpsPerCore] = float64(realRequests) / (profiler.GetCoolDownTime() - profiler.GetWarmUpTime())
		if cores > vmNum {
			serveMetric.MetricMap[rpsPerCore] /= float64(vmNum)
		} else {
			serveMetric.MetricMap[rpsPerCore] /= float64(cores)
	for seconds < totalSeconds {
		select {
		case <-ticker.C:
			seconds++
			for i := 0; i < requestsPerSec; i++ {
				sem <- true

				funcName := funcs[funcIdx]
				imageName := images[funcName]

				vmIDString := strconv.Itoa(vmID)

				tStart := time.Now()

				go func(start time.Time, vmIDString, imageName string, semaphore chan bool) {
					defer func() { <-semaphore }()

					// serve
					resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
					require.NoError(t, err, "Function returned error")
					require.Equal(t, resp.Payload, "Hello, replay_response!")

					if *isColdStart {
						message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
						require.NoError(t, err, "Function returned error, "+message)
					}

					log.Printf("Instance returned in %f seconds", time.Since(start).Seconds())

					// wait for time interval
					timeLeft := duration.Nanoseconds() - time.Since(tStart).Nanoseconds()
					log.Printf("timeLeft: %f seconds", float64(timeLeft)*1e-9)

					time.Sleep(time.Duration(timeLeft))
				}(tStart, vmIDString, imageName, sem)

				funcIdx++
				funcIdx %= len(funcs)
				vmID++
			}
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
			"chameleon":    15,
			"pyaes":        1,
			"image_rotate": 26,
			"json_serdes":  16,
			"lr_serving":   3,
			"cnn_serving":  60,
			"rnn_serving":  26,
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
