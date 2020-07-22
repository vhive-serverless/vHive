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
	"testing"

	"github.com/stretchr/testify/require"
)

func TestServePauseSnapLoadServe(t *testing.T) {
	// Needs to be cleaned up manually.
	fID := "3"
	imageName := "ustiugov/helloworld:runner_workload"
	var servedTh uint64 = 0
	pinnedFuncNum := 0
	funcPool = NewFuncPool(!isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	resp, err := funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 1st run")
	require.Equal(t, resp.IsColdStart, true)
	require.Equal(t, resp.Payload, "Hello, world!")

	resp, err = funcPool.Serve(context.Background(), fID, imageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	// Breaks here because removing instance does not work
	message, err := funcPool.RemoveInstance(fID, imageName, true)
	require.NoError(t, err, "Function returned error, "+message)
}
