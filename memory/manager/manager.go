package manager

/*
#include "header.h"
*/
import "C"

import (
	"os"
	"sync"
	"unsafe"
	"golang.org/x/sys/unix"
	"errors"
	"syscall"
	"encoding/binary"
	"fmt"

	log "github.com/sirupsen/logrus"
)

const (
	DefaultMemManagerBaseDir = "/root/fccd-mem_manager"
	pageSize = 4096
	MaxVMsNum = 10000
)

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	RecordReplayModeEnabled bool
	MemManagerBaseDir string
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	sync.Mutex
	MemoryManagerCfg
	inactive map[string]*SnapshotState
	activeFdState map[int]*SnapshotState // indexed by FD
	activeVmFd   map[string]int         // Indexed by vmID
	epfd         int
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(cfg *MemoryManagerCfg, quitCh chan int) *MemoryManager {
	log.Debug("Inializing the memory manager")

	v := new(MemoryManager)
	v.inactive = make(map[string]*SnapshotState)
	v.activeFdState = make(map[int]*SnapshotState)
	v.activeVmFd = make(map[string]int)

	v.MemoryManagerCfg = *cfg
	if v.MemManagerBaseDir == "" {
		v.MemManagerBaseDir = DefaultMemManagerBaseDir
	}
	if err := os.MkdirAll(v.MemManagerBaseDir, 0666); err != nil {
		log.Fatal("Failed to create mem manager base dir", err)
	}

	readyCh := make(chan int)
	go v.pollUserPageFaults(readyCh, quitCh)

	<-readyCh

	return v
}

// RegisterVM Register a VM which is going to be
// managed by the memory manager
func (v *MemoryManager) RegisterVM(cfg *SnapshotStateCfg) error {
	v.Lock()
	defer v.Unlock()

	vmID := cfg.vmID

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Registering VM with the memory manager")

	if _, ok := v.inactive[vmID]; ok {
		logger.Error("VM already registered the memory manager")
		return errors.New("VM exists in the memory manager")
	}

	if _, ok := v.activeVmFd[vmID]; ok {
		logger.Error("VM already active in the memory manager")
		return errors.New("VM already active in the memory manager")
	}

	state := NewSnapshotState(cfg)

	v.inactive[vmID] = state

	logger.Debug("VM registered successfully")
	return nil
}

// DeregisterVM Deregisters a VM from the memory manager
func (v *MemoryManager) DeregisterVM(vmID string) error {
	v.Lock()
	defer v.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Degistering VM from the memory manager")

	if _, ok := v.inactive[vmID]; !ok {
		logger.Error("VM is not register or is still active in the memory manager")
		return errors.New("VM is not register or is still active in the memory manager")
	}

	delete(v.inactive, vmID)

	logger.Debug("Successfully degistered VM from the memory manager")
	return nil
}

// AddInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) AddInstance(vmID string, userFaultFDFile *os.File) (err error) {
	v.Lock()
	defer v.Unlock()
	
	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Adding instance to the memory manager")

	var (
		event syscall.EpollEvent
		fdInt    int
		ok bool
		state *SnapshotState
	)

	state, ok = v.inactive[vmID]
	if !ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	if _, ok = v.activeVmFd[vmID]; ok {
		logger.Error("VM exists in the memory manager")
		return errors.New("VM exists in the memory manager")
	}

	if err := state.mapGuestMemory(); err != nil {
		logger.Error("Failed to map guest memory")
		return err
	}

	if userFaultFDFile == nil {
		state.getUFFD()
	} else {
		state.userFaultFD = userFaultFDFile	
	}

	fdInt = int(state.userFaultFD.Fd())

	delete(v.inactive, vmID)
	v.activeVmFd[vmID] = fdInt
	v.activeFdState[fdInt] = state

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fdInt)

	if err := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_ADD, int(fdInt), &event); err != nil {
		logger.Error("Failed to subscribe VM")
		return err
	}

	logger.Debug("Instance added successfully")
	return nil
}

// RemoveInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) RemoveInstance(vmID string) error {
	v.Lock()
	defer v.Unlock()

	logger := log.WithFields(log.Fields{"vmID": vmID})

	logger.Debug("Removing instance from the memory manager")

	var (
		state *SnapshotState
		fdInt    int
		ok    bool
	)

	if _, ok := v.inactive[vmID]; ok {
		logger.Error("VM not registered with the memory manager")
		return errors.New("VM not registered with the memory manager")
	}

	fdInt, ok = v.activeVmFd[vmID]
	if !ok {
		logger.Error("Failed to find fd")
		return errors.New("Failed to find fd")
	}

	state, ok = v.activeFdState[fdInt]
	if !ok {
		logger.Error("Failed to find snapshot state")
		return errors.New("Failed to find snapshot state")
	}

	if err := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_DEL, fdInt, nil); err != nil {
		logger.Error("Failed to unsubscribe VM")
		return err
	}

	if err := state.unmapGuestMemory(); err != nil {
		logger.Error("Failed to munmap guest memory")
		return err
	}

	state.userFaultFD.Close()

	delete(v.activeFdState, fdInt)
	delete(v.activeVmFd, vmID)
	v.inactive[vmID] = state

	return nil
}

// FetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (v *MemoryManager) FetchState(vmID string) (err error) {
	// NOT IMPLEMENTED
	return nil
}

func (v *MemoryManager) pollUserPageFaults(readyCh, quitCh chan int) {
	var (
		events [MaxVMsNum]syscall.EpollEvent
		err error
	)

	log.Debug("Starting polling loop")

	v.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		log.Fatalf("epoll_create1: %v", err)
		os.Exit(1)
	}
	defer syscall.Close(v.epfd)

	close(readyCh)

	log.Debug("Polling loop running")

	for {
		select {
		case <-quitCh:
			log.Debug("Handler received a signal to quit")
			return
		default:
			fmt.Println("callin epoll_wait")
			nevents, e := syscall.EpollWait(v.epfd, events[:], -1)
			if e != nil {
				log.Fatalf("epoll_wait: %v", e)
				break
			}
			fmt.Println(nevents)
			if nevents < 1 {
				panic("Wrong number of events")
			}

			for i := 0; i < nevents; i++ {
				event := events[i]
				fd := int(event.Fd)
				_, ok := v.activeFdState[fd]
				if !ok {
					log.Fatalf("received event from file which is not active")
				}

				address := v.extractPageFaultAddress(fd)

				state := v.getSnapshotState(fd)
				state.startAddressOnce.Do(
					func() {
						log.Debug("Received page fault to start address")
						state.startAddress = address
					})
				go v.servePageFault(fd, address)
			}
		}
	}
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

func (v *MemoryManager) servePageFault(fd int, address uint64) {
	state := v.getSnapshotState(fd)
	offset := address - state.startAddress

	log.Debugf("Serving offset: %d", offset)

	src := uint64(uintptr(unsafe.Pointer(&state.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(pageSize) - 1))
	mode := uint64(0)

	installRegion(fd, src, dst, mode, 1)
}


func (v *MemoryManager) extractPageFaultAddress(fd int) uint64 {
	goMsg := make([]byte, C.sizeof_struct_uffd_msg)
	if nread, err := syscall.Read(fd, goMsg); err != nil || nread != len(goMsg) {
		log.Fatalf("Read uffd_msg failed: %v", err)
	}

	if event := uint8(goMsg[0]); event != uint8(C.const_UFFD_EVENT_PAGEFAULT) {
		log.Fatal("Received wrong event type")
	}

	return binary.LittleEndian.Uint64(goMsg[16:])
}

func (v *MemoryManager) getSnapshotState(fd int) *SnapshotState {
	if state, ok := v.activeFdState[fd]; ok {
		return state
	}
	log.Fatalf("getSnapshotState: fd not found")
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

func registerForUpf(startAddress []byte, len uint64) uintptr {
	return uintptr(C.register_for_upf(unsafe.Pointer(&startAddress[0]), C.ulong(len)))
}