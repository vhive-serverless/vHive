package manager

import (
	"os"
	"golang.org/x/sys/unix"

	"github.com/ftrvxmtrx/fd"
	log "github.com/sirupsen/logrus"
	"testing"
	"net"
	"context"
	"time"
	//"fmt"
)


func TestManager(t *testing.T) {
	var (
		uffd uintptr
		region []byte
		regionSize int = 4 * pageSize
		uffdSock string = "/tmp/uffd.sock"
		uffdFileName string = "/tmp/uffd_file.file"
	)

	if err := os.RemoveAll(uffdSock); err != nil {
		log.Fatal(err)
	}

	l, err := net.Listen("unix", uffdSock)
	if err != nil {
		log.Fatal("listen error:", err)
	}
	defer l.Close()



	region, err = unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE | unix.MAP_ANONYMOUS)
	if err != nil {
		log.Errorf("Failed to mmap: %v", err)
	}

	uffd = registerForUpf(region, uint64(regionSize))

	uffdFile := os.NewFile(uffd, uffdFileName)
	defer uffdFile.Close()

	putFd(uffdSock, uffdFile)
}

func putFd(uffdSock string, uffdFile *os.File) {
	var d net.Dialer
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		c, err := d.DialContext(ctx, "unix", uffdSock)
		if err != nil {
			if ctx.Err() != nil {
				log.Fatalf("Failed to dial: %v", err)
			}

			time.Sleep(1 * time.Millisecond)
			continue
		}

		defer c.Close()

		sendfdConn := c.(*net.UnixConn)

		err = fd.Put(sendfdConn, uffdFile)
		if err != nil {
			log.Fatalf("Failed to put the uffd: %v", err)
		}

		break
	}
}