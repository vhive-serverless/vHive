package profile

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type testCase struct {
	warmTime, tearDownTime float64
	expected               map[string]float64
}

func TestReadPerfData(t *testing.T) {
	fileName := "test"

	p := NewProfiler(0, 100, 0, 1, "", fileName, false)

	result := []map[string]float64{
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

	cases := []testCase{
		{warmTime: 0, tearDownTime: 2, expected: result[0]},
		{warmTime: 0, tearDownTime: 1, expected: result[1]},
		{warmTime: 1, tearDownTime: 2, expected: result[2]},
	}

	for _, tCase := range cases {
		err := createData()
		require.NoError(t, err, "Failed creating test file")
		testName := fmt.Sprintf("%f,%f", tCase.warmTime, tCase.tearDownTime)

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

	p := NewProfiler(-1, 100, 0, 1, "", fileName, false)
	err := p.Run()
	require.EqualError(t, err, "profiler execution time is less than 0s", "Failed creating perf stat")

	p = NewProfiler(0, 1, 0, 1, "", fileName, false)
	err = p.Run()
	require.EqualError(t, err, "profiler print interval is less than 10ms", "Failed creating perf stat")

	p = NewProfiler(0, 100, 0, 1, "", fileName, false)

	err = p.Run()
	require.NoError(t, err, "profiler run returned error: %v.", err)

	time.Sleep(1 * time.Second)

	err = os.Remove(fileName + ".csv")
	require.NoError(t, err, "Failed removing test file.")
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
