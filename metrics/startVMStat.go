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

// NewStartVMStat Creates a new StartVMStat
func NewStartVMStat() *StartVMStat {
	s := new(StartVMStat)
	return s
}

// Total Calculates the total time it took to StartVM
func (s *StartVMStat) Total() int64 {
	return s.GetImage + s.FcCreateVM + s.NewContainer + s.NewTask + s.TaskWait + s.TaskStart
}

// PrintTotal Prints the total time to StartVM
func (s *StartVMStat) PrintTotal() {
	fmt.Printf("StartVM total: %d us\n", s.Total())
}

// PrintAll Prints a breakdown of the time it took to StartVM
func (s *StartVMStat) PrintAll() {
	fmt.Printf("StartVM Stats\tus\n")
	fmt.Printf("GetImage     \t%d\n", s.GetImage)
	fmt.Printf("FcCreateVM   \t%d\n", s.FcCreateVM)
	fmt.Printf("NewContainer \t%d\n", s.NewContainer)
	fmt.Printf("NewTask      \t%d\n", s.NewTask)
	fmt.Printf("TaskWait     \t%d\n", s.TaskWait)
	fmt.Printf("TaskStart    \t%d\n", s.TaskStart)
	fmt.Printf("Total        \t%d\n", s.Total())
}

// PrintStartVMStats prints the mean and
// standard deviation of each component of
// StartVM statistics
func PrintStartVMStats(startVMstats ...*StartVMStat) {
	getImages := make([]float64, len(startVMstats))
	fcCreateVMs := make([]float64, len(startVMstats))
	newContainers := make([]float64, len(startVMstats))
	newTasks := make([]float64, len(startVMstats))
	taskWaits := make([]float64, len(startVMstats))
	taskStarts := make([]float64, len(startVMstats))
	totals := make([]float64, len(startVMstats))

	for i, s := range startVMstats {
		getImages[i] = float64(s.GetImage)
		fcCreateVMs[i] = float64(s.FcCreateVM)
		newContainers[i] = float64(s.NewContainer)
		newTasks[i] = float64(s.NewTask)
		taskWaits[i] = float64(s.TaskWait)
		taskStarts[i] = float64(s.TaskStart)
		totals[i] = float64(s.Total())
	}

	var (
		mean float64
		std  float64
	)
	fmt.Printf("StartVM Stats\tMean(us)\tStdDev(us)\n")
	mean, std = stat.MeanStdDev(getImages, nil)
	fmt.Printf("GetImage     \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(fcCreateVMs, nil)
	fmt.Printf("FcCreateVM   \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(newContainers, nil)
	fmt.Printf("NewContainer \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(newTasks, nil)
	fmt.Printf("NewTask      \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(taskWaits, nil)
	fmt.Printf("TaskWait     \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(taskStarts, nil)
	fmt.Printf("TaskStart    \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(totals, nil)
	fmt.Printf("Total        \t%12.2f\t%12.2f\n", mean, std)
}

// Aggregate Aggregates multiple stats into one
func (s *StartVMStat) Aggregate(otherStats ...*StartVMStat) {
	for _, other := range otherStats {
		s.GetImage += other.GetImage
		s.FcCreateVM += other.FcCreateVM
		s.NewContainer += other.NewContainer
		s.NewTask += other.NewTask
		s.TaskWait += other.TaskWait
		s.TaskStart += other.TaskStart
	}
}
