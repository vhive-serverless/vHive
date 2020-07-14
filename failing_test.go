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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
