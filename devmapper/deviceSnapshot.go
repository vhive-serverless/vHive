// MIT License
//
// Copyright (c) 2021 Amory Hoste and EASE lab
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
	"bytes"
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
)

// DeviceSnapshot represents a device mapper snapshot
type DeviceSnapshot struct {
	sync.Mutex
	poolName           string
	deviceName         string
	deviceId           string
	mountDir           string
	mountedReadonly    bool
	numMounted         int
	numActivated       int
}

// GetDevicePath returns the path to the snapshot device.
func (dsnp *DeviceSnapshot) GetDevicePath() string {
	return fmt.Sprintf("/dev/mapper/%s", dsnp.deviceName)
}

// getPoolpath returns the path of the thin pool used by the snapshot.
func (dsnp *DeviceSnapshot) getPoolPath() string {
	return fmt.Sprintf("/dev/mapper/%s", dsnp.poolName)
}

// NewDeviceSnapshot initializes a new device mapper snapshot.
func NewDeviceSnapshot(poolName, deviceName, deviceId string) *DeviceSnapshot {
	dsnp := new(DeviceSnapshot)
	dsnp.poolName = poolName
	dsnp.deviceName = deviceName
	dsnp.deviceId = deviceId
	dsnp.mountDir = ""
	dsnp.mountedReadonly = false
	dsnp.numMounted = 0
	dsnp.numActivated = 0
	return dsnp
}

// Activate creates a snapshot.
func (dsnp *DeviceSnapshot) Activate() error {
	dsnp.Lock()
	defer dsnp.Unlock()

	if dsnp.numActivated == 0 {
		tableEntry := fmt.Sprintf("0 20971520 thin %s %s", dsnp.getPoolPath(), dsnp.deviceId)

		var errb bytes.Buffer
		cmd := exec.Command("sudo", "dmsetup", "create", dsnp.deviceName, "--table", tableEntry)
		cmd.Stderr = &errb
		err := cmd.Run()

		if err != nil {
			return errors.Wrapf(err, "activating snapshot %s: %s", dsnp.deviceName, errb.String())
		}
	}

	dsnp.numActivated += 1

	return nil
}

// Deactivate removes a snapshot.
func (dsnp *DeviceSnapshot) Deactivate() error {
	dsnp.Lock()
	defer dsnp.Unlock()

	if dsnp.numActivated == 1 {
		cmd := exec.Command("sudo", "dmsetup", "remove", dsnp.deviceName)
		err := cmd.Run()
		if err != nil {
			return errors.Wrapf(err, "deactivating snapshot %s", dsnp.deviceName)
		}
	}

	dsnp.numActivated -= 1
	return nil
}

// Mount mounts a snapshot device and returns the path where it is mounted. For better performance and efficiency,
// a snapshot is only mounted once and shared if it is already mounted.
func (dsnp *DeviceSnapshot) Mount(readOnly bool) (string, error) {
	dsnp.Lock()
	defer dsnp.Unlock()

	if dsnp.numActivated == 0 {
		return "", errors.New("failed to mount: snapshot not activated")
	}

	if dsnp.numMounted != 0 && (!dsnp.mountedReadonly || dsnp.mountedReadonly && !readOnly) {
		return "", errors.New("failed to mount: can't mount snapshot for both reading and writing")
	}

	if dsnp.numMounted == 0 {
		mountDir, err := ioutil.TempDir("", dsnp.deviceName)
		if err != nil {
			return "", err
		}
		mountDir = removeTrailingSlash(mountDir)

		err = mountExt4(dsnp.GetDevicePath(), mountDir, readOnly)
		if err != nil {
			return "", errors.Wrapf(err, "mounting %s at %s", dsnp.GetDevicePath(), mountDir)
		}
		dsnp.mountDir = mountDir
		dsnp.mountedReadonly = readOnly
	}

	dsnp.numMounted += 1

	return dsnp.mountDir, nil
}

// UnMounts a device snapshot. Due to mounted snapshot being shared, a snapshot is only actually unmounted if it is not
// in use by anyone else.
func (dsnp *DeviceSnapshot) UnMount() error {
	dsnp.Lock()
	defer dsnp.Unlock()

	if dsnp.numMounted == 1 {
		err := unMountExt4(dsnp.mountDir)
		if err != nil {
			return errors.Wrapf(err, "unmounting %s", dsnp.mountDir)
		}

		err = os.RemoveAll(dsnp.mountDir)
		if err != nil {
			return errors.Wrapf(err, "removing %s", dsnp.mountDir)
		}
		dsnp.mountDir = ""
	}

	dsnp.numMounted -= 1
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