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

func TestPrepareSnapshotLoadResetsWorkingSetState(t *testing.T) {
	baseDir := t.TempDir()
	vmID := "vm-prepare-ws"
	guestMemPath := filepath.Join(baseDir, "guest_mem")
	vmmStatePath := filepath.Join(baseDir, "state")
	workingSetPath := filepath.Join(baseDir, "working_set_pages")
	workingSetTracePath := filepath.Join(baseDir, "working_set_trace")

	prepareGuestMemoryFile(t, guestMemPath, 2*os.Getpagesize())
	writeTestFile(t, vmmStatePath, "state")

	manager := NewMemoryManager(MemoryManagerCfg{})
	cfg := SnapshotStateCfg{
		VMID:                vmID,
		BaseDir:             baseDir,
		VMMStatePath:        vmmStatePath,
		GuestMemPath:        guestMemPath,
		WorkingSetPath:      workingSetPath,
		WorkingSetTracePath: workingSetTracePath,
		GuestMemSize:        2 * os.Getpagesize(),
	}
	if err := manager.RegisterVM(cfg); err != nil {
		t.Fatalf("RegisterVM returned error: %v", err)
	}

	state := manager.instances[vmID]
	state.trace.AppendRecord(Record{offset: 0})
	state.isRecordReady = true
	trace := state.trace

	nextVMMStatePath := filepath.Join(baseDir, "next_state")
	nextSocketPath := filepath.Join(baseDir, "next_uffd.sock")
	nextTracePath := filepath.Join(baseDir, "next_working_set_trace")
	writeTestFile(t, nextVMMStatePath, "next-state")

	nextCfg := cfg
	nextCfg.VMMStatePath = nextVMMStatePath
	nextCfg.InstanceSockAddr = nextSocketPath
	nextCfg.WorkingSetTracePath = nextTracePath
	nextCfg.IsLazyMode = true

	if err := manager.PrepareSnapshotLoad(nextCfg); err != nil {
		t.Fatalf("PrepareSnapshotLoad returned error: %v", err)
	}

	got := manager.instances[vmID]
	if got.trace == trace {
		t.Fatal("PrepareSnapshotLoad retained the previous trace state")
	}
	if got.isRecordReady {
		t.Fatal("PrepareSnapshotLoad retained working set readiness")
	}
	if got.trace.traceFileName != nextTracePath {
		t.Fatalf("trace path = %q, want %q", got.trace.traceFileName, nextTracePath)
	}
	if got.VMMStatePath != nextVMMStatePath {
		t.Fatalf("VMMStatePath = %q, want %q", got.VMMStatePath, nextVMMStatePath)
	}
	if got.InstanceSockAddr != nextSocketPath {
		t.Fatalf("InstanceSockAddr = %q, want %q", got.InstanceSockAddr, nextSocketPath)
	}
	if !got.IsLazyMode {
		t.Fatal("PrepareSnapshotLoad did not update IsLazyMode")
	}
}

func TestFetchStateLoadsWorkingSetAcrossVMIDs(t *testing.T) {
	baseDir := t.TempDir()
	snapshotDir := filepath.Join(baseDir, "revision")
	if err := os.Mkdir(snapshotDir, 0755); err != nil {
		t.Fatalf("os.Mkdir returned error: %v", err)
	}

	guestMemPath := filepath.Join(snapshotDir, "mem_file")
	vmmStatePath := filepath.Join(snapshotDir, "snap_file")
	workingSetPath := filepath.Join(snapshotDir, "working_set_pages")
	workingSetTracePath := filepath.Join(snapshotDir, "working_set_trace")
	pageSize := uint64(os.Getpagesize())
	prepareGuestMemoryFile(t, guestMemPath, 5*int(pageSize))
	writeTestFile(t, vmmStatePath, "state")

	manager := NewMemoryManager(MemoryManagerCfg{})
	recordCfg := SnapshotStateCfg{
		VMID:                "vm-record",
		BaseDir:             filepath.Join(baseDir, "vm-record"),
		VMMStatePath:        vmmStatePath,
		GuestMemPath:        guestMemPath,
		WorkingSetPath:      workingSetPath,
		WorkingSetTracePath: workingSetTracePath,
		GuestMemSize:        5 * int(pageSize),
	}
	if err := manager.RegisterVM(recordCfg); err != nil {
		t.Fatalf("RegisterVM returned error: %v", err)
	}
	recordState := manager.instances[recordCfg.VMID]
	recordState.trace.AppendRecord(Record{offset: 3 * pageSize})
	recordState.trace.AppendRecord(Record{offset: pageSize})
	if err := recordState.trace.ProcessRecord(guestMemPath, workingSetPath, pageSize); err != nil {
		t.Fatalf("ProcessRecord returned error: %v", err)
	}

	replayCfg := recordCfg
	replayCfg.VMID = "vm-replay"
	replayCfg.BaseDir = filepath.Join(baseDir, replayCfg.VMID)
	if err := manager.PrepareSnapshotLoad(replayCfg); err != nil {
		t.Fatalf("PrepareSnapshotLoad returned error: %v", err)
	}
	if err := manager.FetchState(replayCfg.VMID); err != nil {
		t.Fatalf("FetchState returned error: %v", err)
	}

	replayState := manager.instances[replayCfg.VMID]
	if !replayState.isRecordReady {
		t.Fatal("FetchState did not mark the persisted working set ready")
	}

	guestMem, err := os.ReadFile(guestMemPath)
	if err != nil {
		t.Fatalf("os.ReadFile guest memory returned error: %v", err)
	}
	wantWorkingSet := append([]byte{}, guestMem[pageSize:2*pageSize]...)
	wantWorkingSet = append(wantWorkingSet, guestMem[3*pageSize:4*pageSize]...)
	if !reflect.DeepEqual(replayState.workingSet, wantWorkingSet) {
		t.Fatal("loaded working set does not match recorded pages")
	}
}

