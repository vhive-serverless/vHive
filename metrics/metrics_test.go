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
	s1 := NewStartVMStat()
	s1.GetImage = 10
	s1.TaskStart = 15
	require.Equal(t, int64(25), s1.Total(), "Total is incorrect")

	s2 := NewStartVMStat()
	s2.GetImage = 10
	s2.TaskStart = 15
	s2.NewContainer = 150

	agg := NewStartVMStat()
	agg.Aggregate(s1, s2)
	require.Equal(t, int64(20), agg.GetImage, "GetImage value is incorrect")
	require.Equal(t, int64(30), agg.TaskStart, "TaskStart value is incorrect")
	require.Equal(t, int64(150), agg.NewContainer, "NewContainer value is incorrect")
	require.Equal(t, int64(0), agg.TaskWait, "TaskWait value is incorrect")
	require.Equal(t, int64(0), agg.NewTask, "NewTask value is incorrect")
	require.Equal(t, int64(0), agg.FcCreateVM, "FcCreateVM value is incorrect")
	require.Equal(t, int64(200), agg.Total(), "Aggregate Total is incorrect")

	PrintStartVMStats(s1, s2)
}

func TestLoadSnapshotStats(t *testing.T) {
	s1 := NewLoadSnapshotStat()
	s1.Full = 10
	require.Equal(t, int64(10), s1.Total(), "Total is incorrect")

	s2 := NewLoadSnapshotStat()
	s2.Full = 10

	agg := NewLoadSnapshotStat()
	agg.Aggregate(s1, s2)
	require.Equal(t, int64(20), agg.Full, "Full Total is incorrect")
	require.Equal(t, int64(20), agg.Total(), "Aggregate Total is incorrect")

	PrintLoadSnapshotStats(s1, s2)
}

func TestServeStats(t *testing.T) {
	s1 := NewServeStat()
	s1.GetResponse = 25
	s1.RetireOld = 10
	require.Equal(t, int64(35), s1.Total(), "Total is incorrect")

	PrintServeStats(s1)
}
