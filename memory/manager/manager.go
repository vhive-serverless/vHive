package manager

import (
	"os"

	"github.com/ftrvxmtrx/fd"
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
	snapStateMap map[string]*SnapshotState // indexed by vmID
	fdTraceMap   map[int]*Trace            // Indexed by FD
	epfd         int
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
func (v *MemoryManager) AddInstance(vmID, state *SnapshotState) (err error) {
	log.Debugf("Adding instance to the memory manager")

	var (
		event syscall.EpollEvent
		fd    int
	)

	if _, ok := v.snapStateMap[vmID]; ok {
		// PLAMEN: Return Error
	}

	// receive the fd from the socket
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L32
	state.getUFFD()

	// add the fd to the interesting (epolled) events list, and also to snapStateMap,
	// with epollCtl(..EPOLL_CTL_ADD..)
	fd = int(state.UserFaultFD.Fd())

	v.snapStateMap[vmID] = state
	v.fdTraceMap[fd] = state.Trace

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fd)

	if e := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_ADD, fd, &event); e != nil {
		log.Fatalf("epoll_ctl: %v", e)
		os.Exit(1)
	}

	// mmap the guest memory file for lazy access (default)
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L384
	mapGuestMemory(state.trace)

	return
}

// RemoveInstance Receives a file descriptor by sockAddr from the hypervisor
func (v *MemoryManager) RemoveInstance(vmID string) {
	var (
		state SnapshotState
		fd    int
		ok    bool
	)

	state, ok = v.snapStateMap[vmID]
	if !ok {
		// PLAMEN: return error
	}

	// remove the VM's fd from the interesting (epolled) events list, and also
	// from the snapStateMap, with epollCtl(..EPOLL_CTL_DEL..)
	fd = int(state.UserFaultFD.Fd())
	if _, ok = v.fdTraceMap[fd]; !ok {
		// PLAMEN: Return Error
	}

	if e := syscall.EpollCtl(v.epfd, syscall.EPOLL_CTL_DEL, fd, &event); e != nil {
		log.Fatalf("epoll_ctl: %v", e)
		os.Exit(1)
	}

	// munmap the guest memory file
	// https://github.com/ustiugov/staged-data-tiering/blob/88b9e51b6c36e82261f0937a66e08f01ab9cf941/fc_load_profiler/uffd.go#L403
	unmapGuestMemory(state.trace)

	delete(v.snapStateMap, vmID)
	delete(v.fdTraceMap, fd)
}

// FetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (v *MemoryManager) FetchState(vmID string) (err error) {
	return
}

func (s *SnapshotState) getUFFD() {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", s.InstanceSockAddr)
		if err != nil {
			time.Sleep(1 * time.Millisecond)
			continue
		}

		defer c.Close()

		sendfdConn := c.(*net.UnixConn)

		fs, err := fd.Get(sendfdConn, 1, []string{"a file"})
		if err != nil {
			log.Fatalf("Failed to receive the uffd: %v", err)
		}

		s.UserFaultFD = fs[0]
	}
}

func mapGuestMemory(trace *Trace) {
	fd, err := os.OpenFile(trace.guestMemFileName, os.O_RDONLY, 0600)
	if err != nil {
		log.Fatalf("Failed to open guest memory file: %v", err)
	}

	prot := unix.PROT_READ

	flags := unix.MAP_PRIVATE
	if trace.isPrefault {
		flags |= unix.MAP_POPULATE
	}

	trace.guestMem, err = unix.Mmap(int(fd.Fd()), 0, trace.guestMemSize, prot, flags)
	if err != nil {
		log.Fatalf("Failed to mmap guest memory file: %v", err)
	}
}

func unmapGuestMemory(trace *Trace) {
	if err := unix.Munmap(trace.guestMem); err != nil {
		log.Fatalf("Failed to munmap guest memory file: %v", err)
	}
}
