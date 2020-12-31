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
	err := createPerfData(fileName)
	require.NoError(t, err, "Failed creating test file")

	p := NewPerfStat("", fileName, 0, 0)

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

	for _, tCase := range cases {
		testName := fmt.Sprintf("%f,%f", tCase.warmTime, tCase.tearDownTime)

		t.Run(testName, func(t *testing.T) {
			p.warmTime = tCase.warmTime
			p.tearDownTime = tCase.tearDownTime

			data, err := p.readPerfData()
			require.NoError(t, err, "Failed reading perf data")

			for i := range data {
				for k, v := range data[i] {
					require.Equal(t, v, tCase.expected[i][k], "results do not match: actual %f, expected %f", v, tCase.expected[i][k])
				}
			}
		})
	}

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed deleting perf data")
}

func TestParseResult(t *testing.T) {
	fileName := "testFile.txt"
	err := createPerfData(fileName)
	require.NoError(t, err, "Failed creating test file")

	p := NewPerfStat("", fileName, 0, 0)
	p.warmTime = 0
	p.tearDownTime = 1

	data, err := p.parseResult()
	require.NoError(t, err, "Failed reading perf data")

	result := map[string]float64{"instructions": 2, "cycles": 3}

	for k, v := range data {
		require.Equal(t, v, result[k], "results do not match %f, %f", v, result[k])
	}

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed deleting perf data")
}

func TestGetResult(t *testing.T) {
	fileName := "testFile.txt"
	err := createPerfData(fileName)
	require.NoError(t, err, "Failed creating test file")

	p := NewPerfStat("", fileName, 0, 0)
	p.warmTime = 0
	p.tearDownTime = 1

	data, err := p.GetResult()
	require.EqualError(t, err, "Perf was not executed", "Failed detecting perf status")

	p.tStart = time.Now()
	data, err = p.GetResult()
	require.NoError(t, err, "Failed retriving perf result")

	result := map[string]float64{"instructions": 2, "cycles": 3}

	for k, v := range data {
		require.Equal(t, v, result[k], "results do not match %f, %f", v, result[k])
	}

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed deleting perf data")
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
