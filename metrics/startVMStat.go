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
	fmt.Printf("StartVM Stats\t\tus\n")
	fmt.Printf("GetImage\t\t%d\n", s.GetImage)
	fmt.Printf("FcCreateVM\t\t%d\n", s.FcCreateVM)
	fmt.Printf("NewContainer\t\t%d\n", s.NewContainer)
	fmt.Printf("NewTask\t\t%d\n", s.NewTask)
	fmt.Printf("TaskWait\t\t%d\n", s.TaskWait)
	fmt.Printf("TaskStart\t\t%d\n", s.TaskStart)
	fmt.Printf("Total\t\t%d\n", s.Total())
}

// Aggregate Aggregates multiple stats into one
func (s *StartVMStat) Aggregate(otherStats ...*StartVMStat) *StartVMStat {
	agg := NewStartVMStat()

	otherStats = append(otherStats, s)

	for _, s := range otherStats {
		agg.GetImage += s.GetImage
		agg.FcCreateVM += s.FcCreateVM
		agg.NewContainer += s.NewContainer
		agg.NewTask += s.NewTask
		agg.TaskWait += s.TaskWait
		agg.TaskStart += s.TaskStart
	}

	return agg
}
