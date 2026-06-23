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

package manager

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPrepareGuestMemoryFileAndValidateGuestMemory(t *testing.T) {
	guestMemPath := filepath.Join(t.TempDir(), "guest_mem")
	guestMemSize := 4 * os.Getpagesize()

	prepareGuestMemoryFile(t, guestMemPath, guestMemSize)

	guestMem, err := os.ReadFile(guestMemPath)
	if err != nil {
		t.Fatalf("os.ReadFile returned error: %v", err)
	}
	if err := validateGuestMemory(guestMem); err != nil {
		t.Fatalf("validateGuestMemory returned error: %v", err)
	}

	guestMem[os.Getpagesize()] = 0
	if err := validateGuestMemory(guestMem); err == nil {
		t.Fatal("validateGuestMemory succeeded for corrupted guest memory")
	}
}

func TestMemoryManagerRegisterFetchPrepareDeregister(t *testing.T) {
	baseDir := t.TempDir()
	vmID := "vm-register"
	guestMemPath := filepath.Join(baseDir, "guest_mem")
	vmmStatePath := filepath.Join(baseDir, "state")

	prepareGuestMemoryFile(t, guestMemPath, os.Getpagesize())
	writeTestFile(t, vmmStatePath, "state")

	manager := NewMemoryManager(MemoryManagerCfg{})
	cfg := SnapshotStateCfg{
		VMID:         vmID,
		BaseDir:      baseDir,
		VMMStatePath: vmmStatePath,
		GuestMemPath: guestMemPath,
		GuestMemSize: os.Getpagesize(),
	}

	if err := manager.RegisterVM(cfg); err != nil {
		t.Fatalf("RegisterVM returned error: %v", err)
	}
	if err := manager.RegisterVM(cfg); err == nil {
		t.Fatal("RegisterVM succeeded for duplicate VM")
	}
	if err := manager.FetchState(vmID); err != nil {
		t.Fatalf("FetchState returned error: %v", err)
	}

	nextVMMStatePath := filepath.Join(baseDir, "next_state")
	writeTestFile(t, nextVMMStatePath, "next-state")
	nextCfg := cfg
	nextCfg.VMMStatePath = nextVMMStatePath

	if err := manager.PrepareSnapshotLoad(nextCfg); err != nil {
		t.Fatalf("PrepareSnapshotLoad returned error: %v", err)
	}
	if got := manager.instances[vmID].VMMStatePath; got != nextVMMStatePath {
		t.Fatalf("PrepareSnapshotLoad VMMStatePath = %q, want %q", got, nextVMMStatePath)
	}

	if err := manager.DeregisterVM(vmID); err != nil {
		t.Fatalf("DeregisterVM returned error: %v", err)
	}
	if err := manager.DeregisterVM(vmID); err == nil {
		t.Fatal("DeregisterVM succeeded for unregistered VM")
	}
}

func TestMemoryManagerActivateReceivesFirecrackerMappings(t *testing.T) {
	baseDir := t.TempDir()
	vmID := "vm-activate"
	guestMemPath := filepath.Join(baseDir, "guest_mem")
	socketPath := filepath.Join(baseDir, "uffd.sock")
	guestMemSize := 2 * os.Getpagesize()

	prepareGuestMemoryFile(t, guestMemPath, guestMemSize)

	manager := NewMemoryManager(MemoryManagerCfg{})
	cfg := SnapshotStateCfg{
		VMID:             vmID,
		BaseDir:          baseDir,
		GuestMemPath:     guestMemPath,
		GuestMemSize:     guestMemSize,
		InstanceSockAddr: socketPath,
	}
	if err := manager.RegisterVM(cfg); err != nil {
		t.Fatalf("RegisterVM returned error: %v", err)
	}

	mappings := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             uint64(guestMemSize),
		PageSize:         uint64(os.Getpagesize()),
	}}
	body, err := json.Marshal(mappings)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	uffdStandIn := testEventFD(t)
	socketReadyCh := make(chan error, 1)
	activateErrCh := make(chan error, 1)
	go func() {
		activateErrCh <- manager.ActivateWithSocketReady(vmID, socketReadyCh)
	}()

	if err := receiveSocketReady(t, socketReadyCh); err != nil {
		t.Fatalf("ActivateWithSocketReady failed before socket accept: %v", err)
	}

	conn := dialUnixSocketWithRetry(t, socketPath)
	if err := writeUffdSocketPayload(conn, body, uffdStandIn); err != nil {
		_ = conn.Close()
		t.Fatalf("writeUffdSocketPayload returned error: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close returned error: %v", err)
	}

	if err := receiveActivateResult(t, activateErrCh); err != nil {
		t.Fatalf("ActivateWithSocketReady returned error: %v", err)
	}

	state := manager.instances[vmID]
	if !state.isActive {
		t.Fatal("state is not active after ActivateWithSocketReady")
	}
	if !reflect.DeepEqual(state.guestRegionMappings, mappings) {
		t.Fatalf("guestRegionMappings = %+v, want %+v", state.guestRegionMappings, mappings)
	}
	if err := validateGuestMemory(state.guestMem); err != nil {
		t.Fatalf("validateGuestMemory mapped memory returned error: %v", err)
	}

	signalEventFD(t, uffdStandIn)

	if err := manager.Deactivate(vmID); err != nil {
		t.Fatalf("Deactivate returned error: %v", err)
	}
	if err := manager.DeregisterVM(vmID); err != nil {
		t.Fatalf("DeregisterVM returned error: %v", err)
	}
}

func receiveSocketReady(t *testing.T, readyCh <-chan error) error {
	t.Helper()

	select {
	case err := <-readyCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for UFFD socket readiness")
	}

	return nil
}

func receiveActivateResult(t *testing.T, activateErrCh <-chan error) error {
	t.Helper()

	select {
	case err := <-activateErrCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ActivateWithSocketReady")
	}

	return nil
}

func testEventFD(t *testing.T) *os.File {
	t.Helper()

	fd, err := unix.Eventfd(0, unix.EFD_CLOEXEC|unix.EFD_NONBLOCK)
	if err != nil {
		t.Fatalf("unix.Eventfd returned error: %v", err)
	}

	file := os.NewFile(uintptr(fd), "test-eventfd")
	if file == nil {
		_ = unix.Close(fd)
		t.Fatal("os.NewFile returned nil")
	}
	t.Cleanup(func() { _ = file.Close() })

	return file
}

func signalEventFD(t *testing.T, file *os.File) {
	t.Helper()

	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], 1)
	if _, err := unix.Write(int(file.Fd()), buf[:]); err != nil {
		t.Fatalf("unix.Write(eventfd) returned error: %v", err)
	}
}

func prepareGuestMemoryFile(t *testing.T, guestFileName string, size int) {
	t.Helper()

	toWrite := make([]byte, size)
	pages := size / os.Getpagesize()
	for i := 0; i < pages; i++ {
		for j := os.Getpagesize() * i; j < (i+1)*os.Getpagesize(); j++ {
			toWrite[j] = byte(48 + i)
		}
	}

	if err := os.WriteFile(guestFileName, toWrite, 0600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}

func validateGuestMemory(guestMem []byte) error {
	pages := len(guestMem) / os.Getpagesize()
	for i := 0; i < pages; i++ {
		j := os.Getpagesize() * i
		if guestMem[j] != byte(48+i) {
			return errors.New("incorrect guest memory")
		}
	}

	return nil
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("os.WriteFile returned error: %v", err)
	}
}
