// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev, Amory Hoste and vHive team
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

package devmapper

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"github.com/pkg/errors"
)

// DeviceSnapshot represents a device mapper snapshot
type DeviceSnapshot struct {
	sync.Mutex
	path            string
	mountDir        string
	mountedReadonly bool
}

// NewDeviceSnapshot initializes a new device mapper snapshot.
func NewDeviceSnapshot(path string) *DeviceSnapshot {
	dsnp := new(DeviceSnapshot)
	dsnp.path = path
	dsnp.mountDir = ""
	dsnp.mountedReadonly = false
	return dsnp
}

// Mount a snapshot device and returns the path where it is mounted. For better performance and efficiency,
// a snapshot is only mounted once and shared if it is already mounted.
func (dsnp *DeviceSnapshot) Mount(readOnly bool) (string, error) {
	dsnp.Lock()
	defer dsnp.Unlock()

	mountDir, err := os.MkdirTemp("", filepath.Base(dsnp.path))
	if err != nil {
		return "", err
	}
	mountDir = removeTrailingSlash(mountDir)

	err = mountExt4(dsnp.path, mountDir, readOnly)
	if err != nil {
		return "", errors.Wrapf(err, "mounting %s at %s", dsnp.path, mountDir)
	}
	dsnp.mountDir = mountDir
	dsnp.mountedReadonly = readOnly

	return dsnp.mountDir, nil
}

// UnMount a device snapshot. Due to mounted snapshot being shared, a snapshot is only actually unmounted if it is not
// in use by anyone else.
func (dsnp *DeviceSnapshot) UnMount() error {
	dsnp.Lock()
	defer dsnp.Unlock()

	err := unMountExt4(dsnp.mountDir)
	if err != nil {
		return errors.Wrapf(err, "unmounting %s", dsnp.mountDir)
	}

	err = os.RemoveAll(dsnp.mountDir)
	if err != nil {
		return errors.Wrapf(err, "removing %s", dsnp.mountDir)
	}
	dsnp.mountDir = ""

	return nil
}

// mountExt4 mounts a snapshot device available at devicePath at the specified mountPath.
func mountExt4(devicePath, mountPath string, readOnly bool) error {
	// Specify flags for faster mounting and performance:
	// * Do not update access times for (all types of) files on this filesystem.
	// * Do not allow access to devices (special files) on this filesystem.
	// * Do not allow programs to be executed from this filesystem.
	// * Do not honor set-user-ID and set-group-ID bits or file  capabilities when executing programs from this filesystem.
	// * Suppress the display of certain (printk()) warning messages in the kernel log.
	var flags uintptr = syscall.MS_NOATIME | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_SILENT
	options := make([]string, 0)

	if readOnly {
		// Mount filesystem read-only.
		flags |= syscall.MS_RDONLY
		options = append(options, "noload")
	}

	return syscall.Mount(devicePath, mountPath, "ext4", flags, strings.Join(options, ","))
}

// unMountExt4 unmounts a snapshot device mounted at mountPath.
func unMountExt4(mountPath string) error {
	return syscall.Unmount(mountPath, syscall.MNT_DETACH)
}

// removeTrailingSlash returns a path with the trailing slash removed.
func removeTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path[:len(path)-1]
	} else {
		return path
	}
}

func (dsnp *DeviceSnapshot) GetDevicePath() string {
	return dsnp.path
}
