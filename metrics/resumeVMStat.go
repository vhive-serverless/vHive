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
	"bufio"
	"fmt"
	"os"

	"gonum.org/v1/gonum/stat"
)

// NewResumeVMStat Creates a new ResumeVMStat
func NewResumeVMStat() *ResumeVMStat {
	s := new(ResumeVMStat)
	return s
}

// Total Calculates the total time it took to ResumeVM
func (s *ResumeVMStat) Total() int64 {
	return s.FcResume + s.ReconnectFuncClient
}

// PrintTotal Prints the total time to ResumeVM
func (s *ResumeVMStat) PrintTotal() {
	fmt.Printf("ResumeVM total: %d us\n", s.Total())
}

// PrintAll Prints a breakdown of the time it took to ResumeVM
func (s *ResumeVMStat) PrintAll() {
	fmt.Printf("ResumeVM Stats        \tus\n")
	fmt.Printf("FcResume              \t%d\n", s.FcResume)
	fmt.Printf("ReconnectFuncClient   \t%d\n", s.ReconnectFuncClient)
	fmt.Printf("Total                 \t%d\n", s.Total())
}

// PrintResumeVMStats prints the mean and
// standard deviation of each component of
// ResumeVM statistics
func PrintResumeVMStats(resultsPath string, resumeVMstats ...*ResumeVMStat) {
	fcResumes := make([]float64, len(resumeVMstats))
	reconnectFuncClients := make([]float64, len(resumeVMstats))
	totals := make([]float64, len(resumeVMstats))

	for i, s := range resumeVMstats {
		fcResumes[i] = float64(s.FcResume)
		reconnectFuncClients[i] = float64(s.ReconnectFuncClient)
		totals[i] = float64(s.Total())
	}

	var (
		mean float64
		std  float64
		f    *os.File
		err  error
	)

	if resultsPath == "" {
		f = os.Stdout
	} else {
		f, err = os.Create(resultsPath)
		if err != nil {
			panic(err)
		}
		defer f.Close()
	}

	w := bufio.NewWriter(f)

	fmt.Fprintf(w, "ResumeVM Stats        \tMean(us)\tStdDev(us)\n")
	mean, std = stat.MeanStdDev(fcResumes, nil)
	fmt.Fprintf(w, "FcResume              \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(reconnectFuncClients, nil)
	fmt.Fprintf(w, "ReconnectFuncClient   \t%12.2f\t%12.2f\n", mean, std)
	mean, std = stat.MeanStdDev(totals, nil)
	fmt.Fprintf(w, "Total                 \t%12.2f\t%12.2f\n", mean, std)
	w.Flush()
}
