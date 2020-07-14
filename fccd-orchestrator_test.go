// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov
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
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ctriface "github.com/ustiugov/fccd-orchestrator/ctriface"
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	orch = ctriface.NewOrchestrator("devmapper", 10, true)

	ret := m.Run()

	err := orch.StopActiveVMs()
	if err != nil {
		log.Printf("Failed to stop VMs, err: %v\n", err)
	}

	orch.Cleanup()

	os.Exit(ret)
}

func TestPauseResumeSerial(t *testing.T) {
	fID := "1"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 1st run")
	require.Equal(t, resp.IsColdStart, true)
	require.Equal(t, resp.Payload, "Hello, world!")

	_, err = orch.PauseVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when pausing VM")

	// NOTE: Current implementation just return error but does not time out
	//timeout_ctx, _ := context.WithTimeout(context.Background(), 20*time.Second)
	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.Error(t, err, "Function did not time out on 2nd run")
	require.Equal(t, resp.Payload, "")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 3rd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestServePauseSnapResumeServe(t *testing.T) {
	fID := "2"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 1st run")
	require.Equal(t, resp.IsColdStart, true)
	require.Equal(t, resp.Payload, "Hello, world!")

	_, err = orch.PauseVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when pausing VM")

	_, err = orch.CreateSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Error when creating snapshot of VM")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSendToFunctionSerial(t *testing.T) {
	fID := "7"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	for i := 0; i < 2; i++ {
		resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
		require.NoError(t, err, "Function returned error")
		if i == 0 {
			require.Equal(t, resp.IsColdStart, true)
		}

		require.Equal(t, resp.Payload, "Hello, world!")
	}

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSendToFunctionParallel(t *testing.T) {
	fID := "8"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()
			resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)

	}
	vmGroup.Wait()

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestStartSendStopTwice(t *testing.T) {
	fID := "9"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 1, 2, true)

	for i := 0; i < 2; i++ {
		for k := 0; k < 2; k++ {
			resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}

		message, err := funcPool.RemoveInstance(fID, imageName)
		require.NoError(t, err, "Function returned error, "+message)
	}

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 4, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 2, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotNumericFunction(t *testing.T) {
	fID := "not_cld"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(true, 1, 2, true)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 1, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 1, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotColdFunction(t *testing.T) {
	fID := "10"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(true, 1, 11, true)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.stats.statMap[fID].served
	require.Equal(t, 1, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 1, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestSaveMemorySerial(t *testing.T) {
	fID := "11"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(true, 40, 2, true)

	for i := 0; i < 100; i++ {
		resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, world!")
	}

	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSaveMemoryParallel(t *testing.T) {
	fID := "12"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(true, 40, 2, true)

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()

			resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)

	}
	vmGroup.Wait()

	startsGot := funcPool.stats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestDirectStartStopVM(t *testing.T) {
	fID := "13"
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	message, err := funcPool.AddInstance(fID, imageName)
	require.NoError(t, err, "This error should never happen (addInstance())"+message)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err = funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestAllFunctions(t *testing.T) {
	images := []string{
		"ustiugov/helloworld:var_workload",
		"ustiugov/chameleon:var_workload",
		"ustiugov/pyaes:var_workload",
		"ustiugov/image_rotate:var_workload",
		"ustiugov/json_serdes:var_workload",
		//"ustiugov/lr_serving:var_workload", Issue#15
		//"ustiugov/cnn_serving:var_workload",
		"ustiugov/rnn_serving:var_workload",
		//"ustiugov/lr_training:var_workload",
	}
	funcPool = NewFuncPool(false, 0, 0, true)

	for i := 0; i < 2; i++ {
		var vmGroup sync.WaitGroup
		for fID, imageName := range images {
			reqs := []string{"world", "record", "replay"}
			resps := []string{"world", "record_response", "replay_response"}
			for k := 0; k < 3; k++ {
				vmGroup.Add(1)
				go func(fID int, imageName, request, response string) {
					defer vmGroup.Done()

					resp, err := funcPool.Serve(context.Background(), strconv.Itoa(fID), imageName, request)
					require.NoError(t, err, "Function returned error")

					require.Equal(t, resp.Payload, "Hello, "+response+"!")
				}(fID, imageName, reqs[k], resps[k])
			}
			vmGroup.Wait()
		}
	}

	var vmGroup sync.WaitGroup
	for fID, imageName := range images {
		vmGroup.Add(1)
		go func(fID int, imageName string) {
			defer vmGroup.Done()

			message, err := funcPool.RemoveInstance(strconv.Itoa(fID), imageName)
			require.NoError(t, err, "Function returned error, "+message)
		}(fID, imageName)
	}
	vmGroup.Wait()
}
