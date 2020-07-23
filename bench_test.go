// MIT License
//
// Copyright (c) 2020 Plamen Petrov
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
	"os/exec"
	"testing"

	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"github.com/ustiugov/fccd-orchestrator/metrics"
)

func TestBenchmarkServeWithCache(t *testing.T) {
	fID := "1"
	imageName := "ustiugov/helloworld:runner_workload"
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	// Pull image to work around parallel pulling limitation
	resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance("plr_fnc", imageName, isSyncOffload)
	require.NoError(t, err, "Function returned error, "+message)
	// -----------------------------------------------------

	resp, _, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err = funcPool.RemoveInstance(fID, imageName, isSyncOffload)
	require.NoError(t, err, "Function returned error, "+message)

	benchCount := 10
	serveStats := make([]*metrics.ServeStat, benchCount)

	for i := 0; i < 10; i++ {
		resp, stat, err := funcPool.Serve(context.Background(), fID, imageName, "world")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, world!")

		serveStats[i] = stat

		message, err := funcPool.RemoveInstance(fID, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
	}

	metrics.PrintServeStats(serveStats...)
}

func TestBenchmarkServeNoCache(t *testing.T) {
	fID := "1"
	imageName := "ustiugov/helloworld:runner_workload"
	var (
		servedTh      uint64
		pinnedFuncNum int
		isSyncOffload bool = true
	)
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	// Pull image to work around parallel pulling limitation
	resp, _, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err := funcPool.RemoveInstance("plr_fnc", imageName, isSyncOffload)
	require.NoError(t, err, "Function returned error, "+message)
	// -----------------------------------------------------

	resp, _, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")

	message, err = funcPool.RemoveInstance(fID, imageName, isSyncOffload)
	require.NoError(t, err, "Function returned error, "+message)

	benchCount := 10
	serveStats := make([]*metrics.ServeStat, benchCount)

	for i := 0; i < 10; i++ {
		dropPageCache()

		resp, stat, err := funcPool.Serve(context.Background(), fID, imageName, "world")
		require.NoError(t, err, "Function returned error")
		require.Equal(t, resp.Payload, "Hello, world!")

		serveStats[i] = stat

		message, err := funcPool.RemoveInstance(fID, imageName, isSyncOffload)
		require.NoError(t, err, "Function returned error, "+message)
	}

	metrics.PrintServeStats(serveStats...)
}

func dropPageCache() {
	cmd := exec.Command("sudo", "/bin/sh", "-c", "sync; echo 1 > /proc/sys/vm/drop_caches")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Failed to drop caches: %v", err)
	}
}
