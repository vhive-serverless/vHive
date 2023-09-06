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
	"bytes"
	"context"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/snapshots"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"strings"
	"sync"
)

// DeviceMapper creates and manages device snapshots used to store container images.
type DeviceMapper struct {
	sync.Mutex
	snapDevices     map[string]*DeviceSnapshot // maps revision snapkey to snapshot device
	snapshotService snapshots.Snapshotter      // used to interact with the device mapper through containerd

	// Manage leases to avoid garbage collection of manually created snapshots. Done automatically for snapshots
	// created directly through containerd (eg. container.create)
	leaseManager leases.Manager
	leases       map[string]*leases.Lease
}

func NewDeviceMapper(client *containerd.Client) *DeviceMapper {
	devMapper := new(DeviceMapper)
	devMapper.snapDevices = make(map[string]*DeviceSnapshot)
	devMapper.snapshotService = client.SnapshotService("devmapper")
	devMapper.leaseManager = client.LeasesService()
	devMapper.leases = make(map[string]*leases.Lease)
	return devMapper
}

// getImageKeys returns the key used in containerd to identify the snapshot of the given image
func getImageKey(image containerd.Image, ctx context.Context) (string, error) {
	diffIDs, err := image.RootFS(ctx)
	if err != nil {
		return "", err
	}
	return identity.ChainID(diffIDs).String(), nil
}

// CreateDeviceSnapshotFromImage creates a new device mapper snapshot based on the given image.
func (dmpr *DeviceMapper) CreateDeviceSnapshotFromImage(ctx context.Context, snapshotKey string, image containerd.Image) error {
	parent, err := getImageKey(image, ctx)
	if err != nil {
		return err
	}

	return dmpr.CreateDeviceSnapshot(ctx, snapshotKey, parent)
}

// CreateDeviceSnapshot creates a new device mapper snapshot from the given parent snapshot.
func (dmpr *DeviceMapper) CreateDeviceSnapshot(ctx context.Context, snapKey, parentKey string) error {
	// Create lease to avoid garbage collection
	lease, err := dmpr.leaseManager.Create(ctx, leases.WithID(snapKey))
	if err != nil {
		return err
	}

	// Create snapshot from parent
	leasedCtx := leases.WithLease(ctx, lease.ID)
	mounts, err := dmpr.snapshotService.Prepare(leasedCtx, snapKey, parentKey)
	if err != nil {
		return err
	}

	// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
	devSnapPath := mounts[0].Source

	dmpr.Lock()
	dsnp := NewDeviceSnapshot(devSnapPath)
	dmpr.snapDevices[snapKey] = dsnp
	dmpr.leases[snapKey] = &lease
	dmpr.Unlock()
	return nil
}

// RemoveDeviceSnapshot removes the device mapper snapshot identified by the given snapKey. This is only necessary for
// snapshots created through CreateDeviceSnapshot since other snapshots are managed by containerd. The locking here
// also assumes this function is only used to remove snapshots that are a child and are only used by a single container.
func (dmpr *DeviceMapper) RemoveDeviceSnapshot(ctx context.Context, snapKey string) error {
	dmpr.Lock()

	lease, present := dmpr.leases[snapKey]
	if !present {
		dmpr.Unlock()
		return errors.New(fmt.Sprintf("Delete device snapshot: lease for key %s does not exist", snapKey))
	}

	if _, present := dmpr.snapDevices[snapKey]; !present {
		dmpr.Unlock()
		return errors.New(fmt.Sprintf("Delete device snapshot: device for key %s does not exist", snapKey))
	}
	delete(dmpr.snapDevices, snapKey)
	delete(dmpr.leases, snapKey)
	dmpr.Unlock()

	// Not only deactivates but also deletes device
	err := dmpr.snapshotService.Remove(ctx, snapKey)
	if err != nil {
		return err
	}

	if err := dmpr.leaseManager.Delete(ctx, *lease); err != nil {
		return err
	}

	return nil
}

