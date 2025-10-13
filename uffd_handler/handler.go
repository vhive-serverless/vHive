package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	log "github.com/sirupsen/logrus"

	"golang.org/x/sys/unix"
)

// UFFD constants and structures not defined in golang.org/x/sys/unix
// These values come from the Linux kernel headers: /usr/include/linux/userfaultfd.h
//
// Event types (defined directly in the kernel header):
//
//	#define UFFD_EVENT_PAGEFAULT 0x12
//	#define UFFD_EVENT_REMOVE    0x15
//
// IOCTL commands (computed using the _IOWR macro):
//
//	UFFDIO_COPY = _IOWR(UFFDIO, _UFFDIO_COPY, struct uffdio_copy)
//	where UFFDIO = 0xAA, _UFFDIO_COPY = 0x03, sizeof(struct uffdio_copy) = 40
//	Result: 0xc028aa03
//
//	UFFDIO_ZEROPAGE = _IOWR(UFFDIO, _UFFDIO_ZEROPAGE, struct uffdio_zeropage)
//	where UFFDIO = 0xAA, _UFFDIO_ZEROPAGE = 0x04, sizeof(struct uffdio_zeropage) = 32
//	Result: 0xc020aa04
const (
	UFFD_EVENT_PAGEFAULT = 0x12
	UFFD_EVENT_REMOVE    = 0x15
	UFFDIO_COPY          = 0xc028aa03
	UFFDIO_ZEROPAGE      = 0xc020aa04
)

// UffdMsg represents the userfaultfd message structure
// For pagefault events, this is exactly 32 bytes: 8-byte header + 24-byte payload
type UffdMsg struct {
	Event uint8
	_     [7]byte  // Reserved fields
	Arg   [24]byte // Raw payload - size for pagefault event
}

// Pagefault returns the pagefault event data by interpreting the raw Arg field.
func (m *UffdMsg) Pagefault() UffdMsgPagefault {
	// The pagefault struct is overlaid on the Arg field.
	// We use unsafe.Pointer to cast the Arg array to a pointer to the struct.
	return *(*UffdMsgPagefault)(unsafe.Pointer(&m.Arg[0]))
}

// Remove returns the remove event data.
func (m *UffdMsg) Remove() UffdMsgRemove {
	return *(*UffdMsgRemove)(unsafe.Pointer(&m.Arg[0]))
}

// UffdMsgPagefault represents a pagefault event.
// The layout must match the kernel's `struct uffdio_pagefault`.
type UffdMsgPagefault struct {
	Flags   uint64
	Address uint64
	Feat    [4]byte // This is the `feat` union in the kernel struct
}

// UffdMsgRemove represents a remove event.
type UffdMsgRemove struct {
	Start uint64
	End   uint64
}

// UffdIoCopy represents the ioctl copy structure
type UffdIoCopy struct {
	Dst  uint64
	Src  uint64
	Len  uint64
	Mode uint64
	Copy int64
}

// UffdIoZeropage represents the ioctl zeropage structure
type UffdIoZeropage struct {
	Range    UffdIoRange
	Mode     uint64
	Zeropage int64
}

// UffdIoRange represents a range of addresses
type UffdIoRange struct {
	Start uint64
	Len   uint64
}

// PageFaultTrace represents a page fault event for tracing
type PageFaultTrace struct {
	Address uint64 `json:"address"`
}

// PageFaultTracer handles tracing page fault events to a file
type PageFaultTracer struct {
	file *os.File
}

// NewPageFaultTracer creates a new page fault tracer
func NewPageFaultTracer(filePath string) (*PageFaultTracer, error) {
	if filePath == "" {
		return nil, nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace file: %w", err)
	}

	return &PageFaultTracer{file: file}, nil
}

