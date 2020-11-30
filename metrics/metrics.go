// MIT License
//
// Copyright (c) 2020 Plamen Petrov and EASE lab
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
	"encoding/csv"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/gonum/stat"
)

const (
	// FcResume Time it takes to resume a VM from containerd
	FcResume = "FcResume"
	// ConnectFuncClient Time it takes to reconnect function client
	ConnectFuncClient = "ConnectFuncClient"

	// LoadVMM Name of LoadVMM metric
	LoadVMM = "LoadVMM"

	// AddInstance Time to add instance - load snap or start vm
	AddInstance = "AddInstance"
	// FuncInvocation Time to get response from function
	FuncInvocation = "FuncInvocation"
	// RetireOld Time to offload/stop instance if threshold exceeded
	RetireOld = "RetireOld"

	// GetImage Time to pull docker image
	GetImage = "GetImage"
	// FcCreateVM Time to create VM
	FcCreateVM = "FcCreateVM"
	// NewContainer Time to create new container
	NewContainer = "NewContainer"
	// NewTask Time to create new task
	NewTask = "NewTask"
	// TaskWait Time to wait for task to be ready
	TaskWait = "TaskWait"
	// TaskStart Time to start task
	TaskStart = "TaskStart"
)

// Metric A general metric
type Metric struct {
	MetricMap map[string]float64
}

// NewMetric Create a new metric
func NewMetric() *Metric {
	m := new(Metric)
	m.MetricMap = make(map[string]float64)

	return m
}

// Total Calculates the total time per stat
func (m *Metric) Total() float64 {
	var sum float64
	for _, v := range m.MetricMap {
		sum += v
	}

	return sum
}

// PrintTotal Prints the total time
func (m *Metric) PrintTotal() {
	fmt.Printf("Total: %.1f us\n", m.Total())
}

// PrintAll Prints a breakdown of the time
func (m *Metric) PrintAll() {
	for k, v := range m.MetricMap {
		fmt.Printf("%s:\t%.1f\n", k, v)
	}
	fmt.Printf("Total\t%.1f\n", m.Total())
}

// PrintMeanStd prints the mean and standard
// deviation of each component of Metric
func PrintMeanStd(resultsPath, funcName string, metricsList ...*Metric) error {
	var (
		mean, std   float64
		f           *os.File
		err         error
		agg         map[string][]float64 = make(map[string][]float64)
		totals      []float64            = make([]float64, 0, len(metricsList))
		keys        []string             = make([]string, 0)
		forPrinting []string             = make([]string, 0)
		header                           = []string{"FuncName"}
	)

	if len(metricsList) == 0 {
		return nil
	}

	for k := range metricsList[0].MetricMap {
		keys = append(keys, k)
		agg[k] = make([]float64, 0, len(metricsList))
	}
	sort.Strings(keys)

	for _, key := range keys {
		header = append(header, key, "StdDev")
	}
	header = append(header, "Total", "StdDev")

	for _, m := range metricsList {
		totals = append(totals, m.Total())

		for k, v := range m.MetricMap {
			agg[k] = append(agg[k], v)
		}
	}

	if resultsPath == "" {
		f = os.Stdout
	} else {
		f, err = os.OpenFile(resultsPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			log.Error("Failed to open metrics output file")
			return err
		}
		defer f.Close()
	}

	w := csv.NewWriter(f)
	defer w.Flush()

	fileInfo, err := f.Stat()
	if err != nil {
		log.Error("Failed to stat output file")
		return err
	}

	if fileInfo.Size() == 0 {
		if err := w.Write(header); err != nil {
			log.Error("Failed to write header to csv file")
			return err
		}
	}

	forPrinting = append(forPrinting, funcName)

	for _, k := range keys {
		v := agg[k]
		mean, std = stat.MeanStdDev(v, nil)
		forPrinting = append(forPrinting, strconv.Itoa(int(mean)))
		forPrinting = append(forPrinting, fmt.Sprintf("%.1f", std))
	}

	mean, std = stat.MeanStdDev(totals, nil)
	forPrinting = append(forPrinting, strconv.Itoa(int(mean)))
	forPrinting = append(forPrinting, fmt.Sprintf("%.1f", std))

	if err := w.Write(forPrinting); err != nil {
		log.Error("Failed to write to csv file")
		return err
	}

	return nil
}

// ToUS Converts Duration to microseconds
func ToUS(dur time.Duration) float64 {
	return float64(dur.Microseconds())
}
