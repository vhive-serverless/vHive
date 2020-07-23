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

// NewServeStat Creates a new ServeStat
func NewServeStat() *ServeStat {
	s := new(ServeStat)
	return s
}

// Total Calculates the total time it took to Serve
func (s *ServeStat) Total() int64 {
	return s.AddInstance + s.GetResponse + s.RetireOld
}

// PrintTotal Prints the total time to Serve
func (s *ServeStat) PrintTotal() {
	fmt.Printf("Serve total: %d us\n", s.Total())
}

// PrintAll Prints a breakdown of the time it took to Serve
func (s *ServeStat) PrintAll() {
	fmt.Printf("Serve Stats     \tus\n")
	fmt.Printf("AddInstance     \t%d\n", s.AddInstance)
	fmt.Printf("GetResponse     \t%d\n", s.GetResponse)
	fmt.Printf("RetireOld       \t%d\n", s.RetireOld)
	fmt.Printf("Total           \t%d\n", s.Total())
}

// PrintServeStats prints the mean and
// standard deviation of each component of
// ServeStat statistics
func PrintServeStats(serveStats ...*ServeStat) {
	addInstances := make([]float64, len(serveStats))
	getResponses := make([]float64, len(serveStats))
	retireOlds := make([]float64, len(serveStats))
	totals := make([]float64, len(serveStats))

	for i, s := range startVMstats {
		addInstances[i] = float64(s.AddInstance)
		getResponses[i] = float64(s.GetResponse)
		retireOlds[i] = float64(s.RetireOld)
		totals[i] = float64(s.Total())
	}

	var (
		mean float64
		std  float64
	)
	fmt.Printf("Serve Stats   \tMean(us)\tStdDev(us)\n")
	mean, std = stat.MeanStdDev(addInstances, nil)
	fmt.Printf("AddInstance   \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(getResponses, nil)
	fmt.Printf("GetResponse   \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(retireOlds, nil)
	fmt.Printf("RetireOld     \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(totals, nil)
	fmt.Printf("Total         \t%12.2f\t%12.2f\n", mean, std)
}