// TracePageFault logs a page fault event
func (t *PageFaultTracer) TracePageFault(address, pfn uint64, eventType string, isRemoved, regionFound bool) {
	if t == nil || t.file == nil {
		return
	}

	trace := PageFaultTrace{
		Address: address,
	}

	data, err := json.Marshal(trace)
	if err != nil {
		log.Errorf("Failed to marshal trace data: %v", err)
		return
	}

	if _, err := t.file.Write(append(data, '\n')); err != nil {
		log.Errorf("Failed to write trace data: %v", err)
	}
}

// Close closes the trace file
func (t *PageFaultTracer) Close() error {
	if t == nil || t.file == nil {
		return nil
	}
	return t.file.Close()
}

// GuestRegionUffdMapping describes the mapping between Firecracker base virtual address
// and offset in the buffer or file backend for a guest memory region.
type GuestRegionUffdMapping struct {
	BaseHostVirtAddr uint64 `json:"base_host_virt_addr"`
	Size             uint64 `json:"size"`
	Offset           uint64 `json:"offset"`
	PageSize         uint64 `json:"page_size"`
}

// Contains checks if a fault page address is within this region
func (r *GuestRegionUffdMapping) Contains(faultPageAddr uint64) bool {
	return faultPageAddr >= r.BaseHostVirtAddr &&
		faultPageAddr < r.BaseHostVirtAddr+r.Size
}

// UffdHandler handles userfaultfd events
type UffdHandler struct {
	memRegions    []GuestRegionUffdMapping
	pageSize      uint64
	backingBuffer uintptr
	uffd          int
	removedPages  map[uint64]bool
	tracer        *PageFaultTracer
}

// NewUffdHandler creates a new UFFD handler from a Unix socket stream
func NewUffdHandler(conn *net.UnixConn, backingBuffer uintptr, size uint64, tracer *PageFaultTracer) (*UffdHandler, error) {
	body, uffdFd, err := getMappingsAndFile(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to get mappings and file: %w", err)
	}

	var mappings []GuestRegionUffdMapping
	if err := json.Unmarshal([]byte(body), &mappings); err != nil {
		return nil, fmt.Errorf("cannot deserialize memory mappings: %w, body: %s", err, body)
	}

	if len(mappings) == 0 {
		return nil, fmt.Errorf("no mappings received")
	}

	// Calculate total memory size
	var memSize uint64
	for _, r := range mappings {
		memSize += r.Size
	}

	if memSize != size {
		return nil, fmt.Errorf("memory size mismatch: expected %d, got %d", size, memSize)
	}

	pageSize := mappings[0].PageSize
	if pageSize == 0 || (pageSize&(pageSize-1)) != 0 {
		return nil, fmt.Errorf("invalid page size: %d", pageSize)
	}

	return &UffdHandler{
		memRegions:    mappings,
		pageSize:      pageSize,
		backingBuffer: backingBuffer,
		uffd:          uffdFd,
		removedPages:  make(map[uint64]bool),
		tracer:        tracer,
	}, nil
}

