package uffd_handler

import (
	"crypto/md5"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/snapshotting"

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
// IOCTL commands (computed using the _IOWR or _IOR macros):
//
//	UFFDIO_COPY = _IOWR(UFFDIO, _UFFDIO_COPY, struct uffdio_copy)
//	where UFFDIO = 0xAA, _UFFDIO_COPY = 0x03, sizeof(struct uffdio_copy) = 40
//	Result: 0xc028aa03
//
//	UFFDIO_ZEROPAGE = _IOWR(UFFDIO, _UFFDIO_ZEROPAGE, struct uffdio_zeropage)
//	where UFFDIO = 0xAA, _UFFDIO_ZEROPAGE = 0x04, sizeof(struct uffdio_zeropage) = 32
//	Result: 0xc020aa04
//
//	UFFDIO_WAKE = _IOR(UFFDIO, _UFFDIO_WAKE, struct uffdio_range)
//	where UFFDIO = 0xAA, _UFFDIO_WAKE = 0x02, sizeof(struct uffdio_range) = 16
//	Result: 0x8010aa02
//
// Mode flags:
//
//	UFFDIO_COPY_MODE_DONTWAKE = (1 << 0)
//	Result: 0x1
//
// Page fault flags:
//
//	UFFD_PAGEFAULT_FLAG_MINOR = (1 << 2)
//	Result: 0x4
const (
	UFFD_EVENT_PAGEFAULT      = 0x12
	UFFD_EVENT_REMOVE         = 0x15
	UFFDIO_COPY               = 0xc028aa03
	UFFDIO_ZEROPAGE           = 0xc020aa04
	UFFDIO_CONTINUE           = 0xc020aa07
	UFFDIO_WAKE               = 0x8010aa02
	UFFDIO_COPY_MODE_DONTWAKE = 0x1
	UFFD_PAGEFAULT_FLAG_MINOR = 0x4
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

// UffdIoContinue represents the ioctl continue structure
type UffdIoContinue struct {
	Range  UffdIoRange
	Mode   uint64
	Mapped int64
}

// UffdIoRange represents a range of addresses
type UffdIoRange struct {
	Start uint64
	Len   uint64
}

type MappedChunkInfo struct {
	addr         uintptr
	chunkContent []byte
}

// PageOperations encapsulates the page-level operations for UFFD handling.
// This structure is shared across multiple UFFD handlers to provide consistent
// behavior without duplicating configuration.
type PageOperations struct {
	backingBuffer      uintptr
	pageSize           uint64
	workingSet         []uint64
	firstPageFaultOnce *sync.Once
	lazy               bool
	mappedChunks       sync.Map
	keyLocks           sync.Map
	snapMgr            *snapshotting.SnapshotManager
	threads            int
}

// NewPageOperations creates a new PageOperations instance
// TODO: Remove mappedChunks from signature: it's obsolete
func NewPageOperations(backingBuffer uintptr, pageSize uint64, workingSet []uint64, lazy bool, snapMgr *snapshotting.SnapshotManager, threads int) *PageOperations {
	return &PageOperations{
		backingBuffer:      backingBuffer,
		pageSize:           pageSize,
		workingSet:         workingSet,
		firstPageFaultOnce: &sync.Once{},
		lazy:               lazy,
		snapMgr:            snapMgr,
		threads:            threads,
	}
}

// PopulateFromFile populates a page from the backing file
func (po *PageOperations) PopulateFromFile(uffd int, region *GuestRegionUffdMapping, dst uint64, length uint64) bool {
	po.firstPageFaultOnce.Do(func() {
		po.insertWorkingSet(uffd, region)
	})

	offset := dst - region.BaseHostVirtAddr

	src := uintptr(0)
	if !po.lazy {
		src = po.backingBuffer + uintptr(region.Offset+offset)
	} else {
		// In lazy mode, read the MD5 hash from the recipe file
		recipeOffset := (region.Offset + offset) / po.snapMgr.GetChunkSize() * md5.Size
		hashBytes := (*[md5.Size]byte)(unsafe.Pointer(po.backingBuffer + uintptr(recipeOffset)))
		var hashKey [md5.Size]byte
		copy(hashKey[:], hashBytes[:])
		mappedAddr, err := po.mapChunk(hashKey)

		if err != nil {
			log.Errorf("Failed to map chunk: %v", err)
			return false
		}

		src = mappedAddr + (uintptr(region.Offset+offset) % uintptr(po.snapMgr.GetChunkSize()))
	}

	copy := UffdIoCopy{
		Dst:  dst,
		Src:  uint64(src),
		Len:  length,
		Mode: 0,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(uffd), UFFDIO_COPY, uintptr(unsafe.Pointer(&copy)))
	if errno != 0 {
		if errno == unix.EAGAIN {
			// A 'remove' event is blocking us
			return false
		}
		if errno == unix.EEXIST {
			// Page already exists, this is ok
			// Wake up any threads that might be waiting on the pages we just inserted
			wakeRange := UffdIoRange{
				Start: dst,
				Len:   region.PageSize,
			}
			_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(uffd), uintptr(UFFDIO_WAKE), uintptr(unsafe.Pointer(&wakeRange)))
			if errno != 0 {
				log.Errorf("UFFD wake failed: %v", errno)
			}
			return true
		}
		log.Errorf("UFFD copy failed: %v", errno)
	}

	if copy.Copy <= 0 {
		log.Errorf("UFFD copy returned non-positive value: %d", copy.Copy)
	}

	return true
}

func (po *PageOperations) insertWorkingSet(uffd int, region *GuestRegionUffdMapping) {
	startTime := time.Now()
	var counter int32

	defer func() {
		mode := ""
		if po.lazy {
			mode = "(Lazy Version) "
		}
		log.Infof("%sPre-inserting working set of %d pages in %v", mode, atomic.LoadInt32(&counter), time.Since(startTime))
	}()

	pfnCh := make(chan uint64, len(po.workingSet))
	for _, pfn := range po.workingSet {
		pageAddr := pfn * po.pageSize
		if pageAddr >= region.Offset && pageAddr < region.Offset+region.Size {
			pfnCh <- pfn
		}
	}
	close(pfnCh)

	var wg sync.WaitGroup
	wg.Add(po.threads)

	for w := 0; w < po.threads; w++ {
		go func() {
			defer wg.Done()
			for pfn := range pfnCh {
				pageAddr := pfn * po.pageSize
				atomic.AddInt32(&counter, 1)

				var src uintptr
				if po.lazy {
					recipeOffset := (pageAddr) / po.snapMgr.GetChunkSize() * md5.Size
					hashBytes := (*[md5.Size]byte)(unsafe.Pointer(po.backingBuffer + uintptr(recipeOffset)))
					var hashKey [md5.Size]byte
					copy(hashKey[:], hashBytes[:])

					mappedAddr, err := po.mapChunk(hashKey)
					if err != nil {
						log.Errorf("Failed to map chunk: %v", err)
						continue
					}
					src = mappedAddr + (uintptr(pageAddr) % uintptr(po.snapMgr.GetChunkSize()))
				} else {
					src = po.backingBuffer + uintptr(pageAddr)
				}

				copy := UffdIoCopy{
					Dst:  pageAddr + region.BaseHostVirtAddr,
					Src:  uint64(src),
					Len:  po.pageSize,
					Mode: UFFDIO_COPY_MODE_DONTWAKE,
				}

				_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(uffd), UFFDIO_COPY, uintptr(unsafe.Pointer(&copy)))
				if errno != 0 && errno != unix.EAGAIN && errno != unix.EEXIST {
					log.Errorf("UFFD copy failed: %v", errno)
				}
			}
		}()
	}

	wg.Wait()
}

