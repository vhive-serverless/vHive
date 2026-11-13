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
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

func blockSize(ctx context.Context, path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeDevice == 0 || info.Mode()&os.ModeCharDevice != 0 {
		return 0, fmt.Errorf("%s is not a block device", path)
	}
	out, err := exec.CommandContext(ctx, "blockdev", "--getsize64", path).Output()
	if err != nil {
		return 0, fmt.Errorf("query block device %s: %w", path, err)
	}
	return strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
}

func copyBlockImage(ctx context.Context, src, dst, conv string) error {
	out, err := exec.CommandContext(ctx, "dd", "if="+src, "of="+dst, "bs=4M", "conv="+conv, "status=none").CombinedOutput()
	if err != nil {
		return fmt.Errorf("copy %s to %s: %w: %s", src, dst, err, out)
	}
	return nil
}

// SaveBlockImage captures the exact ext4 image while the source VM is paused.
// Replaying files would not reproduce the journal and metadata in guest memory.
func (dmpr *DeviceMapper) SaveBlockImage(ctx context.Context, key, path string) error {
	mounts, err := dmpr.snapshotService.Mounts(ctx, key)
	if err != nil {
		return err
	}
	if len(mounts) != 1 {
		return fmt.Errorf("expected one block device for %s, got %d", key, len(mounts))
	}
	device := mounts[0].Source
	size, err := blockSize(ctx, device)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".disk-")
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := copyBlockImage(ctx, device, tmp.Name(), "sparse,fsync"); err != nil {
		return err
	}
	info, err := os.Stat(tmp.Name())
	if err != nil {
		return err
	}
	if info.Size() != size {
		return fmt.Errorf("incomplete block image: got %d bytes, want %d", info.Size(), size)
	}
	return os.Rename(tmp.Name(), path)
}

// RestoreBlockImage replaces all blocks of an independently allocated device.
func (dmpr *DeviceMapper) RestoreBlockImage(ctx context.Context, key, path string) error {
	snap, err := dmpr.GetDeviceSnapshot(ctx, key)
	if err != nil {
		return err
	}
	device := snap.GetDevicePath()
	size, err := blockSize(ctx, device)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() != size {
		return fmt.Errorf("expected a regular %d-byte disk image at %s", size, path)
	}
	return copyBlockImage(ctx, path, device, "notrunc,fsync")
}
