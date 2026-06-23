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
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/vhive-serverless/vhive/metrics"

	"unsafe"
)

const (
	uffdSocketPayloadSize   = 64 * 1024
	uffdSocketFDLimit       = 2
	uffdSocketReadTimeout   = time.Second
	uffdSocketAcceptTimeout = 30 * time.Second
)

var (
	errInvalidGuestRegionPageSize = errors.New("guest region page size must be non-zero")
	errGuestRegionNotFound        = errors.New("fault address is outside guest memory mappings")
	errUnexpectedUffdFDCount      = errors.New("expected exactly one uffd fd")
	errNoGuestRegionMappings      = errors.New("no guest region mappings received")
	errEmptyUffdSocketPath        = errors.New("empty uffd socket path")
)

// GuestRegionUffdMapping describes Firecracker's UFFD guest memory mapping.
type GuestRegionUffdMapping struct {
	BaseHostVirtAddr uint64 `json:"base_host_virt_addr"`
	Size             uint64 `json:"size"`
	Offset           uint64 `json:"offset"`
	PageSize         uint64 `json:"page_size"`
}

type pageFaultCopyArgs struct {
	srcOffset uint64
	dstAddr   uint64
	copyLen   uint64
	copyMode  uint64
}

func pageAlignFaultAddress(faultAddr uint64, region GuestRegionUffdMapping) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}

	return faultAddr - faultAddr%region.PageSize, nil
}

func guestMemoryOffsetForFaultPage(region GuestRegionUffdMapping, faultPageAddr uint64) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}
	if !regionContainsFaultPage(region, faultPageAddr) {
		return 0, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultPageAddr)
	}

	regionOffset := faultPageAddr - region.BaseHostVirtAddr
	if region.Offset > math.MaxUint64-regionOffset {
		return 0, fmt.Errorf("guest memory offset overflow for fault address %#x", faultPageAddr)
	}

	return region.Offset + regionOffset, nil
}

func pageFaultCopyArgsForFault(regions []GuestRegionUffdMapping, faultAddr uint64) (pageFaultCopyArgs, error) {
	for _, region := range regions {
		faultPageAddr, err := pageAlignFaultAddress(faultAddr, region)
		if err != nil {
			return pageFaultCopyArgs{}, err
		}
		if !regionContainsFaultPage(region, faultPageAddr) {
			continue
		}

		srcOffset, err := guestMemoryOffsetForFaultPage(region, faultPageAddr)
		if err != nil {
			return pageFaultCopyArgs{}, err
		}

		return pageFaultCopyArgs{
			srcOffset: srcOffset,
			dstAddr:   faultPageAddr,
			copyLen:   region.PageSize,
			copyMode:  0,
		}, nil
	}

	return pageFaultCopyArgs{}, fmt.Errorf("%w: %#x", errGuestRegionNotFound, faultAddr)
}

func regionContainsFaultPage(region GuestRegionUffdMapping, faultPageAddr uint64) bool {
	if region.Size == 0 || faultPageAddr < region.BaseHostVirtAddr {
		return false
	}

	return faultPageAddr-region.BaseHostVirtAddr < region.Size
}

// SnapshotStateCfg Config to initialize SnapshotState
type SnapshotStateCfg struct {
	VMID string

	VMMStatePath, GuestMemPath string

	InstanceSockAddr string
	BaseDir          string // base directory for the instance
	MetricsPath      string // path to csv file where the metrics should be stored
	GuestMemSize     int
	metricsModeOn    bool
}

// SnapshotState Stores the state of the snapshot
// of the VM.
type SnapshotState struct {
	SnapshotStateCfg
	userFaultFD         *os.File
	guestRegionMappings []GuestRegionUffdMapping
	epfd                int
	quitCh              chan int

	// to indicate whether the instance has even been activated. this is to
	// get around cases where offload is called for the first time
	isEverActivated bool
	// for sanity checking on deactivate/activate
	isActive bool

	guestMem []byte

	// Stats
	uniquePFServed []float64
	latencyMetrics []*metrics.Metric

	uniqueNum     int
	currentMetric *metrics.Metric
}

// NewSnapshotState Initializes a snapshot state
func NewSnapshotState(cfg SnapshotStateCfg) *SnapshotState {
	s := new(SnapshotState)
	s.SnapshotStateCfg = cfg

	if s.metricsModeOn {
		s.uniquePFServed = make([]float64, 0)
		s.latencyMetrics = make([]*metrics.Metric, 0)
	}

	return s
}

func (s *SnapshotState) setupStateOnActivate() {
	s.isActive = true
	s.isEverActivated = true
	s.quitCh = make(chan int, 1)

	if s.metricsModeOn {
		s.uniqueNum = 0
		s.currentMetric = metrics.NewMetric()
	}
}

func (s *SnapshotState) getUFFD(socketReadyCh chan<- error) error {
	mappings, userFaultFD, err := receiveUffdMappingsAndFDFromSocket(s.InstanceSockAddr, socketReadyCh)
	if err != nil {
		log.Error("Failed to receive the uffd and guest memory mappings")
		return err
	}

	s.guestRegionMappings = mappings
	s.userFaultFD = userFaultFD

	return nil
}

