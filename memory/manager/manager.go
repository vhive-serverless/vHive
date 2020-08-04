package manager

/*
#include "user_page_faults.h"
*/
import "C"

import (
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	log "github.com/sirupsen/logrus"
)

const (
	defaultMemManagerBaseDir = "/root/fccd-mem_manager"
	pageSize                 = 4096
)

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	RecordReplayModeEnabled bool
	MemManagerBaseDir       string
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	sync.Mutex
	MemoryManagerCfg
	inactive      map[string]*SnapshotState
	activeFdState map[int]*SnapshotState // indexed by FD
	activeVMFD    map[string]int         // Indexed by vmID
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(cfg MemoryManagerCfg) *MemoryManager {
	log.Debug("Initializing the memory manager")

	m := new(MemoryManager)
	m.inactive = make(map[string]*SnapshotState)
	m.activeFdState = make(map[int]*SnapshotState)
	m.activeVMFD = make(map[string]int)

	m.MemoryManagerCfg = cfg
	if m.MemManagerBaseDir == "" {
		m.MemManagerBaseDir = defaultMemManagerBaseDir
	}
	if err := os.MkdirAll(m.MemManagerBaseDir, 0777); err != nil {
		log.Fatal("Failed to create mem manager base dir", err)
	}

	return m
}

// RegisterVM Registers a VM within the memory manager
func (m *MemoryManager) RegisterVM(cfg SnapshotStateCfg) error {
	m.Lock()
	defer m.Unlock()

	vmID := cfg.VMID

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Registering the VM with the memory manager")

	if _, ok := m.inactive[vmID]; ok {
		logger.Error("VM already registered the memory manager")
		return errors.New("VM exists in the memory manager")
	}

	if _, ok := m.activeVMFD[vmID]; ok {
		logger.Error("VM already active in the memory manager")
		return errors.New("VM already active in the memory manager")
	}

	state := NewSnapshotState(cfg)

	m.inactive[vmID] = state

	return nil
}

// DeregisterVM Deregisters a VM from the memory manager
func (m *MemoryManager) DeregisterVM(vmID string) error {
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Deregistering VM from the memory manager")

	if _, ok := m.inactive[vmID]; !ok {
		logger.Error("VM is not register or is still active in the memory manager")
		return errors.New("VM is not register or is still active in the memory manager")
	}

	delete(m.inactive, vmID)

	return nil
}

// Activate Creates an epoller to serve page faults for the VM
// userFaultFDFile is for testing only
func (m *MemoryManager) Activate(vmID string, userFaultFDFile *os.File) (err error) {
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Adding instance to the memory manager")

	var (
		event   syscall.EpollEvent
		fdInt   int
		ok      bool
		state   *SnapshotState
		readyCh chan int = make(chan int)
	)

	state, ok = m.inactive[vmID]
	if !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	if _, ok = m.activeVMFD[vmID]; ok {
		logger.Error("VM exists in the memory manager")
		return errors.New("VM exists in the memory manager")
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

	fdInt = int(state.userFaultFD.Fd())

	m.activate(vmID, fdInt, state)

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
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Removing instance from the memory manager")

	var (
		state *SnapshotState
		fdInt int
		ok    bool
	)

	if _, ok := m.inactive[vmID]; ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	fdInt, ok = m.activeVMFD[vmID]
	if !ok {
		logger.Error("Failed to find fd")
		return errors.New("Failed to find fd")
	}

	state, ok = m.activeFdState[fdInt]
	if !ok {
		logger.Error("Failed to find snapshot state")
		return errors.New("Failed to find snapshot state")
	}

	close(state.quitCh)

	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.userFaultFD.Close()

	m.deactivate(vmID, state)

	return nil
}

func (m *MemoryManager) activate(vmID string, fd int, state *SnapshotState) {
	delete(m.inactive, vmID)
	m.activeVMFD[vmID] = fd
	m.activeFdState[fd] = state
}

func (m *MemoryManager) deactivate(vmID string, state *SnapshotState) {
	delete(m.activeFdState, m.activeVMFD[vmID])
	delete(m.activeVMFD, vmID)
	m.inactive[vmID] = state
}

// FetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (m *MemoryManager) FetchState(vmID string) (err error) {
	// NOT IMPLEMENTED
	return nil
}

func installRegion(fd int, src, dst, mode, len uint64) error {
	cUC := C.struct_uffdio_copy{
		mode: C.ulonglong(mode),
		copy: 0,
		src:  C.ulonglong(src),
		dst:  C.ulonglong(dst),
		len:  C.ulonglong(pageSize * len),
	}

	err := ioctl(uintptr(fd), int(C.const_UFFDIO_COPY), unsafe.Pointer(&cUC))
	if err != nil {
		return err
	}

	return nil
}

func ioctl(fd uintptr, request int, argp unsafe.Pointer) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		fd,
		uintptr(request),
		// Note that the conversion from unsafe.Pointer to uintptr _must_
		// occur in the call expression.  See the package unsafe documentation
		// for more details.
		uintptr(argp),
	)
	if errno != 0 {
		return os.NewSyscallError("ioctl", fmt.Errorf("%d", int(errno)))
	}

	return nil
}

func registerForUpf(startAddress []byte, len uint64) int {
	return int(C.register_for_upf(unsafe.Pointer(&startAddress[0]), C.ulong(len)))
}

func sizeOfUFFDMsg() int {
	return C.sizeof_struct_uffd_msg
}
