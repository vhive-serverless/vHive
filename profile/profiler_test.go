package profile

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

<<<<<<< HEAD
func TestReadPerfData(t *testing.T) {
	var (
		fileName = "test"
		p        = NewProfiler(0, 100, 0, 1, "", fileName, false)
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

	type testCase struct {
		warmTime, tearDownTime float64
		expected               map[string]float64
=======
type testCase struct {
	warmTime, tearDownTime float64
	expected               map[string]float64
}

func TestReadPerfData(t *testing.T) {
	fileName := "test"

	p := NewProfiler(0, 100, 1, "", fileName)

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
>>>>>>> integrate PMU tool to benchmark
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

<<<<<<< HEAD
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
=======
// func TestCalculateMetric(t *testing.T) {
// 	perfVals := map[string]float64{
// 		"idq_uops_not_delivered.cycles_0_uops_deliv.core": 10,
// 		"cpu_clk_unhalted.thread":                         10,
// 		"cycle_activity.stalls_mem_any":                   0.1,
// 	}

// 	result, err := calculateMetric("", perfVals)
// 	require.EqualError(t, err, "the metric does not exist", "Failed popping not exist error")

// 	result, err = calculateMetric("Fetch_Latency", perfVals)
// 	require.Equal(t, 1., result, "metric calculation is incorrect")
// 	require.NoError(t, err, "Failed calculating metric")
// }

// func TestGetEvents(t *testing.T) {
// 	metrics := getMetrics()

// 	_, err := getEvents(metrics, "")
// 	require.EqualError(t, err, "the metric does not exist", "Failed popping not exist error")

// 	events, err := getEvents(metrics, "DRAM_Bound")
// 	require.NoError(t, err, "metric is not found")
// 	expected := []string{"mem_load_uops_retired.l3_hit", "mem_load_uops_retired.l3_miss", "cycle_activity.stalls_l2_miss", "cpu_clk_unhalted.thread"}
// 	require.EqualValues(t, expected, events, "returned events do not match with expected events")

// 	events, err = getEvents(metrics, "Memory_Bound")
// 	require.NoError(t, err, "metric is not found")
// }

// func TestSameMetricEntry(t *testing.T) {
// 	metrics := getMetrics()
// 	funcs := getMetricFuncMap()

// 	for metric := range metrics {
// 		_, isPresent := funcs[metric]
// 		require.True(t, isPresent, "metric %s is not found in functions", metric)
// 	}
// }

// func TestParseResult(t *testing.T) {
// 	fileName := "testFile.txt"
// 	// err := createPerfData(fileName)
// 	// require.NoError(t, err, "Failed creating test file")

// 	p := NewPerfStat(0, 100, "", "", fileName)
// 	p.warmTime = 0
// 	p.tearDownTime = 1

// 	data, err := p.parseResult()
// 	require.NoError(t, err, "Failed reading perf data")

// 	result := map[string]float64{"instructions": 2, "cycles": 3}

// 	require.EqualValues(t, result, data, "results do not match")
// }

// func TestGetResult(t *testing.T) {
// 	fileName := "testFile.txt"
// 	// err := createPerfData(fileName)
// 	// require.NoError(t, err, "Failed creating test file")

// 	p := NewPerfStat(0, 100, "", "", fileName)
// 	p.warmTime = 0
// 	p.tearDownTime = 1

// 	data, err := p.GetResult()
// 	require.EqualError(t, err, "Perf was not executed, run perf first", "Failed detecting perf status")

// 	p.tStart = time.Now()
// 	data, err = p.GetResult()
// 	require.NoError(t, err, "Failed retriving perf result")

// 	result := map[string]float64{"instructions": 2, "cycles": 3}
// 	require.EqualValues(t, result, data, "results do not match")
// }

func TestProfilerRun(t *testing.T) {
	fileName := "testFile"

	p := NewProfiler(-1, 100, 1, "", fileName)
	err := p.Run()
	require.EqualError(t, err, "perf execution time is less than 0s", "Failed creating perf stat")

	p = NewProfiler(0, 1, 1, "", fileName)
	err = p.Run()
	require.EqualError(t, err, "perf print interval is less than 10ms", "Failed creating perf stat")

	p = NewProfiler(0, 100, 1, "", fileName)

	err = p.Run()
	require.NoError(t, err, "Perf run returned error: %v.", err)
>>>>>>> integrate PMU tool to benchmark

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
