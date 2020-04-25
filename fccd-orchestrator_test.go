// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov
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
	"os"
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

	orch = ctriface.NewOrchestrator("devmapper", 10)
	funcPool = NewFuncPool()

	ret := m.Run()

	err := orch.StopActiveVMs()
	if err != nil {
		log.Printf("Failed to stop VMs, err: %v\n", err)
	}

	os.Exit(ret)
}

func TestSendToFunctionSerial(t *testing.T) {
	fID := "0"
	imageName := "ustiugov/helloworld:runner_workload"

	for i := 0; i < 2; i++ {
		fun := funcPool.GetFunction(fID, imageName, false)

		resp, err := fun.Serve(context.Background(), imageName, "world")
		require.NoError(t, err, "Function returned error")
		if i == 0 {
			require.Equal(t, resp.IsColdStart, true)
		}

		require.Equal(t, resp.Payload, "Hello, world!")
	}

	fun := funcPool.GetFunction(fID, imageName, false)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSendToFunctionParallel(t *testing.T) {
	fID := "1"
	imageName := "ustiugov/helloworld:runner_workload"

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()
			fun := funcPool.GetFunction(fID, imageName, false)

			resp, err := fun.Serve(context.Background(), imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)

	}
	vmGroup.Wait()

	fun := funcPool.GetFunction(fID, imageName, false)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}

func TestStartSendStopTwice(t *testing.T) {
	fID := "200"
	imageName := "ustiugov/helloworld:runner_workload"

	for i := 0; i < 2; i++ {
		fun := funcPool.GetFunction(fID, imageName, false)

		for k := 0; k < 2; k++ {
			resp, err := fun.Serve(context.Background(), imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")
		}

		fun = funcPool.GetFunction(fID, imageName, false)
		message, err := fun.RemoveInstance()
		require.NoError(t, err, "Function returned error, "+message)
	}

	servedGot := funcPool.coldStats.statMap[fID].served
	require.Equal(t, 4, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.coldStats.statMap[fID].started
	require.Equal(t, 2, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotNumericFunction(t *testing.T) {
	fID := "not_cold_func"
	imageName := "ustiugov/helloworld:runner_workload"

	fun := funcPool.GetFunction(fID, imageName, true)

	resp, err := fun.Serve(context.Background(), imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	fun = funcPool.GetFunction(fID, imageName, true)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.coldStats.statMap[fID].served
	require.Equal(t, 0, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.coldStats.statMap[fID].started
	require.Equal(t, 0, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestStatsNotColdFunction(t *testing.T) {
	fID := "3"
	imageName := "ustiugov/helloworld:runner_workload"

	fun := funcPool.GetFunction(fID, imageName, true)

	resp, err := fun.Serve(context.Background(), imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	fun = funcPool.GetFunction(fID, imageName, true)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)

	servedGot := funcPool.coldStats.statMap[fID].served
	require.Equal(t, 0, int(servedGot), "Cold start (served) stats are wrong")
	startsGot := funcPool.coldStats.statMap[fID].started
	require.Equal(t, 0, int(startsGot), "Cold start (starts) stats are wrong")
}

func TestSaveMemorySerial(t *testing.T) {
	fID := "4"
	imageName := "ustiugov/helloworld:runner_workload"

	isSaveMemory := true
	servedThreshold := uint64(40)
	pinnedFunctionsNum := 0

	for i := 0; i < 100; i++ {
		toPin := isToPin(fID, pinnedFunctionsNum)
		require.Equal(t, false, toPin)

		fun := funcPool.GetFunction(fID, imageName, toPin)

		resp, err := fun.Serve(context.Background(), imageName, "world")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, world!")

		if isSaveMemory && fun.GetStatServed() >= servedThreshold {
			go saveMemory(fun, toPin, servedThreshold, log.WithFields(log.Fields{"fID": fID}))
		}
	}

	startsGot := funcPool.coldStats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	fun := funcPool.GetFunction(fID, imageName, false)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}

/*
func TestSaveMemoryParallel(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	fID := "5"
	imageName := "ustiugov/helloworld:runner_workload"

	isSaveMemory := true
	servedThreshold := uint64(4)
	pinnedFunctionsNum := 0

	var vmGroup sync.WaitGroup
	for i := 0; i < 10; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()

			toPin := isToPin(fID, pinnedFunctionsNum)
			require.Equal(t, false, toPin)

			fun := funcPool.GetFunction(fID, imageName, toPin)

			resp, err := fun.Serve(context.Background(), imageName, "world")
			require.NoError(t, err, "Function returned error")
			require.Equal(t, resp.Payload, "Hello, world!")

			if isSaveMemory && fun.GetStatServed() >= servedThreshold {
				go saveMemory(fun, toPin, servedThreshold, log.WithFields(log.Fields{"fID": fID}))
			}
		}(i)

	}
	vmGroup.Wait()

	startsGot := funcPool.coldStats.statMap[fID].started
	require.Equal(t, 3, int(startsGot), "Cold start (starts) stats are wrong")

	fun := funcPool.GetFunction(fID, imageName, false)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}
*/