// ReadEvent reads an event from the userfaultfd
func (h *UffdHandler) ReadEvent() (*UffdMsg, error) {
	var msg UffdMsg
	msgSize := unsafe.Sizeof(msg)

	buf := (*[unsafe.Sizeof(UffdMsg{})]byte)(unsafe.Pointer(&msg))[:]
	n, err := unix.Read(h.uffd, buf)
	if err != nil {
		if err == unix.EAGAIN {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read uffd event: %w", err)
	}

	// The kernel should always send a full message.
	if n != int(msgSize) {
		return nil, fmt.Errorf("incomplete uffd message: got %d bytes, expected %d", n, msgSize)
	}

	return &msg, nil
}

// MarkRangeRemoved marks a range of pages as removed (for balloon device support)
func (h *UffdHandler) MarkRangeRemoved(start, end uint64) {
	pfnStart := start / h.pageSize
	pfnEnd := end / h.pageSize

	for pfn := pfnStart; pfn < pfnEnd; pfn++ {
		h.removedPages[pfn] = true
		// Trace the remove event
		pageAddr := pfn * h.pageSize
		h.tracer.TracePageFault(pageAddr, pfn, "remove", true, false)
	}
}

// ServePF serves a page fault at the given address
func (h *UffdHandler) ServePF(addr uintptr, length uint64) bool {
	// Find the start of the page that the current faulting address belongs to
	dst := uintptr(uint64(addr) & ^(h.pageSize - 1))
	faultPageAddr := uint64(dst)
	faultPfn := faultPageAddr / h.pageSize

	// log.Printf("Handling page fault at addr: %#x (pfn: %d)", addr, faultPfn)

	// If this page was removed (by balloon device), zero it out
	if h.removedPages[faultPfn] {
		// Trace the page fault for removed page
		h.tracer.TracePageFault(faultPageAddr, faultPfn, "pagefault", true, false)
		return h.zeroOut(faultPageAddr)
	}

	// Find the region containing this fault address
	for i := range h.memRegions {
		region := &h.memRegions[i]
		if region.Contains(faultPageAddr) {
			// Trace the page fault
			h.tracer.TracePageFault(faultPageAddr, faultPfn, "pagefault", false, true)
			return h.populateFromFile(region, faultPageAddr, length)
		}
	}

	// Trace the page fault for address not found in any region
	h.tracer.TracePageFault(faultPageAddr, faultPfn, "pagefault", false, false)

	log.Panicf("Could not find addr: %#x within guest region mappings", addr)
	return false
}

// populateFromFile populates a page from the backing file
func (h *UffdHandler) populateFromFile(region *GuestRegionUffdMapping, dst uint64, length uint64) bool {
	offset := dst - region.BaseHostVirtAddr
	src := h.backingBuffer + uintptr(region.Offset+offset)

	copy := UffdIoCopy{
		Dst:  dst,
		Src:  uint64(src),
		Len:  length,
		Mode: 0,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(h.uffd), UFFDIO_COPY, uintptr(unsafe.Pointer(&copy)))
	if errno != 0 {
		if errno == unix.EAGAIN {
			// A 'remove' event is blocking us
			return false
		}
		if errno == unix.EEXIST {
			// Page already exists, this is ok
			return true
		}
		log.Panicf("UFFD copy failed: %v", errno)
	}

	if copy.Copy <= 0 {
		log.Panicf("UFFD copy returned non-positive value: %d", copy.Copy)
	}

	return true
}

// zeroOut zeros out a page
func (h *UffdHandler) zeroOut(addr uint64) bool {
	zero := UffdIoZeropage{
		Range: UffdIoRange{
			Start: addr,
			Len:   h.pageSize,
		},
		Mode: 0,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(h.uffd), UFFDIO_ZEROPAGE, uintptr(unsafe.Pointer(&zero)))
	if errno != 0 {
		if errno == unix.EAGAIN {
			return false
		}
		log.Panicf("Unexpected zeropage result: %v", errno)
	}

	return true
}

// Runtime manages the UFFD handler runtime
type Runtime struct {
	stream            *net.UnixConn
	backingFile       *os.File
	backingMemory     uintptr
	backingMemorySize uint64
	uffds             map[int]*UffdHandler
	streamFd          int
	tracer            *PageFaultTracer
}

// NewRuntime creates a new runtime
func NewRuntime(conn *net.UnixConn, backingFile *os.File, tracer *PageFaultTracer) (*Runtime, error) {
	fileInfo, err := backingFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file metadata: %w", err)
	}

	backingMemorySize := uint64(fileInfo.Size())

	// Memory map the backing file
	backingMemory, err := unix.Mmap(
		int(backingFile.Fd()),
		0,
		int(backingMemorySize),
		unix.PROT_READ,
		unix.MAP_PRIVATE|unix.MAP_POPULATE,
	)
	if err != nil {
		return nil, fmt.Errorf("mmap on backing file failed: %w", err)
	}

	rt := &Runtime{
		stream:            conn,
		backingFile:       backingFile,
		backingMemory:     uintptr(unsafe.Pointer(&backingMemory[0])),
		backingMemorySize: backingMemorySize,
		uffds:             make(map[int]*UffdHandler),
		streamFd:          -1,
		tracer:            tracer,
	}

	// Retrieve the underlying file descriptor for the UnixConn without
	// duplicating it. Using SyscallConn.Control gives us the real fd used by
	// the connection, which we can poll on and compare reliably.
	sc, err := conn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get SyscallConn from stream: %w", err)
	}
	var fdErr error
	err = sc.Control(func(fd uintptr) {
		rt.streamFd = int(fd)
	})
	if err != nil {
		fdErr = err
	}
	if fdErr != nil {
		return nil, fmt.Errorf("failed to obtain stream fd: %v", fdErr)
	}

	return rt, nil
}