func (po *PageOperations) mapChunk(hashKey [md5.Size]byte) (uintptr, error) {
	// Return already mapped chunk if exists
	if value, ok := po.mappedChunks.Load(hashKey); ok {
		return value.(*MappedChunkInfo).addr, nil
	}

	// Ensure per-key lock exists
	lockIface, _ := po.keyLocks.LoadOrStore(hashKey, &sync.Mutex{})
	keyMu := lockIface.(*sync.Mutex)
	keyMu.Lock()
	defer keyMu.Unlock()

	if value, ok := po.mappedChunks.Load(hashKey); ok {
		return value.(*MappedChunkInfo).addr, nil
	}

	hash := hex.EncodeToString(hashKey[:])
	chunkContent, err := po.snapMgr.DownloadAndReturnChunk(hash)
	if err != nil {
		return 0, fmt.Errorf("failed to download and return chunk file %s: %w", hash, err)
	}

	mappedAddr := uintptr(unsafe.Pointer(&chunkContent[0]))
	po.mappedChunks.Store(hashKey, &MappedChunkInfo{
		addr:         mappedAddr,
		chunkContent: chunkContent,
	})

	return mappedAddr, nil
}

// ZeroOut zeros out a page
func (po *PageOperations) ZeroOut(uffd int, addr uint64) bool {
	zero := UffdIoZeropage{
		Range: UffdIoRange{
			Start: addr,
			Len:   po.pageSize,
		},
		Mode: 0,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(uffd), UFFDIO_ZEROPAGE, uintptr(unsafe.Pointer(&zero)))
	if errno != 0 {
		if errno == unix.EAGAIN {
			return false
		}
		log.Errorf("Unexpected zeropage result: %v", errno)
	}

	return true
}

