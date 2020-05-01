// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov
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

package main

import (
	"errors"
	"fmt"
	"sort"
	"sync/atomic"
)

// FuncStat Per-function stats
type FuncStat struct {
	served  uint64
	started uint64
}

// Stats Stats for the cold functions in the function pool
type Stats struct {
	statMap map[string]*FuncStat
}

// NewStats Initializes per-function stat
func NewStats() *Stats {
	cs := new(Stats)
	cs.statMap = make(map[string]*FuncStat)

	return cs
}

// CreateStats Creates stats for a function
func (cs *Stats) CreateStats(fID string) error {
	if _, isPresent := cs.statMap[fID]; isPresent {
		return errors.New("Stat exists")
	}

	cs.statMap[fID] = new(FuncStat)

	return nil
}

// IncStarted Increments per-function instance-started counter
func (cs *Stats) IncStarted(fID string) {
	atomic.AddUint64(&cs.statMap[fID].started, 1)
}

// IncServed Increments per-function requests-served counter
func (cs *Stats) IncServed(fID string) {
	atomic.AddUint64(&cs.statMap[fID].served, 1)
}

// SprintStats Prints all stats
func (cs *Stats) SprintStats() string {
	var s = "==== Stats by cold functions ====\n"
	s += "fID, #started, #served\n"

	funcs := make([]string, 0, len(cs.statMap))
	for fID := range cs.statMap {
		funcs = append(funcs, fID)
	}
	sort.Strings(funcs)

	for _, fID := range funcs {
		s += fmt.Sprintf("%s, %d, %d\n", fID,
			atomic.LoadUint64(&cs.statMap[fID].started),
			atomic.LoadUint64(&cs.statMap[fID].served))
	}

	s += "==================================="

	return s
}
