// MIT License
//
<<<<<<< HEAD
// Copyright (c) 2020 Yuchen Niu and EASE lab
=======
// Copyright (c) 2020 Yuchen Niu
>>>>>>> skeleton of perf_bench_test
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
	"strconv"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestBenchRequestPerSecond(t *testing.T) {
	var (
		servedTh       uint64
		pinnedFuncNum  int
		isSyncOffload  bool = true
		images              = getAllImages()
		funcs               = mapToArray(images)
		funcIdx             = 0
		vmID                = 0
		requestsPerSec      = 1
		concurrency         = 1
		totalSeconds        = 5 // in second
		// duration           = time.Minute
	)

	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	ticker := time.NewTicker(time.Second)
	seconds := 0
	sem := make(chan bool, concurrency)

	for seconds < totalSeconds {
		select {
		case <-ticker.C:
			seconds++
			for i := 0; i < requestsPerSec; i++ {
				sem <- true

				funcName := funcs[funcIdx]
				imageName := images[funcName]

				// pull image
				resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "record")

				require.NoError(t, err, "Function returned error")
				require.Equal(t, resp.Payload, "Hello, record_response!")

				vmIDString := strconv.Itoa(vmID)

				createSnapshots(t, concurrency, vmID, imageName, isSyncOffload)
				log.Info("Snapshot created")

				createRecords(t, concurrency, vmID, imageName, isSyncOffload)
				log.Info("Record done")

				if !*isWithCache {
					dropPageCache()
				}

				tStart := time.Now()

				go func(start time.Time, vmIDString string, semaphore chan bool) {
					defer func() { <-semaphore }()

					// serve
					resp, _, err := funcPool.Serve(context.Background(), vmIDString, imageName, "replay")
					require.NoError(t, err, "Function returned error")
					require.Equal(t, resp.Payload, "Hello, replay_response!")

					message, err := funcPool.RemoveInstance(vmIDString, imageName, isSyncOffload)
					require.NoError(t, err, "Function returned error, "+message)

					log.Printf("Instance finished in %f seconds", time.Since(start).Seconds())
					// wait for time interval
					// timeLeft := duration.Nanoseconds() - time.Since(tStart).Nanoseconds()
					// require.GreaterOrEqual(t, timeLeft, 0)

					// time.Sleep(time.Duration(timeLeft))
				}(tStart, vmIDString, sem)

				funcIdx++
				funcIdx %= len(funcs)
				vmID++
			}

		}
	}

	ticker.Stop()
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}
}

///////////////////////////////////////////////////////////////////////////////
////////////////////////// Auxialiary functions below /////////////////////////
///////////////////////////////////////////////////////////////////////////////
func mapToArray(m map[string]string) []string {
	list := []string{}
	for k := range m {
		list = append(list, k)
	}

	return list
}
