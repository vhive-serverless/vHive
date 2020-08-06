package manager

import (
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"unsafe"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID                                         string
	VMMStatePath, GuestMemPath, InstanceSockAddr string
	BaseDir                                      string // base directory for the instance
	IsRecordMode                                 bool
	GuestMemSize                                 int
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	startAddressOnce *sync.Once // to check if start address has been initialized
	startAddress     uint64
	userFaultFD      *os.File
	trace            *Trace
	epfd             int
	quitCh           chan int

	// install the whole working set in the guest memory
	isReplayWorkingSet bool
	// prefetch the VMM state to the host memory
	isPrefetchVMMState bool
	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isWSCopy     bool
	isReplayDone bool

	servedNum int

	guestMem   []byte
	workingSet []byte
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.SnapshotStateCfg = cfg

	s.trace = initTrace(s.getTraceFile())

	return s
}

func (s *SnapshotState) getUFFD() error {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", s.InstanceSockAddr)
		if err != nil {
			if ctx.Err() != nil {
				log.Error("Failed to dial within the context timeout")
				return err
			}
			time.Sleep(1 * time.Millisecond)
			continue
		}

		defer c.Close()

		sendfdConn := c.(*net.UnixConn)

		fs, err := fd.Get(sendfdConn, 1, []string{"a file"})
		if err != nil {
			log.Error("Failed to receive the uffd")
			return err
		}

		s.userFaultFD = fs[0]

		return nil
	}
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

func (s *SnapshotState) pollUserPageFaults(readyCh chan int) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var (
		events [1]syscall.EpollEvent
	)

	logger.Debug("Starting polling loop")

	defer syscall.Close(s.epfd)

	close(readyCh)

	for {
		select {
		case <-s.quitCh:
			logger.Debug("Handler received a signal to quit")
			return
		default:
			nevents, err := syscall.EpollWait(s.epfd, events[:], -1)
			if err != nil {
				logger.Fatalf("epoll_wait: %v", err)
				break
			}

			if nevents < 1 {
				panic("Wrong number of events")
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)

				stateFd := int(s.userFaultFD.Fd())

				if fd != stateFd && stateFd != -1 {
					logger.Fatalf("Received event from unknown fd")
				}

				goMsg := make([]byte, sizeOfUFFDMsg())

				if nread, err := syscall.Read(fd, goMsg); err != nil || nread != len(goMsg) {
					if !errors.Is(err, syscall.EBADF) {
						log.Fatalf("Read uffd_msg failed: %v", err)
					}
					break
				}

				if event := uint8(goMsg[0]); event != uffdPageFault() {
					log.Fatal("Received wrong event type")
				}

				address := binary.LittleEndian.Uint64(goMsg[16:])

				if err := s.servePageFault(fd, address); err != nil {
					log.Fatalf("Failed to serve page fault")
				}
			}
		}
	}
}

func (s *SnapshotState) servePageFault(fd int, address uint64) error {
	s.startAddressOnce.Do(
		func() {
			s.startAddress = address
		})

	offset := address - s.startAddress

	src := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(os.Getpagesize()) - 1))
	mode := uint64(0)

	return installRegion(fd, src, dst, mode, 1)
}
