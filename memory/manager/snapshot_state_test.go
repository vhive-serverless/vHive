package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestPageAlignFaultAddress(t *testing.T) {
	region := GuestRegionUffdMapping{
		BaseHostVirtAddr: 0x100000,
		Size:             0x4000,
		PageSize:         0x1000,
	}

	got, err := pageAlignFaultAddress(0x101234, region)
	if err != nil {
		t.Fatalf("pageAlignFaultAddress returned error: %v", err)
	}
	if want := uint64(0x101000); got != want {
		t.Fatalf("pageAlignFaultAddress() = %#x, want %#x", got, want)
	}
}

func TestPageFaultCopyArgsForFault(t *testing.T) {
	tests := []struct {
		name    string
		regions []GuestRegionUffdMapping
		fault   uint64
		want    pageFaultCopyArgs
	}{
		{
			name: "zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				PageSize:         0x1000,
			}},
			fault: 0x102000,
			want: pageFaultCopyArgs{
				srcOffset: 0x2000,
				dstAddr:   0x102000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "non-zero offset",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x800000,
				PageSize:         0x1000,
			}},
			fault: 0x103000,
			want: pageFaultCopyArgs{
				srcOffset: 0x803000,
				dstAddr:   0x103000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "multiple regions",
			regions: []GuestRegionUffdMapping{
				{
					BaseHostVirtAddr: 0x100000,
					Size:             0x2000,
					PageSize:         0x1000,
				},
				{
					BaseHostVirtAddr: 0x200000,
					Size:             0x3000,
					Offset:           0x900000,
					PageSize:         0x1000,
				},
			},
			fault: 0x201000,
			want: pageFaultCopyArgs{
				srcOffset: 0x901000,
				dstAddr:   0x201000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "not page-aligned",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x4000,
				Offset:           0x300000,
				PageSize:         0x1000,
			}},
			fault: 0x101234,
			want: pageFaultCopyArgs{
				srcOffset: 0x301000,
				dstAddr:   0x101000,
				copyLen:   0x1000,
				copyMode:  0,
			},
		},
		{
			name: "larger page size",
			regions: []GuestRegionUffdMapping{{
				BaseHostVirtAddr: 0x100000,
				Size:             0x8000,
				Offset:           0x500000,
				PageSize:         0x2000,
			}},
			fault: 0x103456,
			want: pageFaultCopyArgs{
				srcOffset: 0x502000,
				dstAddr:   0x102000,
				copyLen:   0x2000,
				copyMode:  0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := pageFaultCopyArgsForFault(tt.regions, tt.fault)
			if err != nil {
				t.Fatalf("pageFaultCopyArgsForFault returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("pageFaultCopyArgsForFault() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPageFaultCopyArgsForFaultOutsideAllRegions(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0x1000,
	}}

	_, err := pageFaultCopyArgsForFault(regions, 0x103000)
	if !errors.Is(err, errGuestRegionNotFound) {
		t.Fatalf("pageFaultCopyArgsForFault() error = %v, want %v", err, errGuestRegionNotFound)
	}
}

func TestPageFaultCopyArgsForFaultZeroPageSize(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0,
	}}

	_, err := pageFaultCopyArgsForFault(regions, 0x100000)
	if !errors.Is(err, errInvalidGuestRegionPageSize) {
		t.Fatalf("pageFaultCopyArgsForFault() error = %v, want %v", err, errInvalidGuestRegionPageSize)
	}
}

func TestPageFaultCopyArgsForGuestOffset(t *testing.T) {
	regions := []GuestRegionUffdMapping{
		{
			BaseHostVirtAddr: 0x100000,
			Size:             0x2000,
			PageSize:         0x1000,
		},
		{
			BaseHostVirtAddr: 0x400000,
			Size:             0x3000,
			Offset:           0x8000,
			PageSize:         0x1000,
		},
	}

	got, err := pageFaultCopyArgsForGuestOffset(regions, 0x9000, 7)
	if err != nil {
		t.Fatalf("pageFaultCopyArgsForGuestOffset returned error: %v", err)
	}

	want := pageFaultCopyArgs{
		srcOffset: 0x9000,
		dstAddr:   0x401000,
		copyLen:   0x1000,
		copyMode:  7,
	}
	if got != want {
		t.Fatalf("pageFaultCopyArgsForGuestOffset() = %+v, want %+v", got, want)
	}
}

func TestPageFaultCopyArgsForGuestOffsetOutsideAllRegions(t *testing.T) {
	regions := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x2000,
		PageSize:         0x1000,
	}}

	_, err := pageFaultCopyArgsForGuestOffset(regions, 0x3000, 0)
	if !errors.Is(err, errGuestRegionNotFound) {
		t.Fatalf("pageFaultCopyArgsForGuestOffset() error = %v, want %v", err, errGuestRegionNotFound)
	}
}

