package manager

import (
	"os"

	log "github.com/sirupsen/logrus"
)

// MemoryManagerCfg Global config of the manager
type MemoryManagerCfg struct {
	// clarify if necessary
	placeholder bool
}

// SnapshotState Holds the paths to snapshot files
type SnapshotState struct {
	VMMStatePath, GuestMemPath, WorkingSetPath string
	InstanceSockAddr                           string
	UserFaultFD                                *os.File
	trace                                      *Trace

	// Enables trace recording for the instance
	IsRecordMode bool
	// serve page faults one by one upon their occurence
	IsLazyServing bool
	// install the whole working set in the guest memory
	IsReplayWorkingSet bool
	// prefetch the VMM state to the host memory
	IsPrefetchVMMState bool
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState( /*...*/ ) *SnapshotState {
	s := new(SnapshotState)
	//trace = initTrace(/*...*/)
	// other fields

	return s
}

// MemoryManager Serves page faults coming from VMs
type MemoryManager struct {
	snapStateMap map[string]SnapshotState // indexed by vmID
}

// NewMemoryManager Initializes a new memory manager
func NewMemoryManager(quitCh chan int) *MemoryManager {
	v := new(MemoryManager)
	log.Debugf("Inializing the memory manager")

	v.snapStateMap = make(map[string]SnapshotState)
	// start the main (polling) loop in a goroutine
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L409

	// use select + cases to execute the infinite loop and also wait for a
	// message on quitCh channel to terminate the main loop

	return v
}

// AddInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) AddInstance(vmID, state SnapshotState) (err error) {
	log.Debugf("Adding instance to the memory manager")
	// receive the fd from the socket
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L32

	// add the fd to the interesting (epolled) events list, and also to snapStateMap,
	// with epollCtl(..EPOLL_CTL_ADD..)

	// mmap the guest memory file for lazy access (default)
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L384

	return
}

// RemoveInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) RemoveInstance(vmID string) {
	// remove the VM's fd from the interesting (epolled) events list, and also
	// from the snapStateMap, with epollCtl(..EPOLL_CTL_DEL..)

	// munmap the guest memory file
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L403
}

// FetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (v *MemoryManager) FetchState(vmID string) (err error) {
	return
}
