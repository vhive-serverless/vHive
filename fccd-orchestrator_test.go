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
	"time"

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

func TestServePauseSnapLoadServe(t *testing.T) {
	// Needs to be cleaned up manually.
	fID := "3"
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

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	// Breaks here because removing instance does not work
	message, err := funcPool.RemoveInstance(fID, imageName)
	require.NoError(t, err, "Function returned error, "+message)
}

func TestParallelLoadServe(t *testing.T) {
	// Needs to be cleaned up manually.
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	// Pull image to work around parallel pulling limitation
	resp, err := funcPool.Serve(context.Background(), "puller_func", imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")
	// -----------------------------------------------------

	vmNum := 5

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			fID := strconv.Itoa(i)
			vmID := fmt.Sprintf("%s_0", fID)
			snapshotFilePath := fmt.Sprintf("/dev/snapshot_file_%s", fID)
			memFilePath := fmt.Sprintf("/dev/mem_file_%s", fID)

			resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
			require.NoError(t, err, "Function returned error on 1st run")
			require.Equal(t, resp.Payload, "Hello, world!")

			_, err = orch.PauseVM(context.Background(), vmID)
			require.NoError(t, err, "Error when pausing VM")

			_, err = orch.CreateSnapshot(context.Background(), vmID, snapshotFilePath, memFilePath)
			require.NoError(t, err, "Error when creating snapshot of VM")

			_, err = orch.Offload(context.Background(), vmID)
			require.NoError(t, err, "Failed to offload VM, "+vmID)

			time.Sleep(300 * time.Millisecond)

			_, err = orch.LoadSnapshot(context.Background(), vmID, snapshotFilePath, memFilePath)
			require.NoError(t, err, "Failed to load snapshot of VM, "+"")

			_, err = orch.ResumeVM(context.Background(), vmID)
			require.NoError(t, err, "Error when resuming VM")

			resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
			require.NoError(t, err, "Function returned error on 2nd run")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)
	}
	vmGroup.Wait()
}

func TestLoadServeMultiple1(t *testing.T) {
	// Does not work. Hangs on 3rd serve
	// Chain:
	// createVM > serve(1) > pause > createSnap > offload > sleep10
	// > loadSnap > resume > serve(2) > offload > sleep10 > loadSnap
	// > resume > serve(3)
	fID := "4"
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

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 3rd run")
	require.Equal(t, resp.Payload, "Hello, world!")
}

func TestLoadServeMultiple2(t *testing.T) {
	// Needs to be cleaned up manually.
	// Works
	// Chain:
	// createVM > serve(1) > pause > createSnap > offload > sleep10
	// > loadSnap > resume > serve(2) > serve(3) > offload > sleep10 > loadSnap
	// > resume
	fID := "5"
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

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 3rd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

}

func TestLoadServeMultiple3(t *testing.T) {
	// Needs to be cleaned up manually.
	// Works
	// Chain:
	// createVM > serve(1) > pause > createSnap > offload > sleep10
	// > loadSnap > resume > offload > sleep10 > loadSnap
	// > resume > serve(2) > serve(3)
	fID := "6"
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

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	_, err = orch.Offload(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Failed to offload VM, "+"")

	time.Sleep(300 * time.Millisecond)

	_, err = orch.LoadSnapshot(context.Background(), fmt.Sprintf(fID+"_0"), "/tmp/snap_test", "/tmp/mem_test")
	require.NoError(t, err, "Failed to load snapshot of VM, "+"")

	_, err = orch.ResumeVM(context.Background(), fmt.Sprintf(fID+"_0"))
	require.NoError(t, err, "Error when resuming VM")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 3rd run")
	require.Equal(t, resp.Payload, "Hello, world!")
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
	fID := "not_cold_func"
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
