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

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	RecordReplayModeEnabled bool
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
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Activating instance in the memory manager")

	var (
		event   syscall.EpollEvent
		fdInt   int
		ok      bool
		state   *SnapshotState
		readyCh chan int = make(chan int)
	)

	state, ok = m.instances[vmID]
	if !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	if state.isActive {
		logger.Error("VM already active")
		return errors.New("VM already active")
	}
	state.isActive = true
	state.isEverActivated = true

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

	state.startAddressOnce = new(sync.Once)
	state.quitCh = make(chan int)

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
	m.Lock()
	defer m.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Deactivating instance from the memory manager")

	var (
		state *SnapshotState
		ok    bool
	)

	state, ok = m.instances[vmID]
	if !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	if !state.isEverActivated {
		return nil
	}

	if !state.isActive {
		logger.Error("VM not activated")
		return errors.New("VM not activated")
	}
	state.isActive = false

	state.quitCh <- 0

	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.userFaultFD.Close()

	return nil
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
		len:  C.ulonglong(uint64(os.Getpagesize()) * len),
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

func uffdPageFault() uint8 {
	return uint8(C.const_UFFD_EVENT_PAGEFAULT)
}