// PageFaultTracer handles tracing page fault events to a file
type PageFaultTracer struct {
	file    *os.File
	writer  *csv.Writer
	counter uint64
	logger  *log.Entry
}

// NewPageFaultTracer creates a new page fault tracer
func NewPageFaultTracer(filePath string, logger *log.Entry) (*PageFaultTracer, error) {
	if filePath == "" {
		return nil, nil
	}

	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace file: %w", err)
	}

	writer := csv.NewWriter(file)

	// Write CSV header
	if err := writer.Write([]string{"pfn"}); err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to write CSV header: %w", err)
	}
	writer.Flush()

	return &PageFaultTracer{
		file:    file,
		writer:  writer,
		counter: 0,
		logger:  logger,
	}, nil
}

// TracePageFault logs a page fault event
func (t *PageFaultTracer) TracePageFault(address, pfn uint64, eventType string, isRemoved, regionFound bool) {
	if t == nil {
		return
	}
	t.counter++
	if t.writer == nil {
		return
	}

	// Write PFN to CSV
	if err := t.writer.Write([]string{strconv.FormatUint(pfn, 10)}); err != nil {
		log.Errorf("Failed to write trace data: %v", err)
		return
	}
	t.writer.Flush()
}

// Close closes the trace file
func (t *PageFaultTracer) Close() error {
	if t == nil {
		return nil
	}
	t.logger.Debugf("Handled %d page faults", t.counter)
	if t.writer != nil {
		t.writer.Flush()
	}
	if t.file != nil {
		return t.file.Close()
	}
	return nil
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
	memRegions   []GuestRegionUffdMapping
	pageSize     uint64
	uffd         int
	removedPages map[uint64]bool
	tracer       *PageFaultTracer
	pageOps      *PageOperations
}

// NewUffdHandler creates a new UFFD handler from a Unix socket stream
func NewUffdHandler(conn *net.UnixConn, pageOps *PageOperations, size uint64, tracer *PageFaultTracer) (*UffdHandler, error) {
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
		memRegions:   mappings,
		pageSize:     pageSize,
		uffd:         uffdFd,
		removedPages: make(map[uint64]bool),
		tracer:       tracer,
		pageOps:      pageOps,
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

		// Find the region and calculate backing file PFN for removed page
		backingFilePfn := uint64(0)
		for i := range h.memRegions {
			region := &h.memRegions[i]
			if region.Contains(pageAddr) {
				offset := pageAddr - region.BaseHostVirtAddr
				backingFilePfn = (region.Offset + offset) / h.pageSize
				break
			}
		}

		h.tracer.TracePageFault(pageAddr, backingFilePfn, "remove", true, false)
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
		// Trace the page fault for removed page (no backing file offset for zeroed pages)
		h.tracer.TracePageFault(faultPageAddr, (^uint64(0)), "pagefault", true, false)
		return h.pageOps.ZeroOut(h.uffd, faultPageAddr)
	}

	// Find the region containing this fault address
	for i := range h.memRegions {
		region := &h.memRegions[i]
		if region.Contains(faultPageAddr) {
			// Calculate the actual offset in the backing file
			offset := faultPageAddr - region.BaseHostVirtAddr
			backingFilePfn := (region.Offset + offset) / h.pageSize

			// Trace the page fault with the actual PFN from backing file
			h.tracer.TracePageFault(faultPageAddr, backingFilePfn, "pagefault", false, true)
			return h.pageOps.PopulateFromFile(h.uffd, region, faultPageAddr, length)
		}
	}

	// Trace the page fault for address not found in any region
	h.tracer.TracePageFault(faultPageAddr, (^uint64(0)), "pagefault", false, false)

	log.Errorf("Could not find addr: %#x within guest region mappings", addr)
	return false
}

