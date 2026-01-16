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
	"encoding/csv"
	"os"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadResultCSV(t *testing.T) {
	fileName := "testFile.txt"
	err := createTestFile(fileName)
	require.NoError(t, err, "Failed creating test file")

	data := readResultCSV("", fileName)
	expected := [][]string{
		{"field1", "field2", "field/3"},
		{"1", "4", "7"},
		{"2", "5", "8"},
		{"3", "6", "9"},
	}

	for i := range data {
		for j := range data[i] {
			require.Equal(t, data[i][j], expected[i][j], "Data does not match: actual %s, expected %s", data[i][j], expected[i][j])
		}
	}

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed deleting test csv")
}

func TestCreatingPlotter(t *testing.T) {
	fileName := "testFile.txt"
	err := createTestFile(fileName)
	require.NoError(t, err, "Failed creating test file")

	PlotLineCharts(4, "", fileName, "X-axis")

	plotNames := []string{"field1.png", "field2.png", "field-3.png"}
	for _, fname := range plotNames {
		_, err := os.Stat(fname)
		require.False(t, os.IsNotExist(err), "Target file %s was not found", fname)
	}

	err = os.Remove(fileName)
	require.NoError(t, err, "Failed deleting test csv")
	for _, fname := range plotNames {
		err = os.Remove(fname)
		require.NoError(t, err, "Failed deleting plot %s", fname)
	}
}

func TestFindMetricGroup(t *testing.T) {
	metrics, err := loadMetrics("toplev_metrics.json")
	require.NoError(t, err, "Failed reading json file")

	testFields := map[string]int{"L1_Bound": 0, "L2_Bound": 0, "L3_Bound": 0, "DRAM_Bound": 0, "Store_Bound": 0,
		"Frontend_Bound": 0, "Bad_Speculation": 0, "Backend_Bound": 0, "Retiring": 0}
	expected := map[string][]string{
		"Level_1":      {"Backend_Bound", "Bad_Speculation", "Frontend_Bound", "Retiring"},
		"Memory_Bound": {"DRAM_Bound", "L1_Bound", "L2_Bound", "L3_Bound", "Store_Bound"},
	}
	results := findMetricGroup(metrics, testFields)
	for _, v := range results {
		sort.Strings(v)
	}
	require.Equal(t, expected, results, "returned wrong metric groups")
}

func createTestFile(filePath string) error {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	records := [][]string{
		{"field1", "field2", "field/3"},
		{"1", "4", "7"},
		{"2", "5", "8"},
		{"3", "6", "9"},
	}

	w := csv.NewWriter(f)
	err = w.WriteAll(records)
	if err != nil {
		return err
	}

	return nil
}
