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
	"strconv"
	"sync"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

var (
	isColdStart     = flag.Bool("coldStart", false, "Profile cold starts")
	vmNum           = flag.Int("vm", 2, "The number of VMs")
	targetReqPerSec = flag.Int("requestPerSec", 2, "The target number of requests per second")
	executionTime   = flag.Int("executionTime", 5, "The execution time of the benchmark in second")
)

func TestBenchRequestPerSecond(t *testing.T) {
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
		images             = getImages()
		funcs              = []string{}
		timeInterval       = time.Duration(time.Second.Nanoseconds() / int64(*targetReqPerSec))
		totalRequests      = *executionTime * *targetReqPerSec
		vmID               = 0
	)

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
		vmIDString := strconv.Itoa(i)
		_, err := funcPool.AddInstance(vmIDString, images[funcs[i%len(funcs)]])
		require.NoError(t, err, "Function returned error")
	}

	if !*isWithCache && *isColdStart {
		log.Debug("Profile cold start")
		dropPageCache()
	}

	log.SetLevel(log.DebugLevel)

	ticker := time.NewTicker(timeInterval)
	requests := 0
	var vmGroup sync.WaitGroup

	for requests < totalRequests {
		select {
		case <-ticker.C:
			requests++

			funcName := funcs[vmID%len(funcs)]
			imageName := images[funcName]

			vmGroup.Add(1)

			tStart := time.Now()

			go func(start time.Time, vmID int, imageName string) {
				defer vmGroup.Done()

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

				log.Printf("vmID %d returned in %f seconds", vmID, time.Since(start).Seconds())

				timeLeft := timeInterval.Seconds() - time.Since(tStart).Seconds()
				log.Printf("vmID %d time left: %f seconds", vmID, timeLeft)
			}(tStart, vmID, imageName)

			vmID = (vmID + 1) % *vmNum
		}
	}
	ticker.Stop()

	// wait for all functions finish before exit
	vmGroup.Wait()
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
