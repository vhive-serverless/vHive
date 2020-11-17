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
	vmNum       = flag.Int("vm", 2, "The number of VMs")
)

func TestBenchRequestPerSecond(t *testing.T) {

	var (
		servedTh        uint64
		pinnedFuncNum   int
		isSyncOffload   bool = true
		images               = getImages()
		funcs                = []string{}
		workQueues           = make([]chan bool, *vmNum) // Queue the request to wait for predecessor finish
		vmID                 = 0
		totalSeconds         = 5
		targetReqPerSec      = 2
		duration             = 30 * time.Second
	)
	log.SetLevel(log.InfoLevel)

	checkInputValidation(t)

	createResultsDir()

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	// pull images
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

	// warm VMs
	for i := 0; i < *vmNum; i++ {
		vmID += i
		vmIDString := strconv.Itoa(vmID)
		_, err := funcPool.AddInstance(vmIDString, images[funcs[vmID%len(funcs)]])
		require.NoError(t, err, "Function returned error")
		workQueues[vmID] = make(chan bool, 1)
	}

	if !*isWithCache && *isColdStart {
		log.Debug("Profile cold start")
		dropPageCache()
	}

	log.SetLevel(log.DebugLevel)

	timeInterval := time.Second.Nanoseconds() / int64(targetReqPerSec)
	ticker := time.NewTicker(time.Duration(timeInterval))
	totalRequests := totalSeconds * targetReqPerSec
	requests := 0
	vmID = 0

	for requests < totalRequests {
		select {
		case <-ticker.C:
			requests++

			funcName := funcs[vmID%len(funcs)]
			imageName := images[funcName]

			workQueues[vmID] <- true

			tStart := time.Now()

			go func(start time.Time, vmID int, imageName string) {
				defer func() { <-workQueues[vmID] }()

				vmIDString := strconv.Itoa(vmID)

				// serve
				resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, replay_response!")

				if *isColdStart {
					// if profile cold start, remove instance after serve returns
					require.Equal(t, resp.IsColdStart, true)
					message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
					require.NoError(t, err, "Function returned error, "+message)
				} else {
					require.Equal(t, resp.IsColdStart, false)
				}

				log.Printf("Instance returned in %f seconds", time.Since(start).Seconds())

				// wait for time interval
				timeLeft := duration.Nanoseconds() - time.Since(tStart).Nanoseconds()
				log.Printf("timeLeft: %f seconds", float64(timeLeft)*1e-9)

				time.Sleep(time.Duration(timeLeft))
			}(tStart, vmID, imageName)

			vmID++
			vmID %= *vmNum
		}
	)

	for _, funcName := range funcs {
		values = append(values, reqsPerSec[funcName])
		sum += reqsPerSec[funcName]
	}
	ticker.Stop()

	// wait for all functions to finish
	for i := 0; i < *vmNum; i++ {
		workQueues[i] <- true
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

func getImages() map[string]string {
	return map[string]string{
		"helloworld": "ustiugov/helloworld:var_workload",
		// "chameleon":  "ustiugov/chameleon:var_workload",
		//"pyaes":        "ustiugov/pyaes:var_workload",
		//"image_rotate": "ustiugov/image_rotate:var_workload",
		//"json_serdes":  "ustiugov/json_serdes:var_workload",
		//"lr_serving":   "ustiugov/lr_serving:var_workload",
		//"cnn_serving":  "ustiugov/cnn_serving:var_workload",
		//"rnn_serving":  "ustiugov/rnn_serving:var_workload",
		//"lr_training":  "ustiugov/lr_training:var_workload",
	}
}
