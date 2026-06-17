package manager

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const (
	uffdSocketPayloadSize = 64 * 1024
	uffdSocketFDLimit     = 2
	uffdSocketReadTimeout = time.Second
)

var (
	errUnexpectedUffdFDCount = errors.New("expected exactly one uffd fd")
	errNoGuestRegionMappings = errors.New("no guest region mappings received")
)

func receiveUffdMappingsAndFD(conn *net.UnixConn) ([]GuestRegionUffdMapping, *os.File, error) {
	if err := conn.SetReadDeadline(time.Now().Add(uffdSocketReadTimeout)); err != nil {
		return nil, nil, err
	}
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	body := make([]byte, uffdSocketPayloadSize)
	oob := make([]byte, unix.CmsgSpace(uffdSocketFDLimit*4))

	n, oobn, flags, _, err := conn.ReadMsgUnix(body, oob)
	if err != nil {
		return nil, nil, err
	}
	if flags&unix.MSG_TRUNC != 0 {
		return nil, nil, errors.New("uffd mappings payload was truncated")
	}
	if flags&unix.MSG_CTRUNC != 0 {
		return nil, nil, errors.New("uffd fd control message was truncated")
	}

	fds, err := parseUnixRights(oob[:oobn])
	if err != nil {
		return nil, nil, err
	}
	if len(fds) != 1 {
		closeFDs(fds)
		return nil, nil, fmt.Errorf("%w: got %d", errUnexpectedUffdFDCount, len(fds))
	}

	uffdFile := os.NewFile(uintptr(fds[0]), "userfaultfd")
	if uffdFile == nil {
		return nil, nil, errors.New("failed to create file for uffd fd")
	}

	var mappings []GuestRegionUffdMapping
	if err := json.Unmarshal(body[:n], &mappings); err != nil {
		_ = uffdFile.Close()
		return nil, nil, fmt.Errorf("cannot deserialize memory mappings: %w", err)
	}
	if len(mappings) == 0 {
		_ = uffdFile.Close()
		return nil, nil, errNoGuestRegionMappings
	}

	return mappings, uffdFile, nil
}

func parseUnixRights(oob []byte) ([]int, error) {
	scms, err := unix.ParseSocketControlMessage(oob)
	if err != nil {
		return nil, err
	}

	var fds []int
	for i := range scms {
		rights, err := unix.ParseUnixRights(&scms[i])
		if err != nil {
			return nil, err
		}
		fds = append(fds, rights...)
	}

	return fds, nil
}

func closeFDs(fds []int) {
	for _, receivedFD := range fds {
		_ = unix.Close(receivedFD)
	}
}
