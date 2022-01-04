package devmapper

import (
	"context"
	"fmt"
	"github.com/containerd/containerd"
	"github.com/containerd/containerd/leases"
	"github.com/containerd/containerd/snapshots"
	"github.com/ease-lab/vhive/devmapper/thindelta"
	"github.com/opencontainers/image-spec/identity"
	"github.com/pkg/errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// DeviceMapper creates and manages device snapshots used to store container images.
type DeviceMapper struct {
	sync.Mutex
	poolName           string
	snapDevices        map[string]*DeviceSnapshot   // maps revision snapkey to snapshot device
	snapshotService    snapshots.Snapshotter        // used to interact with the device mapper through containerd
	thinDelta          *thindelta.ThinDelta

	// Manage leases to avoid garbage collection of manually created snapshots. Done automatically for snapshots
	// created directly through containerd (eg. container.create)
	leaseManager      leases.Manager
	leases            map[string]*leases.Lease
}

func NewDeviceMapper(client *containerd.Client, poolName, metadataDev string) *DeviceMapper {
	devMapper := new(DeviceMapper)
	devMapper.poolName = poolName
	devMapper.thinDelta = thindelta.NewThinDelta(poolName, metadataDev)
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

	// Retrieve snapshot info
	deviceName := filepath.Base(mounts[0].Source)
	info, err := dmpr.snapshotService.Stat(ctx, snapKey)
	if err != nil {
		return err
	}

	dmpr.Lock()
	dsnp := NewDeviceSnapshot(dmpr.poolName, deviceName, info.SnapshotDev)
	dsnp.numActivated = 1 // Newly created snapshots through containerd are always activated
	dmpr.snapDevices[snapKey] = dsnp
	dmpr.leases[snapKey] = &lease
	dmpr.Unlock()
	return nil
}

// CommitDeviceSnapshot commits the changes made on a newly created snapshot (see containerd docs).
func (dmpr *DeviceMapper) CommitDeviceSnapshot(ctx context.Context, snapName, snapKey string) error {
	lease := dmpr.leases[snapKey]
	leasedCtx := leases.WithLease(ctx, lease.ID)

	if err := dmpr.snapshotService.Commit(leasedCtx, snapName, snapKey); err != nil {
		return err
	}

	dmpr.Lock()
	dmpr.snapDevices[snapKey].numActivated = 0
	dmpr.Unlock()
	return nil
}

// RemoveDeviceSnapshot removes the device mapper snapshot identified by the given snapKey. This is only necessary for
// snapshots created through CreateDeviceSnapshot since other snapshots are managed by containerd. The locking here
// also assumes this function is only used to remove snapshots that are a child and are only used by a single container.
func (dmpr *DeviceMapper) RemoveDeviceSnapshot(ctx context.Context, snapKey string) error {
	dmpr.Lock()

	lease, present := dmpr.leases[snapKey]
	if ! present {
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
		info, err := dmpr.snapshotService.Stat(ctx, snapKey)
		if err != nil {
			return nil, err
		}
		deviceName := getDeviceName(dmpr.poolName, info.SnapshotId)

		dsnp := NewDeviceSnapshot(dmpr.poolName, deviceName, info.SnapshotDev)
		if _, err := os.Stat(dsnp.GetDevicePath()); err == nil {
			// Snapshot already activated
			dsnp.numActivated = 1
		}

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

// getDeviceName returns the device name of a snapshot with the specified id made on the given poolName
func getDeviceName(poolName, snapshotId string) string {
	return fmt.Sprintf("%s-snap-%s", poolName, snapshotId)
}

// CreatePatch creates a patch file storing the difference between an image and the container filesystem
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

	// 1. Activate image snapshot
	err = imageSnap.Activate()
	if err != nil {
		return errors.Wrapf(err, "failed to activate image snapshot")
	}
	defer imageSnap.Deactivate()

	// 2. Mount original and snapshot image
	imageMountPath, err := imageSnap.Mount(true)
	if err != nil {
		return err
	}
	defer imageSnap.UnMount()

	containerMountPath, err := containerSnap.Mount(true)
	if err != nil {
		return err
	}
	defer containerSnap.UnMount()

	// 3. Save changes to file
	result := extractPatch(imageMountPath, containerMountPath, patchPath)

	return result
}

// extractPatch extracts the file differences between the file systems mounted at the supplied paths using rsync and
// writes the differences to the supplied patchPath.
func extractPatch(imageMountPath, containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--only-write-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(imageMountPath), addTrailingSlash(containerMountPath))
	err := cmd.Run()
	if err != nil {
		return errors.Wrapf(err, "creating patch between %s and %s at %s", imageMountPath, containerMountPath, patchPath)
	}

	err = os.Remove(patchPath + ".sh") // Remove unnecessary script output
	if err!= nil {
		return errors.Wrapf(err, "removing %s", patchPath + ".sh")
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
	defer containerSnap.UnMount()

	// 2. Apply changes to container mounted file system
	return applyPatch(containerMountPath, patchPath)
}

// applyPatch applies the file changes stored in the supplied patch file to the filesystem mounted at the supplied path
func applyPatch(containerMountPath, patchPath string) error {
	patchArg := fmt.Sprintf("--read-batch=%s", patchPath)
	cmd := exec.Command("sudo", "rsync", "-ar", patchArg, addTrailingSlash(containerMountPath))
	err := cmd.Run()
	if err!= nil {
		return errors.Wrapf(err, "applying %s at %s", patchPath, containerMountPath)
	}
	return nil
}

/****************************************************************************************
 * Below functions are legacy but useful for a first implementation of remote snapshotting.
 * They are not in use anymore due to thin_delta only supporting one metadata snapshot at
 * a time, which reduces the amount of snapshots we can make concurrently. Although this
 * might be easy to fix.
 ****************************************************************************************/

// ForkContainerSnap duplicates the snapshot with key oldContainerSnapKey into a new snapshot with name
// newContainerSnapName which can be used to boot a new container. The new snapshot is created by extracting the
// changes applied by oldContainerSnap on top of the image using thin_delta and writing these on a new snapshot created
// from the same image.
func (dmpr *DeviceMapper) ForkContainerSnap(ctx context.Context, oldContainerSnapKey, newContainerSnapName string, image containerd.Image) error {
	oldContainerSnap, err := dmpr.GetDeviceSnapshot(ctx, oldContainerSnapKey)
	if err != nil {
		return err
	}

	imageSnap, err := dmpr.GetImageSnapshot(ctx, image)
	if err != nil {
		return err
	}

	// 1. Get block difference of the old container snapshot from thinpool metadata
	blockDelta, err := dmpr.thinDelta.GetBlocksDelta(imageSnap.deviceId, oldContainerSnap.deviceId)
	if err != nil {
		return errors.Wrapf(err, "getting block delta")
	}

	// 2. Read the calculated block difference from the old container snapshot
	if err := blockDelta.ReadBlocks(oldContainerSnap.GetDevicePath()); err != nil {
		return errors.Wrapf(err, "reading block delta")
	}

	// 3. Create the new container snapshot
	newContainerSnapKey := newContainerSnapName + "active"
	if err := dmpr.CreateDeviceSnapshotFromImage(ctx, newContainerSnapKey, image); err != nil {
		return errors.Wrapf(err, "creating forked container snapshot")
	}
	newContainerSnap, err := dmpr.GetDeviceSnapshot(ctx, newContainerSnapKey)
	if err != nil {
		return errors.Wrapf(err, "previously created forked container device does not exist")
	}

	// 4. Write calculated block difference to new container snapshot
	if err := blockDelta.WriteBlocks(newContainerSnap.GetDevicePath()); err != nil {
		return errors.Wrapf(err, "writing block delta")
	}

	// 5. Commit the new container snapshot
	if err := dmpr.CommitDeviceSnapshot(ctx, newContainerSnapName, newContainerSnapKey); err != nil {
		return errors.Wrapf(err, "committing container snapshot")
	}

	return nil
}

