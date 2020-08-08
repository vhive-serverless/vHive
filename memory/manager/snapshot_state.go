package manager

/*
#include "user_page_faults.h"
*/
import "C"

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"sort"
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
	VMID string

	VMMStatePath, GuestMemPath, WorkingSetPath string

	InstanceSockAddr string
	BaseDir          string // base directory for the instance
	MetricsPath      string // path to csv file where the metrics should be stored
	IsRecordMode     bool
	GuestMemSize     int
	metricsModeOn    bool
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	FirstPageFaultOnce *sync.Once // to initialize the start virtual address and replay
	startAddress       uint64
	userFaultFD        *os.File
	trace              *Trace
	epfd               int
	quitCh             chan int

	// install the whole working set in the guest memory
	isReplayWorkingSet bool
	// prefetch the VMM state to the host memory
	isPrefetchVMMState bool
	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isRecordDone bool

	isWSCopy bool

	servedNum int
	uniqueNum int

	guestMem   []byte
	workingSet []byte

	// Stats
	totalPFServed  []float64
	uniquePFServed []float64
	reusedPFServed []float64
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.SnapshotStateCfg = cfg

	s.trace = initTrace(s.getTraceFile())
	if s.metricsModeOn {
		s.totalPFServed = make([]float64, 0)
		s.uniquePFServed = make([]float64, 0)
		s.reusedPFServed = make([]float64, 0)
	}

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

// alignment returns alignment of the block in memory
// with reference to alignSize
//
// Can't check alignment of a zero sized block as &block[0] is invalid
func alignment(block []byte, alignSize int) int {
	return int(uintptr(unsafe.Pointer(&block[0])) & uintptr(alignSize-1))
}

// AlignedBlock returns []byte of size BlockSize aligned to a multiple
// of alignSize in memory (must be power of two)
func AlignedBlock(BlockSize int) []byte {
	alignSize := os.Getpagesize() // must be multiple of the filesystem block size

	block := make([]byte, BlockSize+alignSize)
	if alignSize == 0 {
		return block
	}
	a := alignment(block, alignSize)
	offset := 0
	if a != 0 {
		offset = alignSize - a
	}
	block = block[offset : offset+BlockSize]
	// Can't check alignment of a zero sized block
	if BlockSize != 0 {
		a = alignment(block, alignSize)
		if a != 0 {
			log.Fatal("Failed to align block")
		}
	}
	return block
}

// fetchState Fetches the working set file (or the whole guest memory) and/or the VMM state file
func (s *SnapshotState) fetchState() {
	// if s.isPrefetchVMMState {
	if _, err := ioutil.ReadFile(s.VMMStatePath); err != nil {
		log.Errorf("Failed to fetch VMM state: %v\n", err)
	}
	//}

	size := len(s.trace.trace) * os.Getpagesize()

	// O_DIRECT allows to fully leverage disk bandwidth by bypassing the OS page cache
	f, err := os.OpenFile(s.WorkingSetPath, os.O_RDONLY|syscall.O_DIRECT, 0600)
	if err != nil {
		log.Errorf("Failed to open the working set file for direct-io: %v\n", err)
	}

	s.workingSet = AlignedBlock(size) // direct io requires aligned buffer

	if n, err := f.Read(s.workingSet); n != size || err != nil {
		log.Errorf("Reading working set file failed: %v\n", err)
	}

	//trace.wsFetched = true
	log.Debug("Fetched the entire working set")
	if err := f.Close(); err != nil {
		log.Errorf("Failed to close the working set file: %v\n", err)
	}

	// return nil FIXME: add error checks in this function
}

func (s *SnapshotState) pollUserPageFaults(readyCh chan int) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var (
		events [1]syscall.EpollEvent
	)

	logger.Debug("Starting polling loop")

	defer syscall.Close(s.epfd)

	readyCh <- 0

	// if s.isReplayWorkingSet {
	if s.isRecordDone {
		s.fetchState()
	}
	// }

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
	var workingSetInstalled bool

	s.FirstPageFaultOnce.Do(
		func() {
			s.startAddress = address

			// if s.isReplayWorkingSet
			if s.isRecordDone {
				s.installWorkingSetPages(fd)
				workingSetInstalled = true
			}
			// }
		})

	if workingSetInstalled {
		return nil
	}

	offset := address - s.startAddress

	src := uint64(uintptr(unsafe.Pointer(&s.guestMem[offset])))
	dst := uint64(int64(address) & ^(int64(os.Getpagesize()) - 1))
	mode := uint64(0)

	rec := Record{
		offset:    offset,
		servedNum: s.servedNum,
	}

	if !s.isRecordDone {
		s.trace.AppendRecord(rec)
	} else {
		log.Info("Serving a page that is missing from the working set")
	}

	if s.metricsModeOn {
		if s.isRecordDone {
			if !s.trace.containsRecord(rec) {
				s.uniqueNum++
			}
		}

		s.servedNum++
	}

	return installRegion(fd, src, dst, mode, 1)
}

func (s *SnapshotState) installWorkingSetPages(fd int) {
	log.Info("Installing the working set pages")
	// TODO: parallel (goroutines) vs serial, by region vs by page

	// build a list of sorted regions (probably, it's better to make trace.regions an array instead of a map FIXME)
	keys := make([]uint64, 0)
	for k := range s.trace.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var (
		srcOffset uint64
		wg        sync.WaitGroup
	)

	for _, offset := range keys {
		regLength := s.trace.regions[offset]
		regAddress := s.startAddress + offset
		mode := uint64(C.const_UFFDIO_COPY_MODE_DONTWAKE)
		src := uint64(uintptr(unsafe.Pointer(&s.workingSet[srcOffset])))
		dst := regAddress

		wg.Add(1)

		go func(fd int, src, dst, len uint64) {
			defer wg.Done()

			installRegion(fd, src, dst, mode, len)
		}(fd, src, dst, uint64(regLength))

		srcOffset += uint64(regLength) * 4096
	}

	wg.Wait()

	wake(fd, s.startAddress, s.GuestMemSize)
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

func wake(fd int, startAddress uint64, len int) {
	cUR := C.struct_uffdio_range{
		start: C.ulonglong(startAddress),
		len:   C.ulonglong(len),
	}

	err := ioctl(uintptr(fd), int(C.const_UFFDIO_WAKE), unsafe.Pointer(&cUR))
	if err != nil {
		log.Fatalf("ioctl failed: %v", err)
	}
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
