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

//nolint:unused
package manager

import (
	"os"

	log "github.com/sirupsen/logrus"

	"errors"
)

/*
func TestSingleClient(t *testing.T) {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	var (
		uffd            int
		region          []byte
		regionSize      int    = 4 * os.Getpagesize()
		uffdFileName    string = "/tmp/uffd_file.file"
		guestMemoryPath        = "/tmp/guest_mem"
		vmID            string = "1"
	)

	log.SetLevel(log.DebugLevel)

	prepareGuestMemoryFile(guestMemoryPath, regionSize)

	region, err := unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
	if err != nil {
		log.Errorf("Failed to mmap: %v", err)
	}

	defer unix.Munmap(region)

	uffd = registerForUpf(region, uint64(regionSize))

	uffdFile := os.NewFile(uintptr(uffd), uffdFileName)

	managerCfg := MemoryManagerCfg{}
	manager := NewMemoryManager(managerCfg)

	stateCfg := SnapshotStateCfg{
		VMID:         vmID,
		BaseDir:      "/tmp/snap_base",
		GuestMemPath: guestMemoryPath,
		GuestMemSize: regionSize,
	}

	err = manager.RegisterVM(stateCfg)
	require.NoError(t, err, "Failed to register VM")

	err = manager.Activate(vmID, uffdFile)
	require.NoError(t, err, "Failed to add VM")

	err = validateGuestMemory(region)
	require.NoError(t, err, "Failed to validate guest memory")

	err = manager.Deactivate(vmID)
	require.NoError(t, err, "Failed to remove intance")

	err = manager.DeregisterVM(vmID)
	require.NoError(t, err, "Failed to deregister vm")
}

func TestParallelClients(t *testing.T) {
	numParallel := 1000

	log.SetFormatter(&log.TextFormatter{
		TimestampFormat: ctrdlog.RFC3339NanoFixed,
		FullTimestamp:   true,
	})
	var (
		regionSize int = 4 * os.Getpagesize()
		err        error
	)

	log.SetLevel(log.DebugLevel)

	clients := make(map[int]*upfClient)

	for i := 0; i < numParallel; i++ {
		vmID := fmt.Sprintf("%d", i)
		guestMemoryPath := "/tmp/guest_mem_" + vmID

		prepareGuestMemoryFile(guestMemoryPath, regionSize)

		region, err := unix.Mmap(-1, 0, regionSize, unix.PROT_READ, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS)
		if err != nil {
			log.Errorf("Failed to mmap: %v", err)
		}
		defer unix.Munmap(region)

		uffd := registerForUpf(region, uint64(regionSize))
		uffdFileName := fmt.Sprintf("file_%s", vmID)
		uffdFile := os.NewFile(uintptr(uffd), uffdFileName)

		clients[i] = initClient(uffd, region, uffdFileName, guestMemoryPath, vmID, uffdFile)
	}

	managerCfg := MemoryManagerCfg{}
	manager := NewMemoryManager(managerCfg)

	var wg sync.WaitGroup

	for i := 0; i < numParallel; i++ {
		c := clients[i]
		stateCfg := SnapshotStateCfg{
			VMID:         c.vmID,
			BaseDir:      "/tmp/snap_base",
			GuestMemPath: c.guestMemoryPath,
			GuestMemSize: regionSize,
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			err = manager.RegisterVM(stateCfg)
			require.NoError(t, err, "Failed to register VM")

			err = manager.Activate(c.vmID, c.uffdFile)
			require.NoError(t, err, "Failed to add VM")

			err = validateGuestMemory(c.region)
			require.NoError(t, err, "Failed to validate guest memory")

			err = manager.Deactivate(c.vmID)
			require.NoError(t, err, "Failed to remove intance")

			err = manager.DeregisterVM(c.vmID)
			require.NoError(t, err, "Failed to deregister vm")
		}()

	}

	wg.Wait()
}
*/

func prepareGuestMemoryFile(guestFileName string, size int) {
	toWrite := make([]byte, size)
	pages := size / os.Getpagesize()
	for i := 0; i < pages; i++ {
		for j := os.Getpagesize() * i; j < (i+1)*os.Getpagesize(); j++ {
			toWrite[j] = byte(48 + i)
		}
	}

	err := os.WriteFile(guestFileName, toWrite, 0777)
	if err != nil {
		panic(err)
	}
}

func validateGuestMemory(guestMem []byte) error {
	pages := len(guestMem) / os.Getpagesize()
	for i := 0; i < pages; i++ {
		log.Debugf("Validating page %d's contents...\n", i)
		j := os.Getpagesize() * i
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