// Continue handles a minor page fault by issuing UFFDIO_CONTINUE
func (h *UffdHandler) Continue(addr uintptr) bool {
	// Find the start of the page that the current faulting address belongs to
	dst := uintptr(uint64(addr) & ^(h.pageSize - 1))
	faultPageAddr := uint64(dst)

	cont := UffdIoContinue{
		Range: UffdIoRange{
			Start: faultPageAddr,
			Len:   h.pageSize,
		},
		Mode: 0,
	}

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(h.uffd), UFFDIO_CONTINUE, uintptr(unsafe.Pointer(&cont)))
	if errno != 0 {
		if errno == unix.EAGAIN {
			return false
		}
		log.Errorf("UFFD continue failed: %v", errno)
		return false
	}

	if cont.Mapped <= 0 {
		log.Errorf("UFFD continue returned non-positive value: %d", cont.Mapped)
		return false
	}

	return true
}

// Runtime manages the UFFD handler runtime
type Runtime struct {
	stream             *net.UnixConn
	backingFile        *os.File
	backingMemory      uintptr
	backingMemoryBytes []byte
	backingMemorySize  uint64
	uffds              map[int]*UffdHandler
	streamFd           int
	tracer             *PageFaultTracer
	lazy               bool
	snapMgr            *snapshotting.SnapshotManager
	pageOps            *PageOperations
	logger             *log.Entry
}