func TestTraceProcessRecordPersistsWorkingSetAndTrace(t *testing.T) {
	baseDir := t.TempDir()
	guestMemPath := filepath.Join(baseDir, "guest_mem")
	workingSetPath := filepath.Join(baseDir, "working_set_pages")
	tracePath := filepath.Join(baseDir, "working_set_trace")
	pageSize := uint64(os.Getpagesize())

	prepareGuestMemoryFile(t, guestMemPath, 5*int(pageSize))

	trace := initTrace(tracePath)
	trace.AppendRecord(Record{offset: 3 * pageSize})
	trace.AppendRecord(Record{offset: pageSize})
	trace.AppendRecord(Record{offset: 2 * pageSize})
	trace.AppendRecord(Record{offset: 2 * pageSize})

	if err := trace.ProcessRecord(guestMemPath, workingSetPath, pageSize); err != nil {
		t.Fatalf("ProcessRecord returned error: %v", err)
	}

	if got, want := len(trace.trace), 3; got != want {
		t.Fatalf("trace length = %d, want %d", got, want)
	}
	if got, want := trace.regions[pageSize], 3; got != want {
		t.Fatalf("trace.regions[%#x] = %d, want %d", pageSize, got, want)
	}

	got, err := os.ReadFile(workingSetPath)
	if err != nil {
		t.Fatalf("os.ReadFile working set returned error: %v", err)
	}
	wantGuest, err := os.ReadFile(guestMemPath)
	if err != nil {
		t.Fatalf("os.ReadFile guest memory returned error: %v", err)
	}
	want := wantGuest[pageSize : 4*pageSize]
	if !reflect.DeepEqual(got, want) {
		t.Fatal("working set contents do not match recorded guest memory pages")
	}

	loadedTrace := initTrace(tracePath)
	if err := loadedTrace.readTrace(); err != nil {
		t.Fatalf("readTrace returned error: %v", err)
	}
	if got, want := loadedTrace.pageSize, pageSize; got != want {
		t.Fatalf("loaded trace page size = %#x, want %#x", got, want)
	}
	if !reflect.DeepEqual(loadedTrace.trace, trace.trace) {
		t.Fatal("loaded trace records do not match persisted records")
	}
	if !reflect.DeepEqual(loadedTrace.regions, trace.regions) {
		t.Fatal("loaded trace regions do not match persisted regions")
	}
}

func TestReceiveUffdMappingsAndFD(t *testing.T) {
	mappings := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x4000,
		Offset:           0x8000,
		PageSize:         0x1000,
	}}
	body, err := json.Marshal(mappings)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	sentFile := tempFileWithContent(t, "uffd fd payload")
	gotMappings, gotFile, err := receiveFromTestSocket(t, func(conn *net.UnixConn) error {
		return writeUffdSocketPayload(conn, body, sentFile)
	})
	if err != nil {
		t.Fatalf("receiveUffdMappingsAndFD returned error: %v", err)
	}
	defer func() { _ = gotFile.Close() }()

	if !reflect.DeepEqual(gotMappings, mappings) {
		t.Fatalf("receiveUffdMappingsAndFD mappings = %+v, want %+v", gotMappings, mappings)
	}

	gotPayload, err := io.ReadAll(gotFile)
	if err != nil {
		t.Fatalf("reading received fd returned error: %v", err)
	}
	if want := "uffd fd payload"; string(gotPayload) != want {
		t.Fatalf("received fd payload = %q, want %q", gotPayload, want)
	}
}

func TestReceiveUffdMappingsAndFDInvalidJSON(t *testing.T) {
	sentFile := tempFileWithContent(t, "unused")
	_, gotFile, err := receiveFromTestSocket(t, func(conn *net.UnixConn) error {
		return writeUffdSocketPayload(conn, []byte("{"), sentFile)
	})
	if gotFile != nil {
		_ = gotFile.Close()
	}
	if err == nil {
		t.Fatal("receiveUffdMappingsAndFD succeeded for invalid JSON")
	}
}

func TestReceiveUffdMappingsAndFDEmptyMappings(t *testing.T) {
	sentFile := tempFileWithContent(t, "unused")
	_, gotFile, err := receiveFromTestSocket(t, func(conn *net.UnixConn) error {
		return writeUffdSocketPayload(conn, []byte("[]"), sentFile)
	})
	if gotFile != nil {
		_ = gotFile.Close()
	}
	if !errors.Is(err, errNoGuestRegionMappings) {
		t.Fatalf("receiveUffdMappingsAndFD error = %v, want %v", err, errNoGuestRegionMappings)
	}
}

func TestReceiveUffdMappingsAndFDMissingFD(t *testing.T) {
	body := validMappingsJSON(t)
	_, gotFile, err := receiveFromTestSocket(t, func(conn *net.UnixConn) error {
		return writeUffdSocketPayload(conn, body)
	})
	if gotFile != nil {
		_ = gotFile.Close()
	}
	if !errors.Is(err, errUnexpectedUffdFDCount) {
		t.Fatalf("receiveUffdMappingsAndFD error = %v, want %v", err, errUnexpectedUffdFDCount)
	}
}

