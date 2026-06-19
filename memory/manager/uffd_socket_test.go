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

	"golang.org/x/sys/unix"
)

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
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("net.ListenUnix returned error: %v", err)
	}
	defer func() { _ = listener.Close() }()

	serverErrCh := make(chan error, 1)
	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.Close() }()
		serverErrCh <- writeUffdSocketPayload(conn, body, sentFile)
	}()

	state := &SnapshotState{
		SnapshotStateCfg: SnapshotStateCfg{
			InstanceSockAddr: socketPath,
		},
	}
	if err := state.getUFFD(); err != nil {
		t.Fatalf("getUFFD returned error: %v", err)
	}
	defer func() { _ = state.userFaultFD.Close() }()

	if err := <-serverErrCh; err != nil {
		t.Fatalf("test server returned error: %v", err)
	}
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
