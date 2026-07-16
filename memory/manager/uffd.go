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
	"sort"
	"syscall"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"

	"github.com/vhive-serverless/vhive/metrics"
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

// pageFaultCopyArgs describes one UFFDIO_COPY operation.
type pageFaultCopyArgs struct {
	srcOffset uint64
	dstAddr   uint64
	copyLen   uint64
	copyMode  uint64
}

// pageAlignFaultAddress rounds a fault address down to its guest page boundary.
func pageAlignFaultAddress(faultAddr uint64, region GuestRegionUffdMapping) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}

	return faultAddr - faultAddr%region.PageSize, nil
}

// guestMemoryOffsetForFaultPage translates a guest fault page to its memory-file offset.
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

// guestAddressForMemoryOffset translates a memory-file offset to a guest address.
func guestAddressForMemoryOffset(region GuestRegionUffdMapping, offset uint64) (uint64, error) {
	if region.PageSize == 0 {
		return 0, errInvalidGuestRegionPageSize
	}
	if !regionContainsGuestMemoryOffset(region, offset) {
		return 0, fmt.Errorf("%w: %#x", errGuestRegionNotFound, offset)
	}

	regionOffset := offset - region.Offset
	if region.BaseHostVirtAddr > math.MaxUint64-regionOffset {
		return 0, fmt.Errorf("guest address overflow for memory offset %#x", offset)
	}

	return region.BaseHostVirtAddr + regionOffset, nil
}

// pageFaultCopyArgsForFault builds copy arguments for a reported guest page fault.
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

// pageFaultCopyArgsForGuestOffset builds copy arguments for working-set replay.
func pageFaultCopyArgsForGuestOffset(regions []GuestRegionUffdMapping, offset uint64, mode uint64) (pageFaultCopyArgs, error) {
	for _, region := range regions {
		if !regionContainsGuestMemoryOffset(region, offset) {
			continue
		}
		dstAddr, err := guestAddressForMemoryOffset(region, offset)
		if err != nil {
			return pageFaultCopyArgs{}, err
		}

		return pageFaultCopyArgs{
			srcOffset: offset,
			dstAddr:   dstAddr,
			copyLen:   region.PageSize,
			copyMode:  mode,
		}, nil
	}

	return pageFaultCopyArgs{}, fmt.Errorf("%w: %#x", errGuestRegionNotFound, offset)
}

// regionContainsFaultPage reports whether a guest page belongs to a mapping.
func regionContainsFaultPage(region GuestRegionUffdMapping, faultPageAddr uint64) bool {
	if region.Size == 0 || faultPageAddr < region.BaseHostVirtAddr {
		return false
	}

	return faultPageAddr-region.BaseHostVirtAddr < region.Size
}

// regionContainsGuestMemoryOffset reports whether a memory-file offset belongs to a mapping.
func regionContainsGuestMemoryOffset(region GuestRegionUffdMapping, offset uint64) bool {
	if region.Size == 0 || offset < region.Offset {
		return false
	}

	return offset-region.Offset < region.Size
}

// guestMappingPageSize returns the common page size used by all mappings.
func guestMappingPageSize(regions []GuestRegionUffdMapping) (uint64, error) {
	var pageSize uint64
	for _, region := range regions {
		if region.PageSize == 0 {
			return 0, errInvalidGuestRegionPageSize
		}
		if pageSize == 0 {
			pageSize = region.PageSize
			continue
		}
		if pageSize != region.PageSize {
			return 0, errors.New("mixed guest region page sizes are not supported for working-set replay")
		}
	}
	if pageSize == 0 {
		return 0, errNoGuestRegionMappings
	}
	return pageSize, nil
}

// getUFFD receives and stores Firecracker's mappings and userfaultfd.
func (s *SnapshotState) getUFFD(socketReadyCh chan<- struct{}) error {
	mappings, userFaultFD, err := receiveUffdMappingsAndFDFromSocket(s.InstanceSockAddr, socketReadyCh)
	if err != nil {
		log.Error("Failed to receive the uffd and guest memory mappings")
		return err
	}

	s.guestRegionMappings = mappings
	s.userFaultFD = userFaultFD

	return nil
}

