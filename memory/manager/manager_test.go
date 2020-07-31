package manager

import (
	"os"
	"golang.org/x/sys/unix"

	log "github.com/sirupsen/logrus"
	"testing"
	"io/ioutil"
	//"time"
	"fmt"
	"sync"
	"errors"
	"github.com/stretchr/testify/require"
)

const (
	NumParallel = 2
)

func TestManager(t *testing.T) {
	var (
		uffd uintptr
		region []byte
		regionSize int = 4 * pageSize
		uffdFileName string = "/tmp/uffd_file.file"
		guestMemoryPath = "/tmp/guest_mem"
		memManagerBaseDir string = "/tmp/manager"
		quitCh chan int = make(chan int)
		vmID string = "1"
	)

	log.SetLevel(log.DebugLevel)

	prepareGuestMemoryFile(guestMemoryPath, regionSize)

	region, err := unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE | unix.MAP_ANONYMOUS)
	if err != nil {
		log.Errorf("Failed to mmap: %v", err)
	}

	uffd = registerForUpf(region, uint64(regionSize))

	uffdFile := os.NewFile(uffd, uffdFileName)

	managerCfg := &MemoryManagerCfg{
		MemManagerBaseDir : memManagerBaseDir,
	}
	manager := NewMemoryManager(managerCfg, quitCh)

	stateCfg := &SnapshotStateCfg{
		vmID: vmID,
		guestMemPath: guestMemoryPath,
		memManagerBaseDir: manager.MemManagerBaseDir,
		guestMemSize: regionSize,
	}

	err = manager.RegisterVM(stateCfg)
	require.NoError(t, err, "Failed to register VM")

	err = manager.AddInstance(vmID, uffdFile)
	require.NoError(t, err, "Failed to add VM")

	err = validateGuestMemory(region)
	require.NoError(t, err, "Failed to validate guest memory")

	err = manager.RemoveInstance(vmID)
	require.NoError(t, err, "Failed to remove intance")

	err = manager.DeregisterVM(vmID)
	require.NoError(t, err, "Failed to deregister vm")
}

func TestManagerParallel(t *testing.T) {
	var (
		regionSize int = 4 * pageSize
		uffd []uintptr = make([]uintptr, NumParallel, NumParallel)
		region [][]byte = make([][]byte, NumParallel, NumParallel)
		uffdFileName []string = make([]string, NumParallel, NumParallel)
		guestMemoryPath []string = make([]string, NumParallel, NumParallel)
		memManagerBaseDir string = "/tmp/manager"
		quitCh chan int = make(chan int)
		vmID []string = make([]string, NumParallel, NumParallel)
		uffdFile []*os.File = make([]*os.File, NumParallel, NumParallel)
		err error
	)

	log.SetLevel(log.DebugLevel)

	for i := 0; i < NumParallel; i++ {
		vmID[i] = fmt.Sprintf("%d", i)
		guestMemoryPath[i] = "/tmp/guest_mem_" + vmID[i]

		prepareGuestMemoryFile(guestMemoryPath[i], regionSize)

		region[i], err = unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE | unix.MAP_ANONYMOUS)
		if err != nil {
			log.Errorf("Failed to mmap: %v", err)
		}

		uffd[i] = registerForUpf(region[i], uint64(regionSize))

		uffdFileName[i] = fmt.Sprintf("/tmp/uffd_file_%s.file", vmID[i])

		uffdFile[i] = os.NewFile(uffd[i], uffdFileName[i])
	}

	var wg sync.WaitGroup

	managerCfg := &MemoryManagerCfg{
		MemManagerBaseDir : memManagerBaseDir,
	}
	manager := NewMemoryManager(managerCfg, quitCh)

	for i := 0; i < NumParallel; i++ {
		wg.Add(1)

			defer wg.Done()
			stateCfg := &SnapshotStateCfg{
				vmID: vmID[i],
				guestMemPath: guestMemoryPath[i],
				memManagerBaseDir: manager.MemManagerBaseDir,
				guestMemSize: regionSize,
			}

			err = manager.RegisterVM(stateCfg)
			require.NoError(t, err, "Failed to register VM")		

			err = manager.AddInstance(vmID[i], uffdFile[i])
			require.NoError(t, err, "Failed to add VM")		

			err = validateGuestMemory(region[i])
			require.NoError(t, err, "Failed to validate guest memory")		

			err = manager.RemoveInstance(vmID[i])
			require.NoError(t, err, "Failed to remove intance")		

			err = manager.DeregisterVM(vmID[i])
			require.NoError(t, err, "Failed to deregister vm")

	}

	wg.Wait()
}

func prepareGuestMemoryFile(guestFileName string, size int) {
	toWrite := make([]byte, size)
	pages := size / pageSize
	for i := 0; i < pages; i++ {
		for j := pageSize * i; j < (i+1) * pageSize; j++ {
			toWrite[j] = byte(48+i)
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
		for j := pageSize * i; j < (i+1) * pageSize; j++ {
			if guestMem[j] != byte(48+i) {
				return errors.New("Incorrect guest memory")
			}
		} 
	}
	return nil	
}

