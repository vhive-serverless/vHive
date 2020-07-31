#define _GNU_SOURCE

#include <sys/types.h>
#include <linux/userfaultfd.h>
#include <stdint.h>
#include <stdio.h>
#include <unistd.h>
#include <sys/ioctl.h>
#include <errno.h>
#include <stdlib.h>
#include <fcntl.h>
#include <signal.h>
#include <poll.h>
#include <string.h>
#include <sys/mman.h>
#include <sys/syscall.h>


uint64_t start_addr = 0ULL;
unsigned long long start_addr_u64 = 0ULL;
char* src_start = NULL;
unsigned long long src_start_u64 = 0ULL;
char* src_start_ws = NULL;
unsigned long long src_start_ws_u64 = 0ULL;
int page_size = 4096;

// constants for use from Go
int const_UFFDIO_WAKE = UFFDIO_WAKE;
int const_UFFDIO_COPY = UFFDIO_COPY;
int const_UFFD_EVENT_PAGEFAULT = UFFD_EVENT_PAGEFAULT;
int const_UFFDIO_COPY_MODE_DONTWAKE = UFFDIO_COPY_MODE_DONTWAKE;

#define errExit(msg) \
    do { perror(msg); exit(EXIT_FAILURE); } while (0)

void mmap_guest_memory_file(int64_t size, int prefault) {
    int fd = open("/home/ustiugov/mem_file", O_RDONLY, (mode_t)0600);
    if (fd == -1) {
	errExit("read");
    }

    char* addr;

    if (prefault == 0) {
	addr = (char*) mmap(NULL, size, PROT_READ, MAP_PRIVATE, fd, 0);
    } else {
	addr = (char*) mmap(NULL, size, PROT_READ, MAP_PRIVATE|MAP_POPULATE, fd, 0);
    }
    if (addr == MAP_FAILED)
	errExit("preallocate_pages: mmap");

    src_start = addr;
    src_start_u64 = (unsigned long) (src_start);
}

char* mmap_ws_file_read(int64_t size) {
    int fd = open("./ws", O_RDONLY|O_DIRECT, (mode_t)0600);
    if (fd == -1) {
	errExit("read");
    }

    void* addr = NULL;
    int ret = posix_memalign(&addr, 4096, size); // must be FS block size aligned
    if (ret != 0) {
	errExit("memalign failed");
    }

    ret = read(fd, addr, size);
    if (ret == -1) {
	errExit("read failed");
    }

    ret = close(fd);
    if (ret == -1) {
	errExit("read failed");
    }

    return (char*)addr;
}

uint64_t get_address(void* msg_ptr) {
    struct uffd_msg msg = *(struct uffd_msg*)msg_ptr;
    if (msg.event != UFFD_EVENT_PAGEFAULT) {
	printf("Wrong event type\n");
	exit(EXIT_FAILURE);
    }

    if (start_addr == 0ULL) {
	start_addr = msg.arg.pagefault.address;
	start_addr_u64 = (unsigned long) (start_addr);
    }

    return msg.arg.pagefault.address;
}

void fetch_ws(int ws_size) {
    src_start_ws = mmap_ws_file_read(page_size * ws_size);
    src_start_ws_u64 = (unsigned long) (src_start_ws);
}

void serve_fault(int uffd, uint64_t address) {
    struct uffdio_copy uffdio_copy;

    uint64_t offset = address - start_addr;
    uffdio_copy.mode = 0;
    uffdio_copy.copy = 0;
    uffdio_copy.src = (unsigned long) (src_start + offset);
    uffdio_copy.dst = (unsigned long) address & ~(page_size - 1);
    uffdio_copy.len = page_size;

    if (ioctl(uffd, UFFDIO_COPY, &uffdio_copy) == -1)
	errExit("ioctl-UFFDIO_COPY");
}

void install_region(int uffd, uint64_t reg_address, int len) {
    struct uffdio_copy uffdio_copy;

    uint64_t offset = reg_address - start_addr;
    uffdio_copy.mode = UFFDIO_COPY_MODE_DONTWAKE;
    uffdio_copy.copy = 0;
    uffdio_copy.src = (unsigned long) (src_start + offset);
    uffdio_copy.dst = (unsigned long) reg_address & ~(page_size - 1);
    uffdio_copy.len = page_size * len;

    if (ioctl(uffd, UFFDIO_COPY, &uffdio_copy) == -1)
	errExit("ioctl-UFFDIO_COPY");
}

void install_region_ws(int uffd, uint64_t reg_address, uint64_t src_offset, int len) {
    struct uffdio_copy uffdio_copy;

    uffdio_copy.mode = UFFDIO_COPY_MODE_DONTWAKE;
    uffdio_copy.copy = 0;
    uffdio_copy.src = (unsigned long) (src_start_ws + src_offset);
    uffdio_copy.dst = (unsigned long) reg_address & ~(page_size - 1);
    uffdio_copy.len = page_size * len;

    if (ioctl(uffd, UFFDIO_COPY, &uffdio_copy) == -1)
	errExit("ioctl-UFFDIO_COPY");
}

void wake(int uffd) {
    struct uffdio_range uffdio_range;
    uffdio_range.start = (unsigned long) start_addr;
    uffdio_range.len = 512 * 1024 * 1024;
    if (ioctl(uffd, UFFDIO_WAKE, &uffdio_range) == -1)
	errExit("ioctl-UFFDIO_WAKE");
}

long register_for_upf(void *start_address, unsigned long len) {
    struct uffdio_api uffdio_api;
    struct uffdio_register uffdio_register;
    long uffd;

    uffd = syscall(__NR_userfaultfd, O_CLOEXEC | O_NONBLOCK);
    if (uffd == -1)
            errExit("userfaultfd");

    uffdio_api.api = UFFD_API;
    uffdio_api.features = 0;
    if (ioctl(uffd, UFFDIO_API, &uffdio_api) == -1)
        errExit("ioctl-UFFDIO_API");

    uffdio_register.range.start = (unsigned long) start_address;
    uffdio_register.range.len = len;
    uffdio_register.mode = UFFDIO_REGISTER_MODE_MISSING;
    if (ioctl(uffd, UFFDIO_REGISTER, &uffdio_register) == -1)
        errExit("ioctl-UFFDIO_REGISTER");

    return uffd;
}
