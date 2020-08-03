package manager

import (
	"os"

	"golang.org/x/sys/unix"

	"io/ioutil"
	"testing"

	log "github.com/sirupsen/logrus"

	"errors"
	"fmt"
	"sync"

	ctrdlog "github.com/containerd/containerd/log"
	"github.com/stretchr/testify/require"
)

const (
	NumParallel = 2
)

func TestManagerSingleClient(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	var (
		uffd              int
		region            []byte
		regionSize        int      = 4 * pageSize
		uffdFileName      string   = "/tmp/uffd_file.file"
		guestMemoryPath            = "/tmp/guest_mem"
		memManagerBaseDir string   = "/tmp/manager"
		quitCh            chan int = make(chan int)
		vmID              string   = "1"
	)

	log.SetLevel(log.DebugLevel)

	prepareGuestMemoryFile(guestMemoryPath, regionSize)

	region, err := unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		log.Errorf("Failed to mmap: %v", err)
	}

	uffd = registerForUpf(region, uint64(regionSize))

	uffdFile := os.NewFile(uintptr(uffd), uffdFileName)

	managerCfg := &MemoryManagerCfg{
		MemManagerBaseDir: memManagerBaseDir,
	}
	manager := NewMemoryManager(managerCfg, quitCh)

	stateCfg := &SnapshotStateCfg{
		vmID:              vmID,
		guestMemPath:      guestMemoryPath,
		memManagerBaseDir: manager.MemManagerBaseDir,
		guestMemSize:      regionSize,
	}

	//time.Sleep(2 * time.Second)

	err = manager.RegisterVM(stateCfg)
	require.NoError(t, err, "Failed to register VM")
	//time.Sleep(2 * time.Second)

	err = manager.AddInstance(vmID, uffdFile)
	require.NoError(t, err, "Failed to add VM")

	//time.Sleep(2 * time.Second)

	err = validateGuestMemory(region)
	require.NoError(t, err, "Failed to validate guest memory")

	err = manager.RemoveInstance(vmID)
	require.NoError(t, err, "Failed to remove intance")

	err = manager.DeregisterVM(vmID)
	require.NoError(t, err, "Failed to deregister vm")

}

func TestManagerParallel(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	var (
		regionSize        int      = 4 * pageSize
		memManagerBaseDir string   = "/tmp/manager"
		quitCh            chan int = make(chan int)
		err               error
	)

	log.SetLevel(log.DebugLevel)

	clients := make(map[int]*upfClient)

	for i := 0; i < NumParallel; i++ {
		vmID := fmt.Sprintf("%d", i)
		guestMemoryPath := "/tmp/guest_mem_" + vmID

		prepareGuestMemoryFile(guestMemoryPath, regionSize)

		region, err := unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
		if err != nil {
			log.Errorf("Failed to mmap: %v", err)
		}

		uffd := registerForUpf(region, uint64(regionSize))
		uffdFileName := fmt.Sprintf("file_%s", vmID)
		uffdFile := os.NewFile(uintptr(uffd), uffdFileName)

		clients[i] = initClient(uffd, region, uffdFileName, guestMemoryPath, vmID, uffdFile)
	}

	var wg sync.WaitGroup

	managerCfg := &MemoryManagerCfg{
		MemManagerBaseDir: memManagerBaseDir,
	}
	manager := NewMemoryManager(managerCfg, quitCh)

	for i := 0; i < NumParallel; i++ {
		wg.Add(1)

		defer wg.Done()

		c := clients[i]
		stateCfg := &SnapshotStateCfg{
			vmID:              c.vmID,
			guestMemPath:      c.guestMemoryPath,
			memManagerBaseDir: manager.MemManagerBaseDir,
			guestMemSize:      regionSize,
		}

		err = manager.RegisterVM(stateCfg)
		require.NoError(t, err, "Failed to register VM")

		err = manager.AddInstance(c.vmID, c.uffdFile)
		require.NoError(t, err, "Failed to add VM")

		err = validateGuestMemory(c.region)
		require.NoError(t, err, "Failed to validate guest memory")

		err = manager.RemoveInstance(c.vmID)
		require.NoError(t, err, "Failed to remove intance")

		err = manager.DeregisterVM(c.vmID)
		require.NoError(t, err, "Failed to deregister vm")

	}

	wg.Wait()
}

func prepareGuestMemoryFile(guestFileName string, size int) {
	toWrite := make([]byte, size)
	pages := size / pageSize
	for i := 0; i < pages; i++ {
		for j := pageSize * i; j < (i+1)*pageSize; j++ {
			toWrite[j] = byte(48 + i)
		}
	}

	err := ioutil.WriteFile(guestFileName, toWrite, 0666)
	if err != nil {
		panic(err)
	}
}

func validateGuestMemory(guestMem []byte) error {
	pages := len(guestMem) / pageSize
	for i := 0; i < pages; i++ {
		log.Debugf("Validating page %d's contents...\n", i)
		j := pageSize * i
		if guestMem[j] != byte(48+i) {
			return errors.New("Incorrect guest memory")
		}
	}
	return nil
}

type upfClient struct {
	uffd                                int
	region                              []byte
	uffdFileName, guestMemoryPath, vmID string
	uffdFile                            *os.File
}

func initClient(uffd int, region []byte, uffdFileName, guestMemoryPath, vmID string, uffdFile *os.File) *upfClient {
	c := new(upfClient)

	c.uffd = uffd
	c.region = region
	c.uffdFileName = uffdFileName
	c.guestMemoryPath = guestMemoryPath
	c.vmID = vmID
	c.uffdFile = uffdFile

	return c
}
