#define _GNU_SOURCE

#include <sys/types.h>
#include <linux/userfaultfd.h>
#include <sys/ioctl.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/syscall.h>
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>

// constants for use from Go
int const_UFFDIO_WAKE = UFFDIO_WAKE;
int const_UFFDIO_COPY = UFFDIO_COPY;
int const_UFFD_EVENT_PAGEFAULT = UFFD_EVENT_PAGEFAULT;
int const_UFFDIO_COPY_MODE_DONTWAKE = UFFDIO_COPY_MODE_DONTWAKE;

#define errExit(msg) \
    do { perror(msg); exit(EXIT_FAILURE); } while (0)

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
