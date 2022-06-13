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

package fulllocal

import (
	"container/heap"
	"fmt"
	"github.com/ease-lab/vhive/snapshotting"
	"github.com/pkg/errors"
	"math"
	"os"
	"sync"
)

// FullLocalSnapshotManager manages snapshots stored on the node.
type FullLocalSnapshotManager struct {
	sync.Mutex
	baseFolder string

	// Stored snapshots
	snapshots          map[string]*snapshotting.Snapshot
	// Eviction metadata for stored snapshots
	snapStats          map[string]*SnapshotStats

	// Heap of snapshots not in use that can be freed to save space. Sorted by score
	freeSnaps  SnapHeap

	// Eviction
	clock       int64 	// When container last used. Increased to priority terminated container on termination
	capacityMib int64
	usedMib     int64
}

func NewSnapshotManager(baseFolder string, capacityMib int64) *FullLocalSnapshotManager {
	manager := new(FullLocalSnapshotManager)
	manager.snapshots = make(map[string]*snapshotting.Snapshot)
	manager.snapStats = make(map[string]*SnapshotStats)
	heap.Init(&manager.freeSnaps)
	manager.baseFolder = baseFolder
	manager.clock = 0
	manager.capacityMib = capacityMib
	manager.usedMib = 0

	// Clean & init basefolder
	_ = os.RemoveAll(manager.baseFolder)
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)

	return manager
}

// AcquireSnapshot returns a snapshot for the specified revision if it is available and increments the internal counter
// such that the snapshot can't get removed. Similar to how a RW lock works
func (mgr *FullLocalSnapshotManager) AcquireSnapshot(revision string) (*snapshotting.Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	// Check if a snapshot is available for the specified revision
	snapStat, present := mgr.snapStats[revision]
	if !present {
		return nil, errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}

	// Snapshot registered in manager but creation not finished yet
	if ! snapStat.usable { // Could also wait until snapshot usable (trade-off)
		return nil, errors.New("Snapshot is not yet usable")
	}

	if snapStat.numUsing == 0 {
		// Remove from free snaps so can't be deleted (could be done more efficiently)
		heapIdx := 0
		for i, heapSnap := range mgr.freeSnaps {
			if heapSnap.revisionId == revision {
				heapIdx = i
				break
			}
		}
		heap.Remove(&mgr.freeSnaps, heapIdx)
	}

	snapStat.numUsing += 1

	// Update stats for keepalive policy
	snapStat.freq += 1
	snapStat.lastUsedClock = mgr.clock

	return mgr.snapshots[revision], nil
}

// ReleaseSnapshot releases the snapshot with the given revision so that it can possibly get deleted if it is not in use
// by any other VMs.
func (mgr *FullLocalSnapshotManager) ReleaseSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	snapStat, present := mgr.snapStats[revision]
	if !present {
		return errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}

	if snapStat.numUsing == 0 {
		return errors.New("Can't release a snapshot that is not in use")
	}

	snapStat.numUsing -= 1

	if snapStat.numUsing == 0 {
		// Add to free snaps
		snapStat.UpdateScore()
		heap.Push(&mgr.freeSnaps, snapStat)
	}

	return nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the FullLocalSnapshotManager. Once the snapshot has been created,
// CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available fo ruse
func (mgr *FullLocalSnapshotManager) InitSnapshot(revision, image string, coldStartTimeMs int64, memSizeMib, vCPUCount uint32, sparse bool) (*[]string, *snapshotting.Snapshot, error) {
	mgr.Lock()

	if _, present := mgr.snapshots[revision]; present {
		mgr.Unlock()
		return nil, nil, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists", revision))
	}

	var removeContainerSnaps *[]string

	// Calculate an estimate of the snapshot size
	estimatedSnapSizeMibf := float64(memSizeMib) * 1.25
	var estimatedSnapSizeMib = int64(math.Ceil(estimatedSnapSizeMibf))

	// Ensure enough space is available for snapshot to be created
	availableMib := mgr.capacityMib - mgr.usedMib
	if estimatedSnapSizeMib > availableMib {
		var err error
		spaceNeeded := estimatedSnapSizeMib - availableMib
		removeContainerSnaps, err = mgr.freeSpace(spaceNeeded)
		if err != nil {
			mgr.Unlock()
			return removeContainerSnaps, nil, err
		}
	}
	mgr.usedMib += estimatedSnapSizeMib

	// Add snapshot and snapshot metadata to manager
	snap := snapshotting.NewSnapshot(revision, mgr.baseFolder, image, memSizeMib, vCPUCount, sparse)
	mgr.snapshots[revision] = snap

	snapStat := NewSnapshotStats(revision, estimatedSnapSizeMib, coldStartTimeMs, mgr.clock)
	mgr.snapStats[revision] = snapStat
	mgr.Unlock()

	// Create directory to store snapshot data
	err := snap.CreateSnapDir()
	if err != nil {
		return removeContainerSnaps, nil, errors.Wrapf(err, "creating snapDir for snapshots %s", revision)
	}

	return removeContainerSnaps, snap, nil
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *FullLocalSnapshotManager) CommitSnapshot(revision string) error {
	mgr.Lock()
	snapStat, present := mgr.snapStats[revision]
	if !present {
		mgr.Unlock()
		return errors.New(fmt.Sprintf("Snapshot for revision %s to commit does not exist", revision))
	}

	if snapStat.usable {
		mgr.Unlock()
		return errors.New(fmt.Sprintf("Snapshot for revision %s has already been committed", revision))
	}

	snap := mgr.snapshots[revision]
	mgr.Unlock()

	// Calculate actual disk size used
	var sizeIncrement int64
	oldSize := snapStat.TotalSizeMiB

	snapStat.UpdateSize(snap.CalculateDiskSize()) // Should always result in a decrease or equal!
	sizeIncrement = snapStat.TotalSizeMiB - oldSize

	mgr.Lock()
	defer mgr.Unlock()
	mgr.usedMib += sizeIncrement
	snapStat.usable = true
	snapStat.UpdateScore()
	heap.Push(&mgr.freeSnaps, snapStat)

	return nil
}

// freeSpace makes sure neededMib of disk space is available by removing unused snapshots. Make sure to have a lock
// when calling this function.
func (mgr *FullLocalSnapshotManager) freeSpace(neededMib int64) (*[]string, error) {
	var toDelete []string
	var freedMib int64 = 0
	var removeContainerSnaps []string

	// Get id of snapshot and name of devmapper snapshot to delete
	for freedMib < neededMib && len(mgr.freeSnaps) > 0 {
		snapStat := heap.Pop(&mgr.freeSnaps).(*SnapshotStats)
		snapStat.usable = false
		toDelete = append(toDelete, snapStat.revisionId)

		snap := mgr.snapshots[snapStat.revisionId]
		removeContainerSnaps = append(removeContainerSnaps, snap.GetContainerSnapName())
		freedMib += snapStat.TotalSizeMiB
	}

	// Delete snapshots resources, update clock & delete snapshot map entry
	for _, revisionId := range toDelete {
		snap := mgr.snapshots[revisionId]
		if err := snap.Cleanup(); err != nil {
			return &removeContainerSnaps, errors.Wrapf(err, "removing snapshot %s snapDir", snap.GetId())
		}
		delete(mgr.snapshots, revisionId)

		snapStat := mgr.snapStats[revisionId]
		snapStat.UpdateScore() // Update score (see Faascache policy)
		if snapStat.score > mgr.clock {
			mgr.clock = snapStat.score
		}
		delete(mgr.snapStats, revisionId)
	}

	mgr.usedMib -= freedMib

	if freedMib < neededMib {
		return nil, errors.New("There is not enough free space available")
	}

	return &removeContainerSnaps, nil
}