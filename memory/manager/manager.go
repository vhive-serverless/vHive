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
	"encoding/csv"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/vhive-serverless/vhive/metrics"
	"gonum.org/v1/gonum/stat"

	log "github.com/sirupsen/logrus"
)

const (
	serveUniqueMetric = "ServeUnique"
	installWSMetric   = "InstallWS"
	fetchStateMetric  = "FetchState"
)

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	MetricsModeOn bool
	UffdSockAddr  string // it could not be appropriate to put sock here
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	sync.Mutex
	MemoryManagerCfg
	instances         map[string]*SnapshotState // Indexed by vmID
	origins           map[string]string         // Track parent vm for vm loaded from snapshot
	startEpollingCh   chan struct{}
	startEpollingOnce sync.Once
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(cfg MemoryManagerCfg) *MemoryManager {
	log.Debug("Initializing the memory manager")

	m := new(MemoryManager)
	m.instances = make(map[string]*SnapshotState)
	m.origins = make(map[string]string)
	m.startEpollingCh = make(chan struct{}, 1)
	m.MemoryManagerCfg = cfg
	m.startEpollingOnce = sync.Once{}

	return m
}

// RegisterVM Registers a VM within the memory manager
func (m *MemoryManager) RegisterVM(cfg SnapshotStateCfg) error {
	m.Lock()
	defer m.Unlock()

	vmID := cfg.VMID

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Registering the VM with the memory manager")

	if _, ok := m.instances[vmID]; ok {
		logger.Error("VM already registered with the memory manager")
		return errors.New("VM already registered with the memory manager")
	}

	cfg.metricsModeOn = m.MetricsModeOn
	state := NewSnapshotState(cfg)

	m.instances[vmID] = state
	return nil
}

// RegisterVMFromSnap Registers a VM that is loaded from snapshot within the memory manager
func (m *MemoryManager) RegisterVMFromSnap(originVmID string, cfg SnapshotStateCfg) error {
	m.Lock()
	defer m.Unlock()

	vmID := cfg.VMID

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Registering the VM that loaded snapshot with the memory manager")

	if _, ok := m.instances[vmID]; ok {
		logger.Error("VM already registered with the memory manager")
		return errors.New("VM already registered with the memory manager")
	}

	cfg.metricsModeOn = m.MetricsModeOn
	state := NewSnapshotState(cfg)
	// state := m.instances[originVmID]

	m.origins[vmID] = originVmID
	m.instances[vmID] = state
	return nil
}

// DeregisterVM Deregisters a VM from the memory manager
func (m *MemoryManager) DeregisterVM(vmID string) error {
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Deregistering VM from the memory manager")

	state, ok := m.instances[vmID]
	if !ok {
		logger.Error("VM is not registered with the memory manager")
		return errors.New("VM is not registered with the memory manager")
	}

	if state.isActive {
		logger.Error("Failed to deactivate, VM still active")
		return errors.New("Failed to deactivate, VM still active")
	}

	delete(m.instances, vmID)
	delete(m.origins, vmID)

	return nil
}

// Activate Creates an epoller to serve page faults for the VM
func (m *MemoryManager) Activate(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Activating instance in the memory manager")

	var (
		ok      bool
		state   *SnapshotState
		readyCh chan int = make(chan int)
	)

	m.Lock()

	logger.Debug("TEST: Activate: fetch snapstate by vmID for UFFD")

	// originID, ok := m.origins[vmID]

	// if !ok {
	// 	logger.Debug("TEST: not loaded from snapshot")
	// }

	// state, ok = m.instances[originID]

	state, ok = m.instances[vmID]

	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isActive {
		logger.Error("VM already active")
		return errors.New("VM already active")
	}

	select {
	case <-m.startEpollingCh:
		if err := state.mapGuestMemory(); err != nil {
			logger.Error("Failed to map guest memory")
			return err
		}

		if err := state.getUFFD(); err != nil {
			logger.Error("Failed to get uffd")
			return err
		}

		state.setupStateOnActivate()

		go state.pollUserPageFaults(readyCh)

		<-readyCh

	case <-time.After(100 * time.Second):
		return errors.New("Uffd connection to firecracker timeout")
	default:
		return errors.New("Failed to start epoller")
	}

	return nil
}

