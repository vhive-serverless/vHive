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

package snapshotting

import (
	"container/heap"
	"fmt"
	"github.com/pkg/errors"
	"math"
	"os"
	"sync"
)

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	snapshots          map[string]*Snapshot

	// Heap of snapshots not in use sorted on score
	freeSnaps          SnapHeap
	baseFolder         string

	// Eviction
	clock       int64 	// When container last used. Increased to priority terminated container on termination
	capacityMib int64
	usedMib     int64
}

func NewSnapshotManager(baseFolder string, capacityMib int64) *SnapshotManager {
	manager := new(SnapshotManager)
	manager.snapshots = make(map[string]*Snapshot)
	heap.Init(&manager.freeSnaps)
	manager.baseFolder = baseFolder
	manager.clock = 0
	manager.capacityMib = capacityMib
	manager.usedMib = 0

	// Clean & init basefolder
	os.RemoveAll(manager.baseFolder)
	os.MkdirAll(manager.baseFolder, os.ModePerm)

	return manager
}

// AcquireSnapshot returns a snapshot for the specified revision if it is available and increments the internal counter
// such that the snapshot can't get removed. Similar to how a RW lock works
func (mgr *SnapshotManager) AcquireSnapshot(revision string) (*Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	// Check if a snapshot is available for the specified revision
	snap, present := mgr.snapshots[revision]
	if !present {
		return nil, errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}

	// Snapshot registered in manager but creation not finished yet
	if ! snap.usable { // Could also wait until snapshot usable (trade-off)
		return nil, errors.New(fmt.Sprintf("Snapshot is not yet usable"))
	}

	if snap.numUsing == 0 {
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

	snap.numUsing += 1

	// Update stats for keepalive policy
	snap.freq += 1
	snap.lastUsedClock = mgr.clock

	return snap, nil
}

// ReleaseSnapshot releases the snapshot with the given revision so that it can possibly get deleted if it is not in use
// by any other VMs.
func (mgr *SnapshotManager) ReleaseSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	snap, present := mgr.snapshots[revision]
	if !present {
		return errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}

	snap.numUsing -= 1

	if snap.numUsing == 0 {
		// Add to free snaps
		snap.UpdateScore()
		heap.Push(&mgr.freeSnaps, snap)
	}

	return nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has been created,
// CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available fo ruse
func (mgr *SnapshotManager) InitSnapshot(revision, image string, coldStartTimeMs int64, memSizeMib, vCPUCount uint32, sparse bool) (*[]string, *Snapshot, error) {
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

	// Add snapshot metadata to manager
	snap := NewSnapshot(revision, mgr.baseFolder, image, estimatedSnapSizeMib, coldStartTimeMs, mgr.clock, memSizeMib, vCPUCount, sparse)
	mgr.snapshots[revision] = snap
	mgr.Unlock()

	// Create directory to store snapshot data
	err := os.Mkdir(snap.snapDir, 0755)
	if err != nil {
		return removeContainerSnaps, nil, errors.Wrapf(err, "creating snapDir for snapshots %s", revision)
	}

	return removeContainerSnaps, snap, nil
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *SnapshotManager) CommitSnapshot(revision string) error {
	mgr.Lock()
	snap, present := mgr.snapshots[revision]
	if !present {
		mgr.Unlock()
		return errors.New(fmt.Sprintf("Snapshot for revision %s to commit does not exist", revision))
	}
	mgr.Unlock()

	// Calculate actual disk size used
	var sizeIncrement int64 = 0
	oldSize := snap.TotalSizeMiB
	snap.UpdateDiskSize() // Should always result in a decrease or equal!
	sizeIncrement = snap.TotalSizeMiB - oldSize

	mgr.Lock()
	defer mgr.Unlock()
	mgr.usedMib += sizeIncrement
	snap.usable = true
	snap.UpdateScore()
	heap.Push(&mgr.freeSnaps, snap)

	return nil
}

// freeSpace makes sure neededMib of disk space is available by removing unused snapshots. Make sure to have a lock
// when calling this function.
func (mgr *SnapshotManager) freeSpace(neededMib int64) (*[]string, error) {
	var toDelete []string
	var freedMib int64 = 0
	var removeContainerSnaps []string

	// Get id of snapshot and name of devmapper snapshot to delete
	for freedMib < neededMib && len(mgr.freeSnaps) > 0 {
		snap := heap.Pop(&mgr.freeSnaps).(*Snapshot)
		snap.usable = false
		toDelete = append(toDelete, snap.revisionId)
		removeContainerSnaps = append(removeContainerSnaps, snap.containerSnapName)
		freedMib += snap.TotalSizeMiB
	}

	// Delete snapshots resources, update clock & delete snapshot map entry
	for _, revisionId := range toDelete {
		snap := mgr.snapshots[revisionId]
		if err := os.RemoveAll(snap.snapDir); err != nil {
			return &removeContainerSnaps, errors.Wrapf(err, "removing snapshot snapDir %s", snap.snapDir)
		}
		snap.UpdateScore() // Update score (see Faascache policy)
		if snap.score > mgr.clock {
			mgr.clock = snap.score
		}
		delete(mgr.snapshots, revisionId)
	}

	mgr.usedMib -= freedMib

	if freedMib < neededMib {
		return nil, errors.New("There is not enough free space available")
	}

	return &removeContainerSnaps, nil
}