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

package cri

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartStop(t *testing.T) {
	containerID := "1"
	_, _, err := coord.startVM(context.Background(), containerID)
	require.NoError(t, err, "could not start VM")

	err = coord.insertMapping(containerID, "1")
	require.NoError(t, err, "could not insert mapping")

	present := coord.isActive(containerID)
	require.True(t, present, "container is not active")

	err = coord.stopVM(context.Background(), containerID)
	require.NoError(t, err, "could not stop VM")

	present = coord.isActive(containerID)
	require.False(t, present, "container is active")
}

func TestParallelStartStop(t *testing.T) {
	var wg sync.WaitGroup

	containerNum := 1000

	for i := 0; i < containerNum; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			containerID := strconv.Itoa(i)
			_, _, err := coord.startVM(context.Background(), containerID)
			require.NoError(t, err, "could not start VM")

			err = coord.insertMapping(containerID, containerID)
			require.NoError(t, err, "could not insert mapping")

			present := coord.isActive(containerID)
			require.True(t, present, "container is not active")

			err = coord.stopVM(context.Background(), containerID)
			require.NoError(t, err, "could not stop VM")

			present = coord.isActive(containerID)
			require.False(t, present, "container is active")
		}(i)
	}

	wg.Wait()
}
