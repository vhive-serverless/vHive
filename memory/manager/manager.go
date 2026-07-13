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
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	sync.Mutex
	MemoryManagerCfg
	instances map[string]*SnapshotState // Indexed by vmID
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(cfg MemoryManagerCfg) *MemoryManager {
	log.Debug("Initializing the memory manager")

	m := new(MemoryManager)
	m.instances = make(map[string]*SnapshotState)
	m.MemoryManagerCfg = cfg

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

// PrepareSnapshotLoad creates or refreshes the state used to serve UFFD faults
// while loading a VM snapshot.
func (m *MemoryManager) PrepareSnapshotLoad(cfg SnapshotStateCfg) error {
	m.Lock()
	defer m.Unlock()

	vmID := cfg.VMID
	if vmID == "" {
		return errors.New("VMID is required")
	}

	cfg.metricsModeOn = m.MetricsModeOn

	state, ok := m.instances[vmID]
	if !ok {
		m.instances[vmID] = NewSnapshotState(cfg)
		return nil
	}
	if state.isActive {
		return errors.New("failed to prepare snapshot load, VM still active")
	}
	if state.userFaultFD != nil {
		_ = state.userFaultFD.Close()
		state.userFaultFD = nil
	}

	state.refreshSnapshotLoad(cfg)
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
		return errors.New("failed to deactivate, VM still active")
	}

	delete(m.instances, vmID)

	return nil
}

// Activate creates an epoller to serve page faults and reports when the UFFD
// socket listener is ready for Firecracker to connect.
func (m *MemoryManager) Activate(vmID string, socketReadyCh chan<- error) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Activating instance in the memory manager")

	var (
		ok      bool
		state   *SnapshotState
		readyCh = make(chan error)
	)

	m.Lock()

	state, ok = m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		err := errors.New("VM not registered with the memory manager")
		notifySocketReady(socketReadyCh, err)
		return err
	}

	m.Unlock()

	if state.isActive {
		logger.Error("VM already active")
		err := errors.New("VM already active")
		notifySocketReady(socketReadyCh, err)
		return err
	}

	if err := state.mapGuestMemory(); err != nil {
		logger.Error("Failed to map guest memory")
		notifySocketReady(socketReadyCh, err)
		return err
	}

	if err := state.getUFFD(socketReadyCh); err != nil {
		logger.Error("Failed to get uffd")
		if unmapErr := state.unmapGuestMemory(); unmapErr != nil {
			logger.WithError(unmapErr).Error("Failed to munmap guest memory after getUFFD failure")
		}
		return err
	}

	state.setupStateOnActivate()

	go state.pollUserPageFaults(readyCh)

	if err := <-readyCh; err != nil {
		logger.WithError(err).Error("Failed to start UFFD page fault polling")
		if state.userFaultFD != nil {
			_ = state.userFaultFD.Close()
		}
		if unmapErr := state.unmapGuestMemory(); unmapErr != nil {
			logger.WithError(unmapErr).Error("Failed to munmap guest memory after UFFD poller failure")
		}
		return err
	}

	return nil
}

// FetchState verifies that snapshot state needed by the memory manager exists.
func (m *MemoryManager) FetchState(vmID string) error {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Fetching state in the memory manager")

	var (
		ok     bool
		state  *SnapshotState
		tStart time.Time
	)

	m.Lock()

	state, ok = m.instances[vmID]
	if !ok {
		m.Unlock()
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.metricsModeOn && state.currentMetric == nil {
		state.currentMetric = metrics.NewMetric()
	}
	if state.metricsModeOn && state.isRecordReady && !state.IsLazyMode {
		tStart = time.Now()
	}

	err := state.fetchState()
	if err == nil && !tStart.IsZero() {
		state.currentMetric.MetricMap[fetchStateMetric] = metrics.ToUS(time.Since(tStart))
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

	state.stopPolling()
	state.waitForPoller()
	state.closeWakeFD()

	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.processMetrics()

	if state.userFaultFD != nil {
		defer func() { _ = state.userFaultFD.Close() }()
	}

	if !state.isRecordReady && !state.IsLazyMode {
		pageSize, err := guestMappingPageSize(state.guestRegionMappings)
		if err != nil {
			return err
		}
		if err := state.trace.ProcessRecord(state.GuestMemPath, state.WorkingSetPath, pageSize); err != nil {
			return err
		}
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
		return errors.New("cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return errors.New("metrics mode is not on")
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
		return errors.New("cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return errors.New("metrics mode is not on")
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
		return nil, errors.New("cannot get stats while VM is active")
	}

	if !m.MetricsModeOn || !state.metricsModeOn {
		logger.Error("Metrics mode is not on")
		return nil, errors.New("metrics mode is not on")
	}

	return state.latencyMetrics, nil
}

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
		strconv.Itoa(len(state.trace.trace)),
		strconv.Itoa(int(totalMean)),
		fmt.Sprintf("%.1f", totalStd),
		strconv.Itoa(int(reusedMean)),
		fmt.Sprintf("%.1f", reusedStd),
		strconv.Itoa(int(uniqueMean)),
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
		strconv.Itoa(len(state.trace.trace)),
		strconv.Itoa(len(state.trace.regions)),
		strconv.Itoa(int(uniqueMean)),
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
	defer func() { _ = csvFile.Close() }()

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