// FetchState Fetches the working set file (or the whole guest memory) and the VMM state file
func (m *MemoryManager) FetchState(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Activating instance in the memory manager")

	var (
		ok     bool
		state  *SnapshotState
		tStart time.Time
		err    error
	)

	m.Lock()

	// originID, ok := m.origins[vmID]
	// if !ok {
	// 	logger.Debug("TEST: not loaded from snapshot")
	// }
	// state, ok = m.instances[originID]

	state, ok = m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("TEST(fetch state): VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isRecordReady && !state.IsLazyMode {
		if state.metricsModeOn {
			tStart = time.Now()
		}
		err = state.fetchState()
		if state.metricsModeOn {
			state.currentMetric.MetricMap[fetchStateMetric] = metrics.ToUS(time.Since(tStart))
		}
	}

	return err
}

// Deactivate Removes the epoller which serves page faults for the VM
func (m *MemoryManager) Deactivate(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Deactivating instance from the memory manager")

	var (
		state *SnapshotState
		ok    bool
	)

	m.Lock()

	state, ok = m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if !state.isEverActivated {
		return nil
	}

	if !state.isActive {
		logger.Error("VM not activated")
		return errors.New("VM not activated")
	}

	state.quitCh <- 0
	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.processMetrics()

	state.userFaultFD.Close()
	if !state.isRecordReady && !state.IsLazyMode {
		state.trace.ProcessRecord(state.GuestMemPath, state.WorkingSetPath)
	}

	state.isRecordReady = true
	state.isActive = false

	return nil
}

