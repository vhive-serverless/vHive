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

/*
#include "user_page_faults.h"
*/
import "C"

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/vhive-serverless/vhive/metrics"

	"unsafe"
)

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID string

	VMMStatePath, GuestMemPath, WorkingSetPath string

	InstanceSockAddr string
	BaseDir          string // base directory for the instance
	MetricsPath      string // path to csv file where the metrics should be stored
	IsLazyMode       bool
	GuestMemSize     int
	metricsModeOn    bool
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	firstPageFaultOnce  *sync.Once // to initialize the start virtual address and replay
	startAddress        uint64
	userFaultFD         *os.File
	guestRegionMappings []GuestRegionUffdMapping
	trace               *Trace
	epfd                int
	quitCh              chan int

	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	isRecordReady bool

	guestMem   []byte
	workingSet []byte

	// Stats
	totalPFServed  []float64
	uniquePFServed []float64
	reusedPFServed []float64
	latencyMetrics []*metrics.Metric

	replayedNum   int // only valid for lazy serving
	uniqueNum     int
	currentMetric *metrics.Metric
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
		s.latencyMetrics = make([]*metrics.Metric, 0)
	}

	return s
}

func (s *SnapshotState) setupStateOnActivate() {
	s.isActive = true
	s.isEverActivated = true
	s.firstPageFaultOnce = new(sync.Once)
	s.quitCh = make(chan int, 1)

	if s.metricsModeOn {
		s.uniqueNum = 0
		s.replayedNum = 0
		s.currentMetric = metrics.NewMetric()
	}
}

func (s *SnapshotState) getUFFD() error {
	mappings, userFaultFD, err := receiveUffdMappingsAndFDFromSocket(s.InstanceSockAddr)
	if err != nil {
		log.Error("Failed to receive the uffd and guest memory mappings")
		return err
	}

	s.guestRegionMappings = mappings
	s.userFaultFD = userFaultFD

	return nil
}

func (s *SnapshotState) processMetrics() {
	if s.metricsModeOn && s.isRecordReady {
		s.uniquePFServed = append(s.uniquePFServed, float64(s.uniqueNum))

		if s.IsLazyMode {
			s.totalPFServed = append(s.totalPFServed, float64(s.replayedNum))
			s.reusedPFServed = append(
				s.reusedPFServed,
				float64(s.replayedNum-s.uniqueNum),
			)
		}

		s.latencyMetrics = append(s.latencyMetrics, s.currentMetric)
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
	defer func() { _ = fd.Close() }()

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
func AlignedBlock(blockSize int) []byte {
	alignSize := os.Getpagesize() // must be multiple of the filesystem block size

	if blockSize == 0 {
		return nil
	}

	block := make([]byte, blockSize+alignSize)

	a := alignment(block, alignSize)
	offset := 0
	if a != 0 {
		offset = alignSize - a
	}
	block = block[offset : offset+blockSize]

	// Check
	if blockSize != 0 {
		a = alignment(block, alignSize)
		if a != 0 {
			log.Fatal("Failed to align block")
		}
	}
	return block
}

// fetchState Fetches the working set file (or the whole guest memory) and the VMM state file
func (s *SnapshotState) fetchState() error {
	if _, err := os.ReadFile(s.VMMStatePath); err != nil {
		log.Errorf("Failed to fetch VMM state: %v\n", err)
		return err
	}

	if !s.IsLazyMode {
		return nil
	}

	size := len(s.trace.trace) * os.Getpagesize()

	// O_DIRECT allows to fully leverage disk bandwidth by bypassing the OS page cache
	f, err := os.OpenFile(s.WorkingSetPath, os.O_RDONLY|syscall.O_DIRECT, 0600)
	if err != nil {
		log.Errorf("Failed to open the working set file for direct-io: %v\n", err)
		return err
	}

	s.workingSet = AlignedBlock(size) // direct io requires aligned buffer

	if n, err := f.Read(s.workingSet); n != size || err != nil {
		log.Errorf("Reading working set file failed: %v\n", err)
		return err
	}

	log.Debug("Fetched the entire working set")
	if err := f.Close(); err != nil {
		log.Errorf("Failed to close the working set file: %v\n", err)
		return err
	}

	return nil
}

func (s *SnapshotState) pollUserPageFaults(readyCh chan error) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var events [1]syscall.EpollEvent

	if err := s.registerEpoller(); err != nil {
		readyCh <- err
		return
	}

	logger.Debug("Starting polling loop")

	defer func() { _ = syscall.Close(s.epfd) }()

	readyCh <- nil

	for {
		select {
		case <-s.quitCh:
			logger.Debug("Handler received a signal to quit")
			return
		default:
			nevents, err := syscall.EpollWait(s.epfd, events[:], -1)
			if err != nil {
				if errors.Is(err, syscall.EINTR) {
					continue
				}
				if errors.Is(err, syscall.EBADF) {
					logger.Debug("UFFD epoller was closed")
					return
				}
				logger.WithError(err).Error("epoll_wait failed")
				return
			}

			if nevents < 1 {
				continue
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)

				stateFd := int(s.userFaultFD.Fd())

				if fd != stateFd && stateFd != -1 {
					logger.WithFields(log.Fields{
						"fd":      fd,
						"stateFd": stateFd,
					}).Error("Received event from unknown fd")
					return
				}

				goMsg := make([]byte, sizeOfUFFDMsg())

				nread, err := syscall.Read(fd, goMsg)
				if err != nil {
					if errors.Is(err, syscall.EINTR) || errors.Is(err, syscall.EAGAIN) {
						continue
					}
					if errors.Is(err, syscall.EBADF) {
						logger.Debug("UFFD fd was closed")
						return
					}
					logger.WithError(err).Error("Read uffd_msg failed")
					return
				}
				if nread != len(goMsg) {
					logger.WithFields(log.Fields{
						"read": nread,
						"want": len(goMsg),
					}).Error("Read incomplete uffd_msg")
					return
				}

				if event := uint8(goMsg[0]); event != uffdPageFault() {
					logger.WithField("event", event).Warn("Ignoring unsupported UFFD event")
					continue
				}

				address := binary.LittleEndian.Uint64(goMsg[16:])

				if err := s.servePageFault(fd, address); err != nil {
					logger.WithError(err).WithField("address", fmt.Sprintf("%#x", address)).Error("Failed to serve page fault")
					return
				}
			}
		}
	}
}