func receiveUffdMappingsAndFDFromSocket(socketPath string, socketReadyCh chan<- error) ([]GuestRegionUffdMapping, *os.File, error) {
	if socketPath == "" {
		notifySocketReady(socketReadyCh, errEmptyUffdSocketPath)
		return nil, nil, errEmptyUffdSocketPath
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		notifySocketReady(socketReadyCh, err)
		return nil, nil, err
	}
	if err := removeStaleUffdSocket(socketPath); err != nil {
		notifySocketReady(socketReadyCh, err)
		return nil, nil, err
	}

	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		notifySocketReady(socketReadyCh, err)
		return nil, nil, err
	}
	defer func() { _ = listener.Close() }()
	defer func() { _ = os.Remove(socketPath) }()

	if err := listener.SetDeadline(time.Now().Add(uffdSocketAcceptTimeout)); err != nil {
		notifySocketReady(socketReadyCh, err)
		return nil, nil, err
	}

	notifySocketReady(socketReadyCh, nil)

	conn, err := listener.AcceptUnix()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = conn.Close() }()

	return receiveUffdMappingsAndFD(conn)
}

func notifySocketReady(socketReadyCh chan<- error, err error) {
	if socketReadyCh == nil {
		return
	}
	socketReadyCh <- err
}

func receiveUffdMappingsAndFD(conn *net.UnixConn) ([]GuestRegionUffdMapping, *os.File, error) {
	if err := conn.SetReadDeadline(time.Now().Add(uffdSocketReadTimeout)); err != nil {
		return nil, nil, err
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	body := make([]byte, uffdSocketPayloadSize)
	oob := make([]byte, unix.CmsgSpace(uffdSocketFDLimit*4))

	n, oobn, flags, _, err := conn.ReadMsgUnix(body, oob)
	if err != nil {
		return nil, nil, err
	}
	if flags&unix.MSG_TRUNC != 0 {
		return nil, nil, errors.New("uffd mappings payload was truncated")
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return nil, nil, errors.New("uffd fd control message was truncated")
	}

	fds, err := parseUnixRights(oob[:oobn])
	if err != nil {
		return nil, nil, err
	}
	if len(fds) != 1 {
		closeFDs(fds)
		return nil, nil, fmt.Errorf("%w: got %d", errUnexpectedUffdFDCount, len(fds))
	}

	uffdFile := os.NewFile(uintptr(fds[0]), "userfaultfd")
	if uffdFile == nil {
		return nil, nil, errors.New("failed to create file for uffd fd")
	}

	var mappings []GuestRegionUffdMapping
	if err := json.Unmarshal(body[:n], &mappings); err != nil {
		_ = uffdFile.Close()
		return nil, nil, fmt.Errorf("cannot deserialize memory mappings: %w", err)
	}
	if len(mappings) == 0 {
		_ = uffdFile.Close()
		return nil, nil, errNoGuestRegionMappings
	}

	return mappings, uffdFile, nil
}

func parseUnixRights(oob []byte) ([]int, error) {
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}

	var fds []int
	for i := range scms {
		rights, err := unix.ParseUnixRights(&scms[i])
		if err != nil {
			closeFDs(fds)
			return nil, err
		}
		fds = append(fds, rights...)
	}

	return fds, nil
}

func closeFDs(fds []int) {
	for _, receivedFD := range fds {
		_ = unix.Close(receivedFD)
	}
}

func removeStaleUffdSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("refusing to remove non-socket uffd path %q", socketPath)
		}
		return os.Remove(socketPath)
	}
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *SnapshotState) processMetrics() {
	if s.metricsModeOn && s.currentMetric != nil {
		s.uniquePFServed = append(s.uniquePFServed, float64(s.uniqueNum))
		s.latencyMetrics = append(s.latencyMetrics, s.currentMetric)
	}
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

// fetchState verifies the VMM state file before snapshot activation.
func (s *SnapshotState) fetchState() error {
	if _, err := os.ReadFile(s.VMMStatePath); err != nil {
		log.Errorf("Failed to fetch VMM state: %v\n", err)
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
	var tStart time.Time

	copyArgs, err := pageFaultCopyArgsForFault(s.guestRegionMappings, address)
	if err != nil {
		return err
	}

	src, err := guestMemPointer(s.guestMem, copyArgs.srcOffset, copyArgs.copyLen)
	if err != nil {
		return err
	}

	if s.metricsModeOn {
		s.uniqueNum++
		tStart = time.Now()
	}

	err = installRegionBytes(fd, src, copyArgs.dstAddr, copyArgs.copyMode, copyArgs.copyLen)

	if s.metricsModeOn {
		s.currentMetric.MetricMap[serveUniqueMetric] += metrics.ToUS(time.Since(tStart))
	}

	return err
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

func sizeOfUFFDMsg() int {
	return C.sizeof_struct_uffd_msg
}

func uffdPageFault() uint8 {
	return uint8(C.const_UFFD_EVENT_PAGEFAULT)
}
