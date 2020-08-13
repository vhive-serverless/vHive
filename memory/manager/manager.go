package manager

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"syscall"

	"github.com/ustiugov/fccd-orchestrator/metrics"
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

// DeregisterVM Deregisters a VM from the memory manager
func (m *MemoryManager) DeregisterVM(vmID string) error {
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Deregistering VM from the memory manager")

	if _, ok := m.instances[vmID]; !ok {
		logger.Error("VM is not registered with the memory manager")
		return errors.New("VM is not registered with the memory manager")
	}

	delete(m.instances, vmID)

	return nil
}

// Activate Creates an epoller to serve page faults for the VM
// userFaultFDFile is for testing only
func (m *MemoryManager) Activate(vmID string, userFaultFDFile *os.File) (err error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Activating instance in the memory manager")

	var (
		event   syscall.EpollEvent
		fdInt   int
		ok      bool
		state   *SnapshotState
		readyCh chan int = make(chan int)
	)

	m.Lock()

	state, ok = m.instances[vmID]
	if !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	m.Unlock()

	if state.isActive {
		logger.Error("VM already active")
		return errors.New("VM already active")
	}

	if err := state.mapGuestMemory(); err != nil {
		logger.Error("Failed to map guest memory")
		return err
	}

	if userFaultFDFile == nil {
		if err := state.getUFFD(); err != nil {
			logger.Error("Failed to get uffd")
			return err
		}
	} else {
		state.userFaultFD = userFaultFDFile
	}

	state.isActive = true
	state.isEverActivated = true
	state.firstPageFaultOnce = new(sync.Once)
	state.quitCh = make(chan int)

	if state.metricsModeOn {
		state.uniqueNum = 0
		state.servedNum = 0
		state.currentMetric = metrics.NewMetric()
	}

	fdInt = int(state.userFaultFD.Fd())

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fdInt)

	state.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		logger.Error("Failed to create epoller")
		return err
	}

	if err := syscall.EpollCtl(state.epfd, syscall.EPOLL_CTL_ADD, fdInt, &event); err != nil {
		logger.Error("Failed to subscribe VM")
		return err
	}

	go state.pollUserPageFaults(readyCh)

	<-readyCh

	return nil
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

	if state.metricsModeOn && state.isRecordReady {
		state.totalPFServed = append(state.totalPFServed, float64(state.servedNum))
		state.uniquePFServed = append(state.uniquePFServed, float64(state.uniqueNum))
		state.reusedPFServed = append(
			state.reusedPFServed,
			float64(state.servedNum-state.uniqueNum),
		)
		state.latencyMetrics = append(state.latencyMetrics, state.currentMetric)
	}

	state.userFaultFD.Close()
	if !state.isRecordReady && !state.IsLazyMode {
		state.trace.ProcessRecord(state.GuestMemPath, state.WorkingSetPath)
	}

	state.isRecordReady = true
	state.isActive = false

	return nil
}

// DumpVMStats Saves the per VM stats
func (m *MemoryManager) DumpUPFPageStats(vmID, functionName, metricsOutFilePath string) (err error) {
	var statHeader = []string{
		"FuncName",
		"RecPages",
		"RecRegions",
		"Served",
		"StdDev",
		"Reused",
		"StdDev",
		"Unique",
		"StdDev",
	}

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Dumping stats about number of page faults")

	m.Lock()

	state, ok := m.instances[vmID]
	if !ok {
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

	totalMean, totalStd := stat.MeanStdDev(state.totalPFServed, nil)
	reusedMean, reusedStd := stat.MeanStdDev(state.reusedPFServed, nil)
	uniqueMean, uniqueStd := stat.MeanStdDev(state.uniquePFServed, nil)

	stats := []string{
		functionName,
		strconv.Itoa(len(state.trace.trace)),   // number of records (i.e., offsets)
		strconv.Itoa(len(state.trace.regions)), // number of contiguous regions in the trace
		strconv.Itoa(int(totalMean)),           // number of pages served
		fmt.Sprintf("%.1f", totalStd),
		strconv.Itoa(int(reusedMean)), // number of pages found in the trace
		fmt.Sprintf("%.1f", reusedStd),
		strconv.Itoa(int(uniqueMean)), // number of pages not found in the trace
		fmt.Sprintf("%.1f", uniqueStd),
	}

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
		log.Error("Failed to stat csv file")
		return err
	}

	if fileInfo.Size() == 0 {
		if err := writer.Write(statHeader); err != nil {
			log.Error("Failed to write header to csv file")
			return err
		}
	}

	if err := writer.Write(stats); err != nil {
		log.Error("Failed to write to csv file")
		return err
	}

	return nil
}

// DumpLatencyStats Dumps latency stats collected for the VM
func (m *MemoryManager) DumpUPFLatencyStats(vmID, functionName, latencyOutFilePath string) (err error) {
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Dumping stats about latency of UPFs")

	m.Lock()

	state, ok := m.instances[vmID]
	if !ok {
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