// receiveUffdMappingsAndFDFromSocket accepts one Firecracker UFFD connection.
func receiveUffdMappingsAndFDFromSocket(socketPath string, socketReadyCh chan<- struct{}) ([]GuestRegionUffdMapping, *os.File, error) {
	if socketPath == "" {
		return nil, nil, errEmptyUffdSocketPath
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0755); err != nil {
		return nil, nil, err
	}
	if err := removeStaleUffdSocket(socketPath); err != nil {
		return nil, nil, err
	}

	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = listener.Close() }()
	defer func() { _ = os.Remove(socketPath) }()

	if err := listener.SetDeadline(time.Now().Add(uffdSocketAcceptTimeout)); err != nil {
		return nil, nil, err
	}

	notifySocketReady(socketReadyCh)

	conn, err := listener.AcceptUnix()
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = conn.Close() }()

	return receiveUffdMappingsAndFD(conn)
}

// notifySocketReady signals that Firecracker can connect to the UFFD socket.
func notifySocketReady(socketReadyCh chan<- struct{}) {
	if socketReadyCh == nil {
		return
	}
	socketReadyCh <- struct{}{}
}

// receiveUffdMappingsAndFD reads mappings and exactly one fd from a Unix connection.
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

// parseUnixRights extracts file descriptors from Unix control messages.
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

// closeFDs closes file descriptors that cannot be returned to the caller.
func closeFDs(fds []int) {
	for _, receivedFD := range fds {
		_ = unix.Close(receivedFD)
	}
}

// removeStaleUffdSocket removes an existing socket without deleting other file types.
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

// pollUserPageFaults reads UFFD events until the handler is stopped or fails.
func (s *SnapshotState) pollUserPageFaults(readyCh chan error) {
	logger := log.WithFields(log.Fields{"vmID": s.VMID})

	var events [2]syscall.EpollEvent

	defer close(s.pollDoneCh)

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

			select {
			case <-s.quitCh:
				logger.Debug("Handler received a signal to quit")
				return
			default:
			}

			for i := 0; i < nevents; i++ {
				event := events[i]

				fd := int(event.Fd)
				if fd == s.wakeFD {
					logger.Debug("Handler received wakeup event")
					return
				}

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

// registerEpoller subscribes the UFFD and wake fd to a new epoll instance.
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

	s.wakeFD, err = unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		_ = syscall.Close(s.epfd)
		logger.Errorf("Failed to create UFFD wake fd %v", err)
		return err
	}

	event.Fd = int32(s.wakeFD)
	if err := syscall.EpollCtl(
		s.epfd,
		syscall.EPOLL_CTL_ADD,
		s.wakeFD,
		&event,
	); err != nil {
		_ = unix.Close(s.wakeFD)
		_ = syscall.Close(s.epfd)
		logger.Errorf("Failed to subscribe UFFD wake fd %v", err)
		return err
	}

	return nil
}

// stopPolling wakes the poller and asks it to exit.
func (s *SnapshotState) stopPolling() {
	select {
	case s.quitCh <- 0:
	default:
	}

	if s.wakeFD < 0 {
		return
	}

	var wake [8]byte
	binary.LittleEndian.PutUint64(wake[:], 1)
	if _, err := unix.Write(s.wakeFD, wake[:]); err != nil &&
		!errors.Is(err, syscall.EBADF) &&
		!errors.Is(err, syscall.EAGAIN) {
		log.WithError(err).Debug("Failed to wake UFFD poller")
	}
}

// waitForPoller waits until the UFFD polling goroutine exits.
func (s *SnapshotState) waitForPoller() {
	if s.pollDoneCh != nil {
		<-s.pollDoneCh
	}
}

// closeWakeFD releases the eventfd used to stop the poller.
func (s *SnapshotState) closeWakeFD() {
	if s.wakeFD >= 0 {
		_ = unix.Close(s.wakeFD)
		s.wakeFD = -1
	}
}

