// MIT License
//
// Copyright (c) 2021 Yuchen Niu and EASE lab
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

package profile

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestReadPerfData(t *testing.T) {
	var (
		fileName = "test"
		result   = []map[string]float64{
			{
				"Frontend_Bound": 2,
				"Backend_Bound":  3},
			{
				"Frontend_Bound": 1,
				"Backend_Bound":  2},
			{
				"Frontend_Bound": 3,
				"Backend_Bound":  4},
		}
	)

	p, err := NewProfiler(0, 100, 0, 1, "", fileName, -1)
	require.NoError(t, err, "Cannot create a profiler instance")

	type testCase struct {
		warmTime, tearDownTime float64
		expected               map[string]float64
	}

	cases := []testCase{
		{warmTime: 0, tearDownTime: 2, expected: result[0]},
		{warmTime: 0, tearDownTime: 1, expected: result[1]},
		{warmTime: 1, tearDownTime: 2, expected: result[2]},
	}

	for _, tCase := range cases {
		err := createData()
		require.NoError(t, err, "Failed creating test file")
		testName := fmt.Sprintf("%.2f,%.2f", tCase.warmTime, tCase.tearDownTime)

		t.Run(testName, func(t *testing.T) {
			p.warmTime = tCase.warmTime
			p.tearDownTime = tCase.tearDownTime

			data, err := p.readCSV()
			require.NoError(t, err, "Failed reading data")
			require.EqualValues(t, tCase.expected, data, "results do not match")
		})
	}
}

func TestProfilerRun(t *testing.T) {
	fileName := "testFile"

	p, err := NewProfiler(-1, 100, 0, 1, "", fileName, -1)
	require.NoError(t, err, "Cannot create a profiler instance")
	err = p.Run()
	require.EqualError(t, err, "profiler execution time is less than 0s", "Failed running profiler")

	p, err = NewProfiler(0, 1, 0, 1, "", fileName, -1)
	require.NoError(t, err, "Cannot create a profiler instance")
	err = p.Run()
	require.EqualError(t, err, "profiler print interval is less than 10ms", "Failed running profiler")

	p, err = NewProfiler(0, 100, 0, 1, "", fileName, -1)
	require.NoError(t, err, "Cannot create a profiler instance")
	err = p.Run()
	require.NoError(t, err, "profiler run returned error: %v.", err)

	time.Sleep(1 * time.Second)
}

func createData() error {
	f, err := os.Create("test.csv")
	if err != nil {
		return err
	}
	defer f.Close()

	lines := []string{"Timestamp,CPUs,Area,Value,Unit,Description,Sample,Stddev,Multiplex,Bottleneck,Idle",
		"0.503247704,C0,Frontend_Bound,1,% Slots <,,,0.0,3.99,,Y",
		"0.503247704,C0,Backend_Bound,2,% Slots <,,,0.0,3.99,,Y",
		"1.503247704,C1,Frontend_Bound,3,% Slots <,,,0.0,3.99,,Y",
		"1.503247704,C1,Backend_Bound,4,% Slots,,,0.0,3.99,<==,Y"}

	for _, line := range lines {
		_, err := f.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}