// GetImageSnapshot retrieves the device mapper snapshot for a given image.
func (dmpr *DeviceMapper) GetImageSnapshot(ctx context.Context, image containerd.Image) (*DeviceSnapshot, error) {
	imageSnapKey, err := getImageKey(image, ctx)
	if err != nil {
		return nil, err
	}

	return dmpr.GetDeviceSnapshot(ctx, imageSnapKey)
}

// GetDeviceSnapshot returns the device mapper snapshot identified by the given snapKey.
func (dmpr *DeviceMapper) GetDeviceSnapshot(ctx context.Context, snapKey string) (*DeviceSnapshot, error) {
	dmpr.Lock()
	defer dmpr.Unlock()

	_, present := dmpr.snapDevices[snapKey]
	if !present {
		// Get snapshot from containerd if not yet stored by vHive devicemapper
		mounts, err := dmpr.snapshotService.Mounts(ctx, snapKey)
		if err != nil {
			return nil, err
		}

		// Devmapper always only has a single mount /dev/mapper/fc-thinpool-snap-x
		devSnapPath := mounts[0].Source

		dsnp := NewDeviceSnapshot(devSnapPath)
		dmpr.snapDevices[snapKey] = dsnp
	}

	return dmpr.snapDevices[snapKey], nil
}

// addTrailingSlash adds a trailing slash to a path if it is not present yet.
func addTrailingSlash(path string) string {
	if strings.HasSuffix(path, "/") {
		return path
	} else {
		return path + "/"
	}
}

// CreatePatch creates a patch file storing the file differences between and image and the changes applied
// by the container using rsync. Note that this is a different approach than using thin_delta which is able to
// extract blocks directly by leveraging the metadata stored by the device mapper.
func (dmpr *DeviceMapper) CreatePatch(ctx context.Context, patchPath, containerSnapKey string, image containerd.Image) error {
	containerSnap, err := dmpr.GetDeviceSnapshot(ctx, containerSnapKey)
	if err != nil {
		return err
	}

	imageSnap, err := dmpr.GetImageSnapshot(ctx, image)
	if err != nil {
		return err
	}

	// 1. Mount original and snapshot image
	imageMountPath, err := imageSnap.Mount(true)
	if err != nil {
		return err
	}
	defer func() { _ = imageSnap.UnMount() }()

	containerMountPath, err := containerSnap.Mount(true)
	if err != nil {
		return err
	}
	defer func() { _ = containerSnap.UnMount() }()

	// 2. Save changes to file
	result := extractPatch(imageMountPath, containerMountPath, patchPath)

	// 3. Change the rights of patch file to enable upload to local storage
	_ = exec.Command("sudo", "chmod", "777", patchPath)

	return result
}

// extractPatch extracts the file differences between the file systems mounted at the supplied paths using rsync and
// writes the differences to the supplied patchPath.
func extractPatch(imageMountPath, containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--only-write-batch=%s", patchPath)

	var errb bytes.Buffer
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(imageMountPath), addTrailingSlash(containerMountPath))
	cmd.Stderr = &errb
	err := cmd.Run()

	if err != nil {
		return errors.Wrapf(err, "creating patch between %s and %s at %s: %s", imageMountPath, containerMountPath, patchPath, errb.String())
	}

	err = os.Remove(patchPath + ".sh") // Remove unnecessary script output
	if err != nil {
		return errors.Wrapf(err, "removing %s", patchPath+".sh")
	}
	return nil
}

// RestorePatch applies the file changes stored in the supplied patch file on top of the given container snapshot.
func (dmpr *DeviceMapper) RestorePatch(ctx context.Context, containerSnapKey, patchPath string) error {
	containerSnap, err := dmpr.GetDeviceSnapshot(ctx, containerSnapKey)
	if err != nil {
		return err
	}

	// 1. Mount container snapshot device
	containerMountPath, err := containerSnap.Mount(false)
	if err != nil {
		return err
	}
	defer func() { _ = containerSnap.UnMount() }()

	// 2. Apply changes to container mounted file system
	return applyPatch(containerMountPath, patchPath)
}

// applyPatch applies the file changes stored in the supplied patch file to the filesystem mounted at the supplied path
func applyPatch(containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--read-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(containerMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "applying %s at %s", patchPath, containerMountPath)
	}
	return nil
}