// DumpUPFPageStats Saves the per VM stats
func (m *MemoryManager) DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) error {
	var (
		statHeader []string
		stats      []string
	)

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Dumping stats about number of page faults")

	m.Lock()

	state, ok := m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isActive {
		logger.Error("Cannot get stats while VM is active")
		return errors.New("Cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return errors.New("Metrics mode is not on")
	}

	if state.IsLazyMode {
		statHeader, stats = getLazyHeaderStats(state, functionName)
	} else {
		statHeader, stats = getRecRepHeaderStats(state, functionName)
	}

	return writeUPFPageStats(metricsOutFilePath, statHeader, stats)
}

// DumpUPFLatencyStats Dumps latency stats collected for the VM
func (m *MemoryManager) DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Dumping stats about latency of UPFs")

	m.Lock()

	state, ok := m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isActive {
		logger.Error("Cannot get stats while VM is active")
		return errors.New("Cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return errors.New("Metrics mode is not on")
	}

	return metrics.PrintMeanStd(latencyOutFilePath, functionName, state.latencyMetrics...)

}

// GetUPFLatencyStats Returns the gathered metrics for the VM
func (m *MemoryManager) GetUPFLatencyStats(vmID string) ([]*metrics.Metric, error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("returning stats about latency of UPFs")

	m.Lock()

	state, ok := m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return nil, errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isActive {
		logger.Error("Cannot get stats while VM is active")
		return nil, errors.New("Cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return nil, errors.New("Metrics mode is not on")
	}

	return state.latencyMetrics, nil
}

func (m *MemoryManager) ListenUffdSocket(uffdSockAddr string) error {
	log.Debug("Start listening to uffd socket")

	m.startEpollingOnce.Do(func() {
		m.startEpollingCh = make(chan struct{})
	})

	ln, err := net.Listen("unix", uffdSockAddr)
	if err != nil {
		log.Errorf("Failed to listen on uffd socket: %v", err)
		return errors.New("Failed to listen on uffd socket")
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("Failed to accept connection on uffd socket: %v", err)
			continue
		}
		go func(conn net.Conn) {
			defer conn.Close()
			if err := ln.Close(); err != nil {
				log.Printf("Failed to close uffd socket listener: %v", err)
			}
			close(m.startEpollingCh)
		}(conn)
		break
	}

	return nil
}

// Deprecated
// func (m *MemoryManager) GetUPFSockPath(vmID string, isSnapshotReady bool) (string, error) {
// 	logger := log.WithFields(log.Fields{"vmID": vmID})

// 	logger.Debug("Get the path of firecracker unix domain socket")

// 	m.Lock()

// 	// id := ""
// 	// if isSnapshotReady {
// 	// 	logger.Debugf("TEST: to find originID by vmID %s", vmID)
// 	// 	originID, ok := m.origins[vmID]
// 	// 	if !ok {
// 	// 		logger.Debug("TEST: not loaded from snapshot")
// 	// 	}
// 	// 	id = originID
// 	// }
// 	// state, ok := m.instances[id]

// 	state, ok := m.instances[vmID]
// 	if !ok {
// 		m.Unlock()
// 		logger.Error("VM not registered with the memory manager")
// 		return "", errors.New("VM not registered with the memory manager")
// 	}

// 	m.Unlock()

// 	if state.isActive {
// 		logger.Error("Cannot get stats while VM is active")
// 		return "", errors.New("Cannot get stats while VM is active")
// 	}

// 	return m.instances[vmID].SnapshotStateCfg.InstanceSockAddr, nil
// }

func getLazyHeaderStats(state *SnapshotState, functionName string) ([]string, []string) {
	header := []string{
		"FuncName",
		"RecPages",
		"RepPages",
		"StdDev",
		"Reused",
		"StdDev",
		"Unique",
		"StdDev",
	}

	uniqueMean, uniqueStd := stat.MeanStdDev(state.uniquePFServed, nil)
	totalMean, totalStd := stat.MeanStdDev(state.totalPFServed, nil)
	reusedMean, reusedStd := stat.MeanStdDev(state.reusedPFServed, nil)

	stats := []string{
		functionName,
		strconv.Itoa(len(state.trace.trace)), // number of records (i.e., offsets)
		strconv.Itoa(int(totalMean)),         // number of pages served
		fmt.Sprintf("%.1f", totalStd),
		strconv.Itoa(int(reusedMean)), // number of pages found in the trace
		fmt.Sprintf("%.1f", reusedStd),
		strconv.Itoa(int(uniqueMean)), // number of pages not found in the trace
		fmt.Sprintf("%.1f", uniqueStd),
	}

	return header, stats
}

func getRecRepHeaderStats(state *SnapshotState, functionName string) ([]string, []string) {
	header := []string{
		"FuncName",
		"RecPages",
		"RecRegions",
		"Unique",
		"StdDev",
	}

	uniqueMean, uniqueStd := stat.MeanStdDev(state.uniquePFServed, nil)

	stats := []string{
		functionName,
		strconv.Itoa(len(state.trace.trace)),   // number of records (i.e., offsets)
		strconv.Itoa(len(state.trace.regions)), // number of contiguous regions in the trace
		strconv.Itoa(int(uniqueMean)),          // number of pages not found in the trace
		fmt.Sprintf("%.1f", uniqueStd),
	}

	return header, stats
}

func writeUPFPageStats(metricsOutFilePath string, statHeader, stats []string) error {
	csvFile, err := os.OpenFile(metricsOutFilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Error("Failed to create csv file for writing stats")
		return err
	}
	defer csvFile.Close()

	writer := csv.NewWriter(csvFile)
	defer writer.Flush()

	fileInfo, err := csvFile.Stat()
	if err != nil {
		log.Errorf("Failed to stat csv file: %v", err)
		return err
	}

	if fileInfo.Size() == 0 {
		if err := writer.Write(statHeader); err != nil {
			log.Errorf("Failed to write header to csv file: %v", err)
			return err
		}
	}

	if err := writer.Write(stats); err != nil {
		log.Errorf("Failed to write to csv file: %v ", err)
		return err
	}

	return nil
}
