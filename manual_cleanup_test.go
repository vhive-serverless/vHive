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
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParallelLoadServe(t *testing.T) {
	// Needs to be cleaned up manually.
	imageName := "ustiugov/helloworld:runner_workload"
	funcPool = NewFuncPool(false, 0, 0, true)

	// Pull image to work around parallel pulling limitation
	resp, err := funcPool.Serve(context.Background(), "plr_fnc", imageName, "world")
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
