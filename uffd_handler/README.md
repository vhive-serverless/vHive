# UFFD Handler - Go Implementation

This is a Go reimplementation of Firecracker's userfaultfd (UFFD) on-demand page fault handler, originally written in Rust.

## Overview

This handler enables on-demand loading of VM memory pages when resuming from a snapshot. It works with Firecracker's userfaultfd mechanism to handle page faults in userspace, loading memory contents from a backing file as needed.

## Original Implementation

The original Rust implementation can be found at:
https://github.com/firecracker-microvm/firecracker/blob/main/src/firecracker/examples/uffd/on_demand_handler.rs

## Features

- **On-demand page loading**: Loads memory pages from a backing file when page faults occur
- **Balloon device support**: Handles memory removal events from balloon device deflation
- **EAGAIN handling**: Properly defers page fault events when remove events are blocking the UFFD queue
- **Event ordering**: Handles the complex interaction between pagefault and remove events

## Architecture

### Key Components

1. **GuestRegionUffdMapping**: Describes memory region mappings between guest virtual addresses and backing file offsets
2. **UffdHandler**: Handles individual UFFD events and serves page faults
3. **Runtime**: Manages the main event loop, polling for events on Unix socket and UFFD file descriptors

### Event Handling

The handler processes two types of events:

- **UFFD_EVENT_PAGEFAULT**: Triggered when the guest accesses a page that hasn't been loaded yet
- **UFFD_EVENT_REMOVE**: Triggered when the balloon device frees memory (via `madvise(MADV_DONTNEED)`)

The handler implements a deferred event mechanism to handle the case where a `remove` event is pending in the UFFD queue, which causes all UFFD ioctls to return EAGAIN.

## Usage

```bash
./handler <uffd_socket_path> <memory_file_path>
```

### Arguments

- `uffd_socket_path`: Path to the Unix domain socket for communication with Firecracker
- `memory_file_path`: Path to the memory snapshot file

### Example

```bash
./handler /tmp/firecracker-uffd.sock /path/to/memory.snapshot
```

## Building

```bash
go build -o handler handler.go
```

## Integration with Firecracker

1. The handler creates and binds to a Unix domain socket
2. Firecracker connects to this socket during snapshot restoration
3. Firecracker sends:
   - Memory region mappings (as JSON)
   - The userfaultfd file descriptor
4. The handler mmaps the memory file and starts handling page faults
5. When the guest VM accesses memory, page faults are served by loading data from the backing file

## Important Notes

### Balloon Device Interaction

When using UFFD with the balloon device, the handler must deal with both `remove` and `pagefault` events. Key considerations:

1. **EAGAIN blocking**: As long as any `remove` event is pending in the UFFD queue, all ioctls return EAGAIN
2. **Event ordering**: UFFD might receive events not in their causal order due to different kernel threads
3. **Deferred events**: The handler pre-fetches all events and defers those that can't be processed immediately

### Panic Hook

The handler installs a panic hook that attempts to notify the Firecracker process (by sending SIGTERM) if the handler crashes, preventing silent failures.

## Differences from Rust Implementation

The Go implementation maintains the same logic and behavior as the Rust version, with the following adaptations:

- Uses `golang.org/x/sys/unix` for syscall interfaces
- Defines UFFD constants and structures manually (not all are in the Go unix package)
  - Constants are sourced from Linux kernel headers (`/usr/include/linux/userfaultfd.h`)
  - Event types: `UFFD_EVENT_PAGEFAULT = 0x12`, `UFFD_EVENT_REMOVE = 0x15`
  - IOCTL commands computed using the `_IOWR` macro:
    - `UFFDIO_COPY = 0xc028aa03` (computed from `_IOWR(0xAA, 0x03, 40)`)
    - `UFFDIO_ZEROPAGE = 0xc020aa04` (computed from `_IOWR(0xAA, 0x04, 32)`)
- Uses Go's native networking and file I/O APIs
- Uses Go's garbage collection instead of manual memory management

## License

This implementation follows the same Apache 2.0 license as the original Firecracker code.

## References

- [Firecracker UFFD Documentation](https://github.com/firecracker-microvm/firecracker/blob/main/docs/snapshotting/handling-page-faults-on-snapshot-resume.md)
- [Linux userfaultfd Documentation](https://www.kernel.org/doc/html/latest/admin-guide/mm/userfaultfd.html)