func TestReceiveUffdMappingsAndFDRejectsMultipleFDs(t *testing.T) {
	body := validMappingsJSON(t)
	firstFile := tempFileWithContent(t, "first")
	secondFile := tempFileWithContent(t, "second")

	_, gotFile, err := receiveFromTestSocket(t, func(conn *net.UnixConn) error {
		return writeUffdSocketPayload(conn, body, firstFile, secondFile)
	})
	if gotFile != nil {
		_ = gotFile.Close()
	}
	if !errors.Is(err, errUnexpectedUffdFDCount) {
		t.Fatalf("receiveUffdMappingsAndFD error = %v, want %v", err, errUnexpectedUffdFDCount)
	}
}

func TestSnapshotStateGetUFFDStoresMappingsAndFD(t *testing.T) {
	mappings := []GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x400000,
		Size:             0x2000,
		PageSize:         0x1000,
	}}
	body, err := json.Marshal(mappings)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}
	sentFile := tempFileWithContent(t, "state fd payload")

	socketPath := filepath.Join(t.TempDir(), "uffd.sock")
	state := &SnapshotState{
		SnapshotStateCfg: SnapshotStateCfg{
			InstanceSockAddr: socketPath,
		},
	}

	stateErrCh := make(chan error, 1)
	go func() {
		stateErrCh <- state.getUFFD(nil)
	}()

	conn := dialUnixSocketWithRetry(t, socketPath)
	if err := writeUffdSocketPayload(conn, body, sentFile); err != nil {
		_ = conn.Close()
		t.Fatalf("writeUffdSocketPayload returned error: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("conn.Close returned error: %v", err)
	}

	if err := <-stateErrCh; err != nil {
		t.Fatalf("getUFFD returned error: %v", err)
	}
	defer func() { _ = state.userFaultFD.Close() }()

	if !reflect.DeepEqual(state.guestRegionMappings, mappings) {
		t.Fatalf("guestRegionMappings = %+v, want %+v", state.guestRegionMappings, mappings)
	}
	gotPayload, err := io.ReadAll(state.userFaultFD)
	if err != nil {
		t.Fatalf("reading state.userFaultFD returned error: %v", err)
	}
	if want := "state fd payload"; string(gotPayload) != want {
		t.Fatalf("state.userFaultFD payload = %q, want %q", gotPayload, want)
	}
}

func dialUnixSocketWithRetry(t *testing.T, socketPath string) *net.UnixConn {
	t.Helper()

	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	deadline := time.Now().Add(time.Second)
	for {
		conn, err := net.DialUnix("unix", nil, addr)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("net.DialUnix(%q) timed out: %v", socketPath, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func receiveFromTestSocket(t *testing.T, send func(*net.UnixConn) error) ([]GuestRegionUffdMapping, *os.File, error) {
	t.Helper()

	socketPath := filepath.Join(t.TempDir(), "uffd.sock")
	addr := &net.UnixAddr{Name: socketPath, Net: "unix"}
	listener, err := net.ListenUnix("unix", addr)
	if err != nil {
		t.Fatalf("net.ListenUnix returned error: %v", err)
	}
	defer func() { _ = listener.Close() }()

	sendErrCh := make(chan error, 1)
	go func() {
		conn, err := net.DialUnix("unix", nil, addr)
		if err != nil {
			sendErrCh <- err
			return
		}
		defer func() { _ = conn.Close() }()
		sendErrCh <- send(conn)
	}()

	conn, err := listener.AcceptUnix()
	if err != nil {
		t.Fatalf("AcceptUnix returned error: %v", err)
	}
	defer func() { _ = conn.Close() }()

	mappings, file, receiveErr := receiveUffdMappingsAndFD(conn)
	if sendErr := <-sendErrCh; sendErr != nil {
		t.Fatalf("test sender returned error: %v", sendErr)
	}

	return mappings, file, receiveErr
}

func writeUffdSocketPayload(conn *net.UnixConn, body []byte, files ...*os.File) error {
	fds := make([]int, 0, len(files))
	for _, file := range files {
		fds = append(fds, int(file.Fd()))
	}

	var oob []byte
	if len(fds) > 0 {
		oob = unix.UnixRights(fds...)
	}

	n, _, err := conn.WriteMsgUnix(body, oob, nil)
	if err != nil {
		return err
	}
	if n != len(body) {
		return fmt.Errorf("sent %d bytes, want %d", n, len(body))
	}

	return nil
}

func validMappingsJSON(t *testing.T) []byte {
	t.Helper()

	body, err := json.Marshal([]GuestRegionUffdMapping{{
		BaseHostVirtAddr: 0x100000,
		Size:             0x1000,
		PageSize:         0x1000,
	}})
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	return body
}

func tempFileWithContent(t *testing.T, content string) *os.File {
	t.Helper()

	file, err := os.CreateTemp(t.TempDir(), "uffd-fd-*")
	if err != nil {
		t.Fatalf("os.CreateTemp returned error: %v", err)
	}
	t.Cleanup(func() { _ = file.Close() })

	if _, err := file.WriteString(content); err != nil {
		t.Fatalf("WriteString returned error: %v", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek returned error: %v", err)
	}

	return file
}
