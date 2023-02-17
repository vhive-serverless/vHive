package manual_reload

import (
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func getPoolPath(poolName string) string {
	return fmt.Sprintf("/dev/mapper/%s", poolName)
}

func activateSnapshot(snapDeviceName, snapshotId, poolName string) error {
	tableEntry := fmt.Sprintf("0 20971520 thin %s %s", getPoolPath(poolName), snapshotId)

	cmd := exec.Command("sudo", "dmsetup", "create", snapDeviceName, "--table", fmt.Sprintf("%s", tableEntry))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "activating snapshot %s", snapDeviceName)
	}
	return nil
}

func deactivateSnapshot(snapDeviceName string) error {
	cmd := exec.Command("sudo", "dmsetup", "remove", snapDeviceName)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "deactivating snapshot %s", snapDeviceName)
	}
	return nil
}

func mountSnapshot(snapDeviceName, snapDevicePath string, readOnly bool) (string, error) {
	mountDir, err := ioutil.TempDir("", snapDeviceName)
	if err != nil {
		return "", err
	}
	mountDir = removeTrailingSlash(mountDir)

	err = mountExt4(snapDevicePath, mountDir, readOnly)
	if err != nil {
		return "", errors.Wrapf(err, "mounting %s at %s", snapDevicePath, mountDir)
	}
	return mountDir, nil
}

func suspendSnapshot(snapDeviceName string) error {
	cmd := exec.Command("sudo", "dmsetup", "suspend", snapDeviceName)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "suspending snapshot %s", snapDeviceName)
	}
	return nil
}

func resumeSnapshot(snapDeviceName string) error {
	cmd := exec.Command("sudo", "dmsetup", "resume", snapDeviceName)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "suspending snapshot %s", snapDeviceName)
	}
	return nil
}

func mountExt4(devicePath, mountPath string, readOnly bool) error {
	// Do not update access times for (all types of) files on this filesystem.
	// Do not allow access to devices (special files) on this filesystem.
	// Do not allow programs to be executed from this filesystem.
	// Do not honor set-user-ID and set-group-ID bits or file  capabilities when executing programs from this filesystem.
	// Suppress the display of certain (printk()) warning messages in the kernel log.
	var flags uintptr = syscall.MS_NOATIME | syscall.MS_NODEV | syscall.MS_NOEXEC | syscall.MS_NOSUID | syscall.MS_SILENT
	options := make([]string, 0)

	if readOnly {
		// Mount filesystem read-only.
		flags |= syscall.MS_RDONLY
		options = append(options, "noload")
	}

	return syscall.Mount(devicePath, mountPath, "ext4", flags, strings.Join(options, ","))
}

func unMountExt4(mountPath string) error {
	return syscall.Unmount(mountPath, syscall.MNT_DETACH)
}

func unMountSnapshot(mountPath string) error {
	err := unMountExt4(mountPath)
	if err != nil {
		return errors.Wrapf(err, "unmounting %s", mountPath)
	}

	err = os.RemoveAll(mountPath)
	if err != nil {
		return errors.Wrapf(err, "removing %s", mountPath)
	}
	return nil
}

func addTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	} else {
		return path + "/"
	}
}

func removeTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path[:len(path)-1]
	} else {
		return path
	}
}

func createPatch(imageMountPath, containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--only-write-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(imageMountPath), addTrailingSlash(containerMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "creating patch between %s and %s at %s", imageMountPath, containerMountPath, patchPath)
	}

	err = os.Remove(patchPath + ".sh") // Remove unnecessary script output
	if err != nil {
		return errors.Wrapf(err, "removing %s", patchPath+".sh")
	}
	return nil
}

func applyPatch(containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--read-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(containerMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "applying %s at %s", patchPath, containerMountPath)
	}
	return nil
}

func digHoles(filePath string) error {
	cmd := exec.Command("sudo", "fallocate", "--dig-holes", filePath)
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "digging holes in %s", filePath)
	}
	return nil
}
