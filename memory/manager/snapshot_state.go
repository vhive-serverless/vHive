package manager

import (
	"context"
	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"unsafe"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	vmID                                         string
	vMMStatePath, guestMemPath, instanceSockAddr string
	memManagerBaseDir                            string
	isRecordMode                                 bool
	guestMemSize                                 int
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	startAddressOnce *sync.Once // to check if start address has been initialized
	startAddress     uint64
	baseDir          string
	userFaultFD      *os.File
	userFaultFDInt   int
	trace            *Trace
	epfd             int
	quitCh           chan int

	// install the whole working set in the guest memory
	isReplayWorkingSet bool
	// prefetch the VMM state to the host memory
	isPrefetchVMMState bool

	isWSCopy     bool
	isReplayDone bool

	servedNum int

	guestMem   []byte
	workingSet []byte
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg *SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.startAddressOnce = new(sync.Once)
	s.SnapshotStateCfg = *cfg

	s.createDir()

	s.trace = initTrace(s.getTraceFile())

	s.quitCh = make(chan int)

	return s
}

func (s *SnapshotState) getUFFD() {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", s.instanceSockAddr)
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

		s.userFaultFD = fs[0]

		return
	}
}

func (s *SnapshotState) createDir() {
	path := filepath.Join(s.memManagerBaseDir, s.vmID)
	if err := os.MkdirAll(path, 0666); err != nil {
		log.Fatalf("Failed to create snapshot state dir for VM %s", s.vmID)
	}
	s.baseDir = path
}

func (s *SnapshotState) getTraceFile() string {
	return filepath.Join(s.baseDir, "trace_"+s.vmID)
}

func (s *SnapshotState) mapGuestMemory() error {
	fd, err := os.OpenFile(s.guestMemPath, os.O_RDONLY, 0666)
	if err != nil {
		log.Errorf("Failed to open guest memory file: %v", err)
		return err
	}

	s.guestMem, err = unix.Mmap(int(fd.Fd()), 0, s.guestMemSize, unix.PROT_READ, unix.MAP_PRIVATE)
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

func (s *SnapshotState) pollUserPageFaults(readyCh chan int) {
	logger := log.WithFields(log.Fields{"vmID": s.vmID})

	var (
		events [MaxVMsNum]syscall.EpollEvent
		err    error
	)

	logger.Debug("Starting polling loop")

	s.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		logger.Fatalf("epoll_create1: %v", err)
		os.Exit(1)
	}
	defer syscall.Close(s.epfd)

	close(readyCh)

	logger.Debug("Polling loop running")

	for {
		select {
		case <-s.quitCh:
			logger.Debug("Handler received a signal to quit")
			return
		default:
			logger.Debug("Calling epoll_wait")
			nevents, e := syscall.EpollWait(s.epfd, events[:], -1)
			if e != nil {
				logger.Fatalf("epoll_wait: %v", e)
				break
			}
			logger.Debugf("Received %d events\n", nevents)

			if nevents < 1 {
				panic("Wrong number of events")
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)
				logger.Debugf("Received event for fd=%d\n", fd)

				if fd != s.userFaultFDInt {
					logger.Fatalf("Received event from unknown fd")
				}

				address := extractPageFaultAddress(fd)

				s.startAddressOnce.Do(
					func() {
						logger.Debug("Received page fault to start address")
						s.startAddress = address
					})

				s.servePageFault(fd, address)
				logger.Debugf("Finished serving page fault\n")
			}
		}
	}
}

func (s *SnapshotState) servePageFault(fd int, address uint64) {
	offset := address - s.startAddress

	log.Debugf("Serving offset: %d", offset)

	src := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(pageSize) - 1))
	mode := uint64(0)

	installRegion(fd, src, dst, mode, 1)
}