// servePageFault copies the requested guest page or installs the recorded working set.
func (s *SnapshotState) servePageFault(fd int, address uint64) error {
	var (
		tStart              time.Time
		workingSetInstalled bool
	)

	copyArgs, err := pageFaultCopyArgsForFault(s.guestRegionMappings, address)
	if err != nil {
		return err
	}

	rec := Record{offset: copyArgs.srcOffset}
	if s.firstPageFaultOnce != nil {
		s.firstPageFaultOnce.Do(func() {
			if !s.isRecordReady || s.IsLazyMode {
				return
			}

			if s.metricsModeOn {
				tStart = time.Now()
			}
			err = s.installWorkingSetPages(fd, copyArgs.dstAddr, copyArgs.copyLen)
			if err != nil {
				return
			}
			if s.metricsModeOn {
				s.currentMetric.MetricMap[installWSMetric] = metrics.ToUS(time.Since(tStart))
			}
			workingSetInstalled = true
		})
		if err != nil {
			return err
		}
	}

	if workingSetInstalled && s.trace.containsRecord(rec) {
		return nil
	}

	src, err := guestMemPointer(s.guestMem, copyArgs.srcOffset, copyArgs.copyLen)
	if err != nil {
		return err
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

// installWorkingSetPages copies recorded pages before waking the first fault.
func (s *SnapshotState) installWorkingSetPages(fd int, faultPageAddr, pageSize uint64) error {
	if len(s.workingSet) == 0 || len(s.trace.regions) == 0 {
		return nil
	}
	if s.trace.pageSize != 0 {
		pageSize = s.trace.pageSize
	}

	keys := make([]uint64, 0, len(s.trace.regions))
	for offset := range s.trace.regions {
		keys = append(keys, offset)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	var workingSetOffset uint64
	for _, regionOffset := range keys {
		regLength := s.trace.regions[regionOffset]
		for i := 0; i < regLength; i++ {
			pageOffset := regionOffset + uint64(i)*pageSize
			copyArgs, err := pageFaultCopyArgsForGuestOffset(
				s.guestRegionMappings,
				pageOffset,
				uint64(C.const_UFFDIO_COPY_MODE_DONTWAKE),
			)
			if err != nil {
				return err
			}

			src, err := guestMemPointer(s.workingSet, workingSetOffset, copyArgs.copyLen)
			if err != nil {
				return err
			}
			if err := installRegionBytes(fd, src, copyArgs.dstAddr, copyArgs.copyMode, copyArgs.copyLen); err != nil {
				return err
			}
			workingSetOffset += copyArgs.copyLen
		}
	}

	return wake(fd, faultPageAddr, pageSize)
}

// installRegionBytes resolves missing pages with UFFDIO_COPY.
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

// guestMemPointer returns a checked pointer into a mapped memory buffer.
func guestMemPointer(guestMem []byte, offset, length uint64) (uint64, error) {
	if length == 0 {
		return 0, errors.New("guest memory copy length must be non-zero")
	}
	if offset >= uint64(len(guestMem)) || length > uint64(len(guestMem))-offset {
		return 0, fmt.Errorf("guest memory copy is outside mapped file: offset=%#x len=%#x size=%#x", offset, length, len(guestMem))
	}

	return uint64(uintptr(unsafe.Pointer(&guestMem[int(offset)]))), nil
}

// ioctl invokes an ioctl and converts errno to a Go error.
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

// wake resumes faults waiting on a previously copied range.
func wake(fd int, startAddress, length uint64) error {
	cUR := C.struct_uffdio_range{
		start: C.ulonglong(startAddress),
		len:   C.ulonglong(length),
	}

	return ioctl(uintptr(fd), int(C.const_UFFDIO_WAKE), unsafe.Pointer(&cUR))
}

// sizeOfUFFDMsg returns the platform uffd_msg size.
func sizeOfUFFDMsg() int {
	return C.sizeof_struct_uffd_msg
}

// uffdPageFault returns the platform page-fault event identifier.
func uffdPageFault() uint8 {
	return uint8(C.const_UFFD_EVENT_PAGEFAULT)
}
