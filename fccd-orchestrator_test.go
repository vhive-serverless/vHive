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
	"fmt"
	"os"
	"sync"
	"testing"

	ctrdlog "github.com/containerd/containerd/log"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	ctriface "github.com/ustiugov/fccd-orchestrator/ctriface"
)

func TestSendToFunction(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	orch = ctriface.NewOrchestrator("devmapper", 1)
	funcPool = NewFuncPool()

	fID := "0"
	imageName := "ustiugov/helloworld:runner_workload"

	log.Info("TestSendToFunction: Send 2 RPCs to the same function (serially)")

	for i := 0; i < 2; i++ {
		fun := funcPool.GetFunction(fID, imageName)

		resp, err := fun.Serve(context.Background(), imageName, "world")
		require.NoError(t, err, "Function returned error")
		if i == 0 {
			require.Equal(t, resp.IsColdStart, true)
		}

		log.Info(fmt.Sprintf("Sent to the function (%d), response=%s", i, resp.Payload))
	}

	fun := funcPool.GetFunction(fID, imageName)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}

func TestSendToFunctionParallel(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	orch = ctriface.NewOrchestrator("devmapper", 1)
	funcPool = NewFuncPool()

	fID := "1"
	imageName := "ustiugov/helloworld:runner_workload"

	log.Info("TestSendToFunctionParallel: Send 100 RPCs to the same function (in parallel)")

	var vmGroup sync.WaitGroup
	for i := 0; i < 100; i++ {
		vmGroup.Add(1)

		go func(i int) {
			defer vmGroup.Done()
			fun := funcPool.GetFunction(fID, imageName)

			resp, err := fun.Serve(context.Background(), imageName, "world")
			require.NoError(t, err, "Function returned error")

			log.Debug(fmt.Sprintf("Sent to the function %d, response=%s", i, resp.Payload))
		}(i)

	}
	log.Info("waiting for goroutines")
	vmGroup.Wait()

	fun := funcPool.GetFunction(fID, imageName)
	message, err := fun.RemoveInstance()
	require.NoError(t, err, "Function returned error, "+message)
}

func TestStartSendStopTwice(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	//log.SetReportCaller(true) // FIXME: make sure it's false unless debugging

	log.SetOutput(os.Stdout)

	log.SetLevel(log.InfoLevel)

	orch = ctriface.NewOrchestrator("devmapper", 1)
	funcPool = NewFuncPool()

	fID := "2"
	imageName := "ustiugov/helloworld:runner_workload"

	log.Info("TestStartSendStopTwice: Add function instance and remove it twice (serially)")

	for i := 0; i < 2; i++ {
		fun := funcPool.GetFunction(fID, imageName)

		resp, err := fun.Serve(context.Background(), imageName, "world")
		require.NoError(t, err, "Function returned error")

		log.Info(fmt.Sprintf("Sent to the function %s (instance %d), response=%s", fID, i, resp.Payload))

		log.Info("Removing an instance")
		fun = funcPool.GetFunction(fID, imageName)
		message, err := fun.RemoveInstance()
		require.NoError(t, err, "Function returned error, "+message)
	}
}
