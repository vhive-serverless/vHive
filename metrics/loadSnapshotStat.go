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
	"fmt"

	"gonum.org/v1/gonum/stat"
)

// NewLoadSnapshotStat Create a new LoadSnapshotStat
func NewLoadSnapshotStat() *LoadSnapshotStat {
	s := new(LoadSnapshotStat)
	return s
}

// Total Calculates the total time taken to LoadSnapshot
func (s *LoadSnapshotStat) Total() int64 {
	return s.Full
}

// PrintTotal Prints the total time taken to LoadSnapshot
func (s *LoadSnapshotStat) PrintTotal() {
	fmt.Printf("LoadSnapshot total: %d us\n", s.Total())
}

// PrintAll Prints a breakdown of the time it took to LoadSnapshot
func (s *LoadSnapshotStat) PrintAll() {
	fmt.Printf("LoadSnap Stats\tus\n")
	fmt.Printf("Full          \t%d\n", s.Full)
	fmt.Printf("Total         \t%d\n", s.Total())
}

// PrintLoadSnapshotStats prints the mean and
// standard deviation of each component of
// LoadSnapshot statistics
func PrintLoadSnapshotStats(loadSnapshotStats ...*LoadSnapshotStat) {
	fulls := make([]float64, len(loadSnapshotStats))
	totals := make([]float64, len(loadSnapshotStats))

	for i, s := range loadSnapshotStats {
		fulls[i] = float64(s.Full)
		totals[i] = float64(s.Total())
	}

	var (
		mean float64
		std  float64
	)
	fmt.Printf("LoadSnap Stats\tMean(us)\tStdDev(us)\n")
	mean, std = stat.MeanStdDev(fulls, nil)
	fmt.Printf("Full          \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(totals, nil)
	fmt.Printf("Total         \t%12.2f\t%12.2f\n", mean, std)
}

// Aggregate Aggregates multiple stats into one
func (s *LoadSnapshotStat) Aggregate(otherStats ...*LoadSnapshotStat) {
	for _, other := range otherStats {
		s.Full += other.Full
	}
}
