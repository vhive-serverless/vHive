package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// UFFD constants and structures not defined in golang.org/x/sys/unix
// These values come from the Linux kernel headers: /usr/include/linux/userfaultfd.h
//
// Event types (defined directly in the kernel header):
//   #define UFFD_EVENT_PAGEFAULT 0x12
//   #define UFFD_EVENT_REMOVE    0x15
//
// IOCTL commands (computed using the _IOWR macro):
//   UFFDIO_COPY = _IOWR(UFFDIO, _UFFDIO_COPY, struct uffdio_copy)
//   where UFFDIO = 0xAA, _UFFDIO_COPY = 0x03, sizeof(struct uffdio_copy) = 40
//   Result: 0xc028aa03
//
//   UFFDIO_ZEROPAGE = _IOWR(UFFDIO, _UFFDIO_ZEROPAGE, struct uffdio_zeropage)
//   where UFFDIO = 0xAA, _UFFDIO_ZEROPAGE = 0x04, sizeof(struct uffdio_zeropage) = 32
//   Result: 0xc020aa04
//
// Mode flags:
//   #define UFFDIO_COPY_MODE_DONTWAKE ((__u64)1<<0)
//   #define UFFDIO_ZEROPAGE_MODE_DONTWAKE ((__u64)1<<0)
const (
	UFFD_EVENT_PAGEFAULT          = 0x12
	UFFD_EVENT_REMOVE             = 0x15
	UFFDIO_COPY                   = 0xc028aa03
	UFFDIO_ZEROPAGE               = 0xc020aa04
	UFFDIO_COPY_MODE_DONTWAKE     = 1
	UFFDIO_ZEROPAGE_MODE_DONTWAKE = 1
)

// UffdMsg represents the userfaultfd message structure
type UffdMsg struct {
	Event uint8
	_     [7]byte
	Arg   UffdMsgArg
}

// UffdMsgArg is a union containing different event types
type UffdMsgArg struct {
	// This is a union in C, we'll use the largest member size
	// and interpret the data based on the event type
	Data [4]uint64
}

// Pagefault returns the pagefault event data
func (a *UffdMsgArg) Pagefault() UffdMsgPagefault {
	return UffdMsgPagefault{
		Address: a.Data[0],
		Flags:   a.Data[1],
	}
}

// Remove returns the remove event data
func (a *UffdMsgArg) Remove() UffdMsgRemove {
	return UffdMsgRemove{
		Start: a.Data[0],
		End:   a.Data[1],
	}
}

// UffdMsgPagefault represents a pagefault event
type UffdMsgPagefault struct {
	Address uint64
	Flags   uint64
}

// UffdMsgRemove represents a remove event
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
}

// NewUffdHandler creates a new UFFD handler from a Unix socket stream
func NewUffdHandler(conn *net.UnixConn, backingBuffer uintptr, size uint64) (*UffdHandler, error) {
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
	}
}

// ServePF serves a page fault at the given address
func (h *UffdHandler) ServePF(addr uintptr, length uint64) bool {
	// Find the start of the page that the current faulting address belongs to
	dst := uintptr(uint64(addr) & ^(h.pageSize - 1))
	faultPageAddr := uint64(dst)
	faultPfn := faultPageAddr / h.pageSize

	log.Printf("Handling page fault at addr: %#x (pfn: %d)", addr, faultPfn)

	// If this page was removed (by balloon device), zero it out
	if h.removedPages[faultPfn] {
		return h.zeroOut(faultPageAddr)
	}

	// Find the region containing this fault address
	for i := range h.memRegions {
		region := &h.memRegions[i]
		if region.Contains(faultPageAddr) {
			return h.populateFromFile(region, faultPageAddr, length)
		}
	}

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
		Mode: UFFDIO_COPY_MODE_DONTWAKE,
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
		Mode: UFFDIO_ZEROPAGE_MODE_DONTWAKE,
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
}

// NewRuntime creates a new runtime
func NewRuntime(conn *net.UnixConn, backingFile *os.File) (*Runtime, error) {
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

	return &Runtime{
		stream:            conn,
		backingFile:       backingFile,
		backingMemory:     uintptr(unsafe.Pointer(&backingMemory[0])),
		backingMemorySize: backingMemorySize,
		uffds:             make(map[int]*UffdHandler),
	}, nil
}

// InstallPanicHook installs a panic hook to notify Firecracker on panic
func (r *Runtime) InstallPanicHook() {
	// Get peer process credentials
	creds, err := r.peerProcessCredentials()
	if err != nil {
		log.Printf("Warning: failed to get peer credentials: %v", err)
		return
	}

	// Install signal handler to kill Firecracker on panic
	oldPanic := log.Flags()
	log.SetFlags(oldPanic)

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
			Fd:     int32(getFd(r.stream)),
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
				if pollfds[i].Fd == int32(getFd(r.stream)) {
					// Handle new uffd from stream
					handler, err := NewUffdHandler(r.stream, r.backingMemory, r.backingMemorySize)
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

// getFd gets the file descriptor from a connection
func getFd(conn *net.UnixConn) int {
	connFile, err := conn.File()
	if err != nil {
		log.Panicf("Failed to get file descriptor: %v", err)
	}
	defer connFile.Close()
	return int(connFile.Fd())
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("Usage: handler <uffd_socket_path> <mem_file_path>")
	}

	uffdSockPath := os.Args[1]
	memFilePath := os.Args[2]

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
	runtime, err := NewRuntime(conn, file)
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
					pf := event.Arg.Pagefault()
					addr := uintptr(pf.Address)
					if !uffdHandler.ServePF(addr, uffdHandler.pageSize) {
						deferredEvents = append(deferredEvents, event)
					}

				case UFFD_EVENT_REMOVE:
					rm := event.Arg.Remove()
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