// InstallPanicHook installs a panic hook to notify Firecracker on panic
func (r *Runtime) InstallPanicHook() {
	// Get peer process credentials
	creds, err := r.peerProcessCredentials()
	if err != nil {
		log.Printf("Warning: failed to get peer credentials: %v", err)
		return
	}

	// Setup signal handling for panic notification
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		if creds != nil {
			_ = syscall.Kill(int(creds.Pid), syscall.SIGTERM)
		}
		os.Exit(1)
	}()
}

// peerProcessCredentials gets the credentials of the peer process
func (r *Runtime) peerProcessCredentials() (*unix.Ucred, error) {
	// Get the underlying file descriptor
	connFile, err := r.stream.File()
	if err != nil {
		return nil, err
	}
	defer connFile.Close()

	creds, err := unix.GetsockoptUcred(int(connFile.Fd()), unix.SOL_SOCKET, unix.SO_PEERCRED)
	if err != nil {
		return nil, fmt.Errorf("failed to get peer process credentials: %w", err)
	}

	return creds, nil
}

// Run runs the main event loop
func (r *Runtime) Run(pfEventDispatch func(*UffdHandler)) {
	pollfds := []unix.PollFd{
		{
			Fd:     int32(r.streamFd),
			Events: unix.POLLIN,
		},
	}

	for {
		nready, err := unix.Poll(pollfds, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			log.Panicf("Could not poll for events: %v", err)
		}

		for i := 0; i < len(pollfds) && nready > 0; i++ {
			if pollfds[i].Revents&unix.POLLIN != 0 {
				nready--
				if pollfds[i].Fd == int32(r.streamFd) {
					// Handle new uffd from stream
					handler, err := NewUffdHandler(r.stream, r.backingMemory, r.backingMemorySize, r.tracer)
					if err != nil {
						log.Panicf("Failed to create UFFD handler: %v", err)
					}

					pollfds = append(pollfds, unix.PollFd{
						Fd:     int32(handler.uffd),
						Events: unix.POLLIN,
					})
					r.uffds[handler.uffd] = handler
				} else {
					// Handle one of uffd page faults
					fd := int(pollfds[i].Fd)
					if handler, ok := r.uffds[fd]; ok {
						pfEventDispatch(handler)
					}
				}
			}
		}

		// Remove closed connections
		newPollfds := pollfds[:0]
		for _, pfd := range pollfds {
			if pfd.Revents&(unix.POLLRDHUP|unix.POLLHUP) == 0 {
				newPollfds = append(newPollfds, pfd)
			}
		}
		pollfds = newPollfds
	}
}

// Helper functions

// getMappingsAndFile receives memory mappings and UFFD file descriptor from socket
func getMappingsAndFile(conn *net.UnixConn) (string, int, error) {
	// Try up to 5 times
	for i := 0; i < 5; i++ {
		body, uffdFd, err := tryGetMappingsAndFile(conn)
		if err == nil && uffdFd >= 0 {
			return body, uffdFd, nil
		}
		if err != nil {
			log.Printf("Could not get UFFD and mapping from Firecracker: %v. Retrying...", err)
		} else {
			log.Printf("Didn't receive UFFD over socket. We received: '%s'. Retrying...", body)
		}
	}
	return "", -1, fmt.Errorf("could not get UFFD and mappings after 5 retries")
}

