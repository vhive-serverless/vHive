// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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

package manager

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/vhive-serverless/vhive/metrics"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID string

	VMMStatePath, GuestMemPath, WorkingSetPath string

	InstanceSockAddr string
	BaseDir          string // base directory for the instance
	MetricsPath      string // path to csv file where the metrics should be stored
	IsLazyMode       bool
	GuestMemSize     int
	metricsModeOn    bool
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	firstPageFaultOnce  *sync.Once
	userFaultFD         *os.File
	guestRegionMappings []GuestRegionUffdMapping
	trace               *Trace
	epfd                int
	wakeFD              int
	quitCh              chan int
	pollDoneCh          chan struct{}

	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isRecordReady bool

	guestMem   []byte
	workingSet []byte

	// Stats
	totalPFServed  []float64
	uniquePFServed []float64
	reusedPFServed []float64
	latencyMetrics []*metrics.Metric

	replayedNum   int
	uniqueNum     int
	currentMetric *metrics.Metric
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	cfg = normalizeSnapshotStateCfg(cfg)
	s.SnapshotStateCfg = cfg

	s.trace = initTrace(s.getTraceFile())
	if s.metricsModeOn {
		s.totalPFServed = make([]float64, 0)
		s.uniquePFServed = make([]float64, 0)
		s.reusedPFServed = make([]float64, 0)
		s.latencyMetrics = make([]*metrics.Metric, 0)
	}

	return s
}

func normalizeSnapshotStateCfg(cfg SnapshotStateCfg) SnapshotStateCfg {
	if cfg.WorkingSetPath == "" && cfg.BaseDir != "" {
		cfg.WorkingSetPath = filepath.Join(cfg.BaseDir, "working_set_pages")
	}
	return cfg
}

func (s *SnapshotState) refreshSnapshotLoad(cfg SnapshotStateCfg) {
	cfg = normalizeSnapshotStateCfg(cfg)

	trace := s.trace
	if trace == nil {
		trace = initTrace(filepath.Join(cfg.BaseDir, "trace"))
	} else {
		trace.traceFileName = filepath.Join(cfg.BaseDir, "trace")
	}
	isRecordReady := s.isRecordReady
	isEverActivated := s.isEverActivated
	totalPFServed := s.totalPFServed
	uniquePFServed := s.uniquePFServed
	reusedPFServed := s.reusedPFServed
	latencyMetrics := s.latencyMetrics

	s.SnapshotStateCfg = cfg
	s.firstPageFaultOnce = nil
	s.userFaultFD = nil
	s.guestRegionMappings = nil
	s.trace = trace
	s.epfd = 0
	s.wakeFD = -1
	s.quitCh = nil
	s.pollDoneCh = nil
	s.isEverActivated = isEverActivated
	s.isActive = false
	s.isRecordReady = isRecordReady
	s.guestMem = nil
	s.workingSet = nil
	s.totalPFServed = totalPFServed
	s.uniquePFServed = uniquePFServed
	s.reusedPFServed = reusedPFServed
	s.latencyMetrics = latencyMetrics
	s.replayedNum = 0
	s.uniqueNum = 0
	s.currentMetric = nil

	if s.metricsModeOn {
		if s.totalPFServed == nil {
			s.totalPFServed = make([]float64, 0)
		}
		if s.uniquePFServed == nil {
			s.uniquePFServed = make([]float64, 0)
		}
		if s.reusedPFServed == nil {
			s.reusedPFServed = make([]float64, 0)
		}
		if s.latencyMetrics == nil {
			s.latencyMetrics = make([]*metrics.Metric, 0)
		}
	}
}

func (s *SnapshotState) setupStateOnActivate() {
	s.isActive = true
	s.isEverActivated = true
	s.firstPageFaultOnce = new(sync.Once)
	s.wakeFD = -1
	s.quitCh = make(chan int, 1)
	s.pollDoneCh = make(chan struct{})

	if s.metricsModeOn {
		s.uniqueNum = 0
		s.replayedNum = 0
		if s.currentMetric == nil {
			s.currentMetric = metrics.NewMetric()
		}
	}
}

func (s *SnapshotState) processMetrics() {
	if !s.metricsModeOn || s.currentMetric == nil {
		return
	}

	if s.isRecordReady {
		if s.IsLazyMode {
			s.totalPFServed = append(s.totalPFServed, float64(s.replayedNum))
			s.reusedPFServed = append(s.reusedPFServed, float64(s.replayedNum-s.uniqueNum))
		}

		s.uniquePFServed = append(s.uniquePFServed, float64(s.uniqueNum))
		s.latencyMetrics = append(s.latencyMetrics, s.currentMetric)
	}
	s.currentMetric = nil
}

func (s *SnapshotState) getTraceFile() string {
	return filepath.Join(s.BaseDir, "trace")
}

func (s *SnapshotState) mapGuestMemory() error {
	fd, err := os.OpenFile(s.GuestMemPath, os.O_RDONLY, 0444)
	if err != nil {
		log.Errorf("Failed to open guest memory file: %v", err)
		return err
	}
	defer func() { _ = fd.Close() }()

	s.guestMem, err = unix.Mmap(int(fd.Fd()), 0, s.GuestMemSize, unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		log.Errorf("Failed to mmap guest memory file: %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) unmapGuestMemory() error {
	if err := unix.Munmap(s.guestMem); err != nil {
		log.Errorf("Failed to munmap guest memory file: %v", err)
		return err
	}

	return nil
}

// fetchState verifies snapshot state and loads the replay working set when ready.
func (s *SnapshotState) fetchState() error {
	if _, err := os.ReadFile(s.VMMStatePath); err != nil {
		log.Errorf("Failed to fetch VMM state: %v\n", err)
		return err
	}

	if s.isRecordReady && !s.IsLazyMode {
		return s.fetchWorkingSet()
	}

	return nil
}

func (s *SnapshotState) fetchWorkingSet() error {
	pageSize := s.trace.pageSize
	if pageSize == 0 {
		pageSize = uint64(os.Getpagesize())
	}

	size := uint64(len(s.trace.trace)) * pageSize
	if size > uint64(int(^uint(0)>>1)) {
		return fmt.Errorf("working set too large: %#x", size)
	}
	if size == 0 {
		s.workingSet = nil
		return nil
	}

	f, err := os.Open(s.WorkingSetPath)
	if err != nil {
		log.Errorf("Failed to open the working set file: %v\n", err)
		return err
	}
	defer func() { _ = f.Close() }()

	s.workingSet = make([]byte, int(size))
	n, err := io.ReadFull(f, s.workingSet)
	if err != nil {
		log.Errorf("Reading working set file failed: %v\n", err)
		return err
	}
	if n != len(s.workingSet) {
		return io.ErrUnexpectedEOF
	}

	log.Debug("Fetched the entire working set")
	return nil
}
