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
	expected               []map[string]float64
}

func TestReadPerfData(t *testing.T) {
	fileName := "testFile.txt"

	p := NewPerfStat(0, 100, "", "", fileName)

	result := []map[string]float64{
		map[string]float64{
			"instructions": 1,
			"cycles":       2},
		map[string]float64{
			"instructions": 3,
			"cycles":       4},
	}

	cases := []testCase{
		{warmTime: 0, tearDownTime: 1, expected: result},
		{warmTime: 0, tearDownTime: 0.5, expected: []map[string]float64{result[0]}},
		{warmTime: 0.5, tearDownTime: 1, expected: []map[string]float64{result[1]}},
	}

	_, err := p.readPerfData()
	require.EqualError(t, err, "Perf was failed to execute, check perf events", "Failed detecting perf status")

	for _, tCase := range cases {
		err := createPerfData(fileName)
		require.NoError(t, err, "Failed creating test file")
		testName := fmt.Sprintf("%f,%f", tCase.warmTime, tCase.tearDownTime)

		t.Run(testName, func(t *testing.T) {
			p.warmTime = tCase.warmTime
			p.tearDownTime = tCase.tearDownTime

			data, err := p.readPerfData()
			require.NoError(t, err, "Failed reading perf data")
			require.EqualValues(t, tCase.expected, data, "results do not match")
		})
	}
}

func TestCalculateMetric(t *testing.T) {
	result, err := calculateMetric("")
	require.EqualError(t, err, "the metric does not exist", "Failed popping not exist error")

	_, err = calculateMetric("L1_Bound", 0.1)
	require.EqualError(t, err, "the number of params does not match with input function", "Failed detecting unmatched parameters")

	result, err = calculateMetric("Fetch_Latency", 10, 20)
	require.Equal(t, 1., result, "metric calculation is incorrect")
	require.NoError(t, err, "Failed calculating metric")
}

func TestGetEvents(t *testing.T) {
	_, err := getEvents("")
	require.EqualError(t, err, "the metric does not exist", "Failed popping not exist error")

	events, err := getEvents("DTLB_Load")
	require.NoError(t, err, "Failed popping not exist error")

	expected := []string{"dtlb_load_misses.stlb_hit", "dtlb_load_misses.walk_duration", "dtlb_load_misses.walk_completed", "cpu_clk_unhalted.thread"}
	require.EqualValues(t, expected, events, "")
}

func TestParseResult(t *testing.T) {
	fileName := "testFile.txt"
	err := createPerfData(fileName)
	require.NoError(t, err, "Failed creating test file")

	p := NewPerfStat(0, 100, "", "", fileName)
	p.warmTime = 0
	p.tearDownTime = 1

	data, err := p.parseResult()
	require.NoError(t, err, "Failed reading perf data")

	result := map[string]float64{"instructions": 2, "cycles": 3}

	require.EqualValues(t, result, data, "results do not match")
}

func TestGetResult(t *testing.T) {
	fileName := "testFile.txt"
	err := createPerfData(fileName)
	require.NoError(t, err, "Failed creating test file")

	p := NewPerfStat(0, 100, "", "", fileName)
	p.warmTime = 0
	p.tearDownTime = 1

	data, err := p.GetResult()
	require.EqualError(t, err, "Perf was not executed, run perf first", "Failed detecting perf status")

	p.tStart = time.Now()
	data, err = p.GetResult()
	require.NoError(t, err, "Failed retriving perf result")

	result := map[string]float64{"instructions": 2, "cycles": 3}
	require.EqualValues(t, result, data, "results do not match")
}

func TestPerfRun(t *testing.T) {
	fileName := "testFile.txt"

	p := NewPerfStat(-1, 100, "", "", fileName)
	err := p.Run()
	require.EqualError(t, err, "perf execution time is less than 0s", "Failed creating perf stat")

	p = NewPerfStat(0, 1, "", "", fileName)
	err = p.Run()
	require.EqualError(t, err, "perf print interval is less than 10ms", "Failed creating perf stat")

	p = NewPerfStat(0, 100, "", "", fileName)

	err = p.Run()
	require.NoError(t, err, "Perf run returned error: %v.", err)

	time.Sleep(1 * time.Second)

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed removing test file.")
}

func createPerfData(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	lines := []string{"# started on Wed Dec 30 08:59:10 2020",
		"",
		"     0.302384780|1||instructions|6018741291|100.00|0.28|insn per cycle",
		"     0.302384780|2||cycles|6024130598|100.00||",
		"     0.603024038|3||instructions|6028674780|100.00|0.24|insn per cycle",
		"     0.603024038|4||cycles|6018134824|100.00||"}

	for _, line := range lines {
		_, err := f.WriteString(line + "\n")
		if err != nil {
			return err
		}
	}

	return nil
}