func TestFetchStateRejectsInvalidWorkingSetArtifacts(t *testing.T) {
	tests := []struct {
		name         string
		traceContent string
		writePages   bool
	}{
		{name: "invalid trace", traceContent: "{"},
		{name: "missing pages", traceContent: `{"version":1,"page_size":4096,"offsets":[0]}`},
		{name: "wrong page data size", traceContent: `{"version":1,"page_size":4096,"offsets":[0]}`, writePages: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			baseDir := t.TempDir()
			vmmStatePath := filepath.Join(baseDir, "snap_file")
			tracePath := filepath.Join(baseDir, "working_set_trace")
			writeTestFile(t, vmmStatePath, "state")
			writeTestFile(t, tracePath, tt.traceContent)
			if tt.writePages {
				writeTestFile(t, filepath.Join(baseDir, "working_set_pages"), "pages")
			}

			state := NewSnapshotState(SnapshotStateCfg{
				BaseDir:             baseDir,
				VMMStatePath:        vmmStatePath,
				WorkingSetPath:      filepath.Join(baseDir, "working_set_pages"),
				WorkingSetTracePath: tracePath,
			})
			if err := state.fetchState(); err == nil {
				t.Fatal("fetchState succeeded for invalid working set artifacts")
			}
			if state.isRecordReady {
				t.Fatal("invalid working set artifacts were marked ready")
			}
		})
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
	socketReadyCh := make(chan struct{}, 1)
	activateErrCh := make(chan error, 1)
	go func() {
		activateErrCh <- manager.Activate(vmID, socketReadyCh)
	}()

	receiveSocketReady(t, socketReadyCh)

	conn := dialUnixSocketWithRetry(t, socketPath)
	if err := writeUffdSocketPayload(conn, body, uffdStandIn); err != nil {
		_ = conn.Close()
		t.Fatalf("writeUffdSocketPayload returned error: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close returned error: %v", err)
	}

	if err := receiveActivateResult(t, activateErrCh); err != nil {
		t.Fatalf("Activate returned error: %v", err)
	}

	state := manager.instances[vmID]
	if !state.isActive {
		t.Fatal("state is not active after Activate")
	}
	if !reflect.DeepEqual(state.guestRegionMappings, mappings) {
		t.Fatalf("guestRegionMappings = %+v, want %+v", state.guestRegionMappings, mappings)
	}
	if err := validateGuestMemory(state.guestMem); err != nil {
		t.Fatalf("validateGuestMemory mapped memory returned error: %v", err)
	}

	if err := manager.Deactivate(vmID); err != nil {
		t.Fatalf("Deactivate returned error: %v", err)
	}
	if err := manager.DeregisterVM(vmID); err != nil {
		t.Fatalf("DeregisterVM returned error: %v", err)
	}
}

func TestMemoryManagerActivateReturnsErrorBeforeSocketListen(t *testing.T) {
	manager := NewMemoryManager(MemoryManagerCfg{})
	socketReadyCh := make(chan struct{}, 1)

	activateErr := manager.Activate("missing-vm", socketReadyCh)
	if activateErr == nil {
		t.Fatal("Activate returned nil error for an unregistered VM")
	}

	select {
	case <-socketReadyCh:
		t.Fatal("socket readiness was reported for an unregistered VM")
	default:
	}
}

func receiveSocketReady(t *testing.T, readyCh <-chan struct{}) {
	t.Helper()

	select {
	case <-readyCh:
		return
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for UFFD socket readiness")
	}
}

func receiveActivateResult(t *testing.T, activateErrCh <-chan error) error {
	t.Helper()

	select {
	case err := <-activateErrCh:
		return err
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Activate")
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