func (s *SnapshotState) registerEpoller() error {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var (
		err   error
		event syscall.EpollEvent
		fdInt int
	)

	fdInt = int(s.userFaultFD.Fd())

	event.Events = syscall.EPOLLIN
	event.Fd = int32(fdInt)

	s.epfd, err = syscall.EpollCreate1(0)
	if err != nil {
		logger.Errorf("Failed to create epoller %v", err)
		return err
	}

	if err := syscall.EpollCtl(
		s.epfd,
		syscall.EPOLL_CTL_ADD,
		fdInt,
		&event,
	); err != nil {
		_ = syscall.Close(s.epfd)
		logger.Errorf("Failed to subscribe VM %v", err)
		return err
	}

	return nil
}

func (s *SnapshotState) servePageFault(fd int, address uint64) error {
	var (
		tStart              time.Time
		workingSetInstalled bool
	)

	s.firstPageFaultOnce.Do(
		func() {
			s.startAddress = address

			if s.isRecordReady && !s.IsLazyMode {
				if s.metricsModeOn {
					tStart = time.Now()
				}
				s.installWorkingSetPages(fd)
				if s.metricsModeOn {
					s.currentMetric.MetricMap[installWSMetric] = metrics.ToUS(time.Since(tStart))
				}

				workingSetInstalled = true
			}
		})

	if workingSetInstalled {
		return nil
	}

	copyArgs, err := pageFaultCopyArgsForFault(s.guestRegionMappings, address)
	if err != nil {
		return err
	}

	src, err := guestMemPointer(s.guestMem, copyArgs.srcOffset, copyArgs.copyLen)
	if err != nil {
		return err
	}

	rec := Record{
		offset: copyArgs.srcOffset,
	}

	if !s.isRecordReady {
		s.trace.AppendRecord(rec)
	} else {
		log.Debug("Serving a page that is missing from the working set")
	}

	if s.metricsModeOn {
		if s.isRecordReady {
			if s.IsLazyMode {
				if !s.trace.containsRecord(rec) {
					s.uniqueNum++
				}
				s.replayedNum++
			} else {
				s.uniqueNum++
			}

		}

		tStart = time.Now()
	}

	err = installRegionBytes(fd, src, copyArgs.dstAddr, copyArgs.copyMode, copyArgs.copyLen)

	if s.metricsModeOn {
		s.currentMetric.MetricMap[serveUniqueMetric] += metrics.ToUS(time.Since(tStart))
	}

	return err
}

func (s *SnapshotState) installWorkingSetPages(fd int) {
	log.Debug("Installing the working set pages")

	// build a list of sorted regions
	keys := make([]uint64, 0)
	for k := range s.trace.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var (
		srcOffset uint64
	)

	for _, offset := range keys {
		regLength := s.trace.regions[offset]
		regAddress := s.startAddress + offset
		mode := uint64(C.const_UFFDIO_COPY_MODE_DONTWAKE)
		src := uint64(uintptr(unsafe.Pointer(&s.workingSet[srcOffset])))
		dst := regAddress

		if err := installRegion(fd, src, dst, mode, uint64(regLength)); err != nil {
			log.Fatalf("install_region: %v", err)
		}

		srcOffset += uint64(regLength) * 4096
	}

	wake(fd, s.startAddress, os.Getpagesize())
}

func installRegion(fd int, src, dst, mode, pageCount uint64) error {
	return installRegionBytes(fd, src, dst, mode, uint64(os.Getpagesize())*pageCount)
}

func installRegionBytes(fd int, src, dst, mode, length uint64) error {
	cUC := C.struct_uffdio_copy{
		mode: C.ulonglong(mode),
		copy: 0,
		src:  C.ulonglong(src),
		dst:  C.ulonglong(dst),
		len:  C.ulonglong(length),
	}

	err := ioctl(uintptr(fd), int(C.const_UFFDIO_COPY), unsafe.Pointer(&cUC))
	if err != nil {
		if errors.Is(err, unix.EEXIST) {
			return nil
		}
		return err
	}

	return nil
}

func guestMemPointer(guestMem []byte, offset, length uint64) (uint64, error) {
	if length == 0 {
		return 0, errors.New("guest memory copy length must be non-zero")
	}
	if offset >= uint64(len(guestMem)) || length > uint64(len(guestMem))-offset {
		return 0, fmt.Errorf("guest memory copy is outside mapped file: offset=%#x len=%#x size=%#x", offset, length, len(guestMem))
	}

	return uint64(uintptr(unsafe.Pointer(&guestMem[int(offset)]))), nil
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
		return os.NewSyscallError("ioctl", errno)
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

//nolint:unused
func registerForUpf(startAddress []byte, len uint64) int {
	return int(C.register_for_upf(unsafe.Pointer(&startAddress[0]), C.ulong(len)))
}

func sizeOfUFFDMsg() int {
	return C.sizeof_struct_uffd_msg
}

func uffdPageFault() uint8 {
	return uint8(C.const_UFFD_EVENT_PAGEFAULT)
}
