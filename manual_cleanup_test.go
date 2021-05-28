// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParallelServe(t *testing.T) {
	var (
		servedTh      uint64 = 1
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	// Pull image to work around parallel pulling limitation
	resp, _, err := funcPool.Serve(context.Background(), "plr-fnc", testImageName, "world")
	require.NoError(t, err, "Function returned error")
	require.Equal(t, resp.Payload, "Hello, world!")
	// -----------------------------------------------------

	vmNum := 5

	var vmGroup sync.WaitGroup
	for i := 0; i < vmNum; i++ {
		vmGroup.Add(1)
		go func(i int) {
			defer vmGroup.Done()
			fID := strconv.Itoa(100 + i)

			resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
			require.NoError(t, err, "Function returned error on 1st run")
			require.Equal(t, resp.Payload, "Hello, world!")

			resp, _, err = funcPool.Serve(context.Background(), fID, testImageName, "world")
			require.NoError(t, err, "Function returned error on 2nd run")
			require.Equal(t, resp.Payload, "Hello, world!")
		}(i)
	}
	vmGroup.Wait()
}

func TestServeThree(t *testing.T) {
	fID := "200"
	var (
		servedTh      uint64 = 1
		pinnedFuncNum int
	)
	funcPool = NewFuncPool(isSaveMemoryConst, servedTh, pinnedFuncNum, isTestModeConst)

	resp, _, err := funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error on 1st run")
	require.Equal(t, resp.IsColdStart, true)
	require.Equal(t, resp.Payload, "Hello, world!")

	resp, _, err = funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error on 2nd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	resp, _, err = funcPool.Serve(context.Background(), fID, testImageName, "world")
	require.NoError(t, err, "Function returned error on 3rd run")
	require.Equal(t, resp.Payload, "Hello, world!")

	// FIXME: Removes this sleep when Issue#30
	time.Sleep(10 * time.Second)
}
