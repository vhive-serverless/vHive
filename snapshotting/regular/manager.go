// MIT License
//
// Copyright (c) 2021 Amory Hoste, Plamen Petrov and EASE lab
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

package regular

import (
	"fmt"
	"github.com/ease-lab/vhive/snapshotting"
	"github.com/pkg/errors"
	"os"
	"sync"
)


// ImprovedSnapshotManager manages snapshots stored on the node.
type RegularSnapshotManager struct {
	sync.Mutex
	activeSnapshots map[string]*snapshotting.Snapshot
	creatingSnapshots map[string]*snapshotting.Snapshot
	idleSnapshots   map[string][]*snapshotting.Snapshot
	baseFolder      string
}

func NewRegularSnapshotManager(baseFolder string) *RegularSnapshotManager {
	manager := new(RegularSnapshotManager)
	manager.activeSnapshots = make(map[string]*snapshotting.Snapshot)
	manager.creatingSnapshots = make(map[string]*snapshotting.Snapshot)
	manager.idleSnapshots = make(map[string][]*snapshotting.Snapshot)
	manager.baseFolder = baseFolder

	// Clean & init basefolder
	os.RemoveAll(manager.baseFolder)
	os.MkdirAll(manager.baseFolder, os.ModePerm)

	return manager
}

func (mgr *RegularSnapshotManager) AcquireSnapshot(image string) (*snapshotting.Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	idles, ok := mgr.idleSnapshots[image]
	if !ok {
		mgr.idleSnapshots[image] = []*snapshotting.Snapshot{}
		return nil, errors.New(fmt.Sprintf("There is no snapshot available for image %s", image))
	}

	if len(idles) != 0 {
		snp := idles[0]
		mgr.idleSnapshots[image] = idles[1:]
		mgr.activeSnapshots[snp.GetId()] = snp
		return snp, nil
	}

	return nil, errors.New(fmt.Sprintf("There is no snapshot available fo rimage %s", image))
}

func (mgr *RegularSnapshotManager) ReleaseSnapshot(vmID string) error {
	mgr.Lock()
	defer mgr.Unlock()

	snap, present := mgr.activeSnapshots[vmID]
	if !present {
		return errors.New(fmt.Sprintf("Get: Snapshot for container %s does not exist", vmID))
	}

	delete(mgr.activeSnapshots, vmID)
	mgr.idleSnapshots[snap.Image] = append(mgr.idleSnapshots[snap.Image], snap)

	return nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the ImprovedSnapshotManager. Once the snapshot has been created,
// CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available fo ruse
func (mgr *RegularSnapshotManager) InitSnapshot(vmID, image string, coldStartTimeMs int64, memSizeMib, vCPUCount uint32, sparse bool) (*[]string, *snapshotting.Snapshot, error) {
	mgr.Lock()
	var removeContainerSnaps *[]string

	// Add snapshot and snapshot metadata to manager
	snap := snapshotting.NewSnapshot(vmID, mgr.baseFolder, image, memSizeMib, vCPUCount, sparse)
	mgr.creatingSnapshots[snap.GetId()] = snap
	mgr.Unlock()

	// Create directory to store snapshot data
	err := snap.CreateSnapDir()
	if err != nil {
		return removeContainerSnaps, nil, errors.Wrapf(err, "creating snapDir for snapshots %s", vmID)
	}

	return removeContainerSnaps, snap, nil
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *RegularSnapshotManager) CommitSnapshot(vmID string) error {
	mgr.Lock()
	defer mgr.Unlock()
	snap := mgr.creatingSnapshots[vmID]
	delete(mgr.creatingSnapshots, vmID)

	_, ok := mgr.idleSnapshots[snap.Image]
	if !ok {
		mgr.idleSnapshots[snap.Image] = []*snapshotting.Snapshot{}
	}

	mgr.idleSnapshots[snap.Image] = append(mgr.idleSnapshots[snap.Image], snap)

	return nil
}
