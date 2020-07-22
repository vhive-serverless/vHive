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

package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStartVMStats(t *testing.T) {
	s := NewStartVMStat()
	s.GetImage = 10
	s.TaskStart = 15
	require.Equal(t, int64(25), s.Total(), "Total is incorrect")

	s1 := NewStartVMStat()
	s1.GetImage = 10
	s1.TaskStart = 15
	s1.NewContainer = 150

	s2 := s.Aggregate(s1)
	require.Equal(t, int64(20), s2.GetImage, "GetImage value is incorrect")
	require.Equal(t, int64(30), s2.TaskStart, "TaskStart value is incorrect")
	require.Equal(t, int64(150), s2.NewContainer, "NewContainer value is incorrect")
	require.Equal(t, int64(0), s2.TaskWait, "TaskWait value is incorrect")
	require.Equal(t, int64(0), s2.NewTask, "NewTask value is incorrect")
	require.Equal(t, int64(0), s2.FcCreateVM, "FcCreateVM value is incorrect")
	require.Equal(t, int64(200), s2.Total(), "Aggregate Total is incorrect")
}

func TestLoadSnapshotStats(t *testing.T) {
	s := NewLoadSnapshotStat()
	s.Full = 10
	require.Equal(t, int64(10), s.Total(), "Total is incorrect")

	s1 := NewLoadSnapshotStat()
	s1.Full = 10

	s2 := s.Aggregate(s1)
	require.Equal(t, int64(20), s2.Full, "Full Total is incorrect")
	require.Equal(t, int64(20), s2.Total(), "Aggregate Total is incorrect")
}