// NewRuntime creates a new runtime
func NewRuntime(conn *net.UnixConn, backingFile *os.File, wsFile *os.File, tracer *PageFaultTracer, lazy bool, snapMgr *snapshotting.SnapshotManager, threads int, logger *log.Entry) (*Runtime, error) {
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

	ws := make([]uint64, 0)
	if lazy {
		// in case of lazy, the backing memory file is just a recipe file containing md5 hashes
		backingMemorySize *= uint64(snapMgr.GetChunkSize()) / uint64(md5.Size)
	}

	if wsFile != nil {
		// Read working set file to pre-map pages
		scanner := csv.NewReader(wsFile)
		records, err := scanner.ReadAll()
		if err != nil {
			return nil, fmt.Errorf("failed to read working set file: %w", err)
		}

		// Skip header row and convert PFNs to addresses
		for i := 1; i < len(records); i++ {
			if len(records[i]) == 0 {
				continue
			}
			pfn, err := strconv.ParseUint(records[i][0], 10, 64)
			if err != nil {
				log.Warnf("Failed to parse PFN from working set: %v", err)
				continue
			}
			ws = append(ws, uint64(pfn))
		}
	}

	backingMemoryPtr := uintptr(unsafe.Pointer(&backingMemory[0]))

	// Create PageOperations with a reasonable default page size
	// The actual page size will be validated when handlers are created
	pageOps := NewPageOperations(backingMemoryPtr, 4096, ws, lazy, snapMgr, threads)

	rt := &Runtime{
		stream:             conn,
		backingFile:        backingFile,
		backingMemory:      backingMemoryPtr,
		backingMemoryBytes: backingMemory,
		backingMemorySize:  backingMemorySize,
		uffds:              make(map[int]*UffdHandler),
		streamFd:           -1,
		tracer:             tracer,
		lazy:               lazy,
		snapMgr:            snapMgr,
		pageOps:            pageOps,
		logger:             logger,
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
			r.logger.Errorf("Could not poll for events: %v", err)
		}

		for i := 0; i < len(pollfds) && nready > 0; i++ {
			if pollfds[i].Revents&unix.POLLIN != 0 {
				nready--
				if pollfds[i].Fd == int32(r.streamFd) {
					// Handle new uffd from stream
					handler, err := NewUffdHandler(r.stream, r.pageOps, r.backingMemorySize, r.tracer)
					r.logger.Debugf("Created new UFFD handler: %v", handler)
					if err != nil {
						if strings.Contains(err.Error(), "EOF") {
							r.logger.Infof("Peer terminated, shutting down gracefully")
							return
						}
						r.logger.Errorf("Failed to create UFFD handler: %v", err)
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

func (r *Runtime) FreeBackingFile() {
	err := unix.Munmap(r.backingMemoryBytes)
	if err != nil {
		log.Fatalf("Failed to unmap memory: %v", err)
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
			if strings.Contains(err.Error(), "EOF") {
				return "", -1, err
			}
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

func StartUffdHandler(uffdSockPath string, memFilePath string, traceFilePath string, wsFilePath string, lazy bool, snapMgr *snapshotting.SnapshotManager, threads int) error {
	log.Debugf("Starting handler at %s", uffdSockPath)

	// Open the memory file
	file, err := os.Open(memFilePath)
	if err != nil {
		return fmt.Errorf("cannot open memfile: %w", err)
	}
	defer file.Close()

	// Create and bind Unix domain socket
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: uffdSockPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("cannot bind to socket path: %w", err)
	}
	log.Debugf("opened the listener at %s", uffdSockPath)
	defer listener.Close()

	// Accept connection from Firecracker
	conn, err := listener.AcceptUnix()
	if err != nil {
		return fmt.Errorf("cannot accept on UDS socket: %w", err)
	}
	defer conn.Close()

	tracer, err := NewPageFaultTracer(traceFilePath, log.WithField("uffd", uffdSockPath))
	if err != nil {
		return fmt.Errorf("failed to create tracer: %w", err)
	}
	defer func() {
		if tracer != nil {
			tracer.Close()
		}
	}()

	var wsFile *os.File
	if wsFilePath != "" {
		// If working set file path is provided, check if it exists and load it
		if stat, err := os.Stat(wsFilePath); err == nil && stat != nil {
			log.Debugf("Loading existing WS file from %s", wsFilePath)
			wsFile, err = os.Open(wsFilePath)
			if err != nil {
				return fmt.Errorf("cannot open WS file: %w", err)
			}
			defer wsFile.Close()
		} else {
			log.Errorf("Failed to open WS file %s: %v", wsFilePath, err)
		}
	}

	// Create runtime
	runtime, err := NewRuntime(conn, file, wsFile, tracer, lazy, snapMgr, threads, log.WithField("uffd", uffdSockPath))
	if err != nil {
		return fmt.Errorf("failed to create runtime: %w", err)
	}

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
					log.Errorf("Failed to read uffd_msg: %v", err)
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
					if pf.Flags&UFFD_PAGEFAULT_FLAG_MINOR != 0 {
						if !uffdHandler.Continue(addr) {
							deferredEvents = append(deferredEvents, event)
						}
					} else if !uffdHandler.ServePF(addr, uffdHandler.pageSize) {
						deferredEvents = append(deferredEvents, event)
					}

				case UFFD_EVENT_REMOVE:
					rm := event.Remove()
					uffdHandler.MarkRangeRemoved(rm.Start, rm.End)

				default:
					log.Errorf("Unexpected event on userfaultfd: %d", event.Event)
				}
			}

			// We assume that really only the pagefault/remove interaction can result in
			// deferred events. In that scenario, the loop will always terminate.
			if len(deferredEvents) == 0 {
				break
			}
		}
	})

	runtime.FreeBackingFile()
	return nil
}