// tryGetMappingsAndFile attempts to receive mappings and file descriptor once
func tryGetMappingsAndFile(conn *net.UnixConn) (string, int, error) {
	buf := make([]byte, 4096)
	oob := make([]byte, unix.CmsgSpace(4)) // Space for one file descriptor

	n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
	if err != nil {
		return "", -1, err
	}

	body := string(buf[:n])

	// Parse control message to extract file descriptor
	scms, err := unix.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return body, -1, nil
	}

	for _, scm := range scms {
		fds, err := unix.ParseUnixRights(&scm)
		if err != nil {
			continue
		}
		if len(fds) > 0 {
			return body, fds[0], nil
		}
	}

	return body, -1, nil
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: handler <uffd_socket_path> <mem_file_path> [trace_file]")
	}
	log.SetLevel(log.DebugLevel)
	log.Debugf("Starting handler")

	uffdSockPath := os.Args[1]
	memFilePath := os.Args[2]
	traceFilePath := ""
	if len(os.Args) > 3 {
		traceFilePath = os.Args[3]
	}

	// Open the memory file
	file, err := os.Open(memFilePath)
	if err != nil {
		log.Fatalf("Cannot open memfile: %v", err)
	}
	defer file.Close()

	// Create and bind Unix domain socket
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: uffdSockPath, Net: "unix"})
	if err != nil {
		log.Fatalf("Cannot bind to socket path: %v", err)
	}
	defer listener.Close()

	// Accept connection from Firecracker
	conn, err := listener.AcceptUnix()
	if err != nil {
		log.Fatalf("Cannot accept on UDS socket: %v", err)
	}
	defer conn.Close()

	// Create runtime
	tracer, err := NewPageFaultTracer(traceFilePath)
	if err != nil {
		log.Fatalf("Failed to create tracer: %v", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Close()
		}
	}()

	runtime, err := NewRuntime(conn, file, tracer)
	if err != nil {
		log.Fatalf("Failed to create runtime: %v", err)
	}

	runtime.InstallPanicHook()

	// Run the page fault handler
	runtime.Run(func(uffdHandler *UffdHandler) {
		// This handler deals with both 'pagefault' and 'remove' events (for balloon device support)
		// See the Rust implementation for detailed comments about the complexity of handling
		// these events correctly with respect to ordering and EAGAIN errors.

		var deferredEvents []*UffdMsg

		for {
			// First, try events that we couldn't handle last round
			eventsToHandle := make([]*UffdMsg, len(deferredEvents))
			copy(eventsToHandle, deferredEvents)
			deferredEvents = deferredEvents[:0]

			// Read all events from the userfaultfd
			for {
				event, err := uffdHandler.ReadEvent()
				if err != nil {
					log.Panicf("Failed to read uffd_msg: %v", err)
				}
				if event == nil {
					break
				}
				eventsToHandle = append(eventsToHandle, event)
			}

			// Process all events
			for _, event := range eventsToHandle {
				switch event.Event {
				case UFFD_EVENT_PAGEFAULT:
					pf := event.Pagefault()
					addr := uintptr(pf.Address)
					if !uffdHandler.ServePF(addr, uffdHandler.pageSize) {
						deferredEvents = append(deferredEvents, event)
					}

				case UFFD_EVENT_REMOVE:
					rm := event.Remove()
					uffdHandler.MarkRangeRemoved(rm.Start, rm.End)

				default:
					log.Panicf("Unexpected event on userfaultfd: %d", event.Event)
				}
			}

			// We assume that really only the pagefault/remove interaction can result in
			// deferred events. In that scenario, the loop will always terminate.
			if len(deferredEvents) == 0 {
				break
			}
		}
	})
}
