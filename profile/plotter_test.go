package profile

import (
	"encoding/csv"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadResultCSV(t *testing.T) {
	fileName := "testFile.txt"
	err := createTestFile(fileName)
	require.NoError(t, err, "Failed creating test file")

	data := readResultCSV(fileName)
	expected := [][]string{
		{"field1", "field2", "field3"},
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

	CSVPlotter(fileName, "")

	plotNames := []string{"field1.png", "field2.png", "field3.png"}
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

func createTestFile(filePath string) error {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	records := [][]string{
		{"field1", "field2", "field3"},
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
