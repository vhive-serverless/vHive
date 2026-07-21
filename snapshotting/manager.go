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

package snapshotting

import (
	"fmt"
	"github.com/pkg/errors"
	"os"
	"sync"

	log "github.com/sirupsen/logrus"
)

// SnapshotManager manages snapshots stored on the node.
type SnapshotManager struct {
	sync.Mutex
	// Stored snapshots (identified by the function instance revision, which is provided by the `K_REVISION` environment
	// variable of knative).
	snapshots  map[string]*Snapshot
	baseFolder string
	catalog    Catalog
}

// Snapshot identified by VM id

func NewSnapshotManager(baseFolder string) *SnapshotManager {
	manager := new(SnapshotManager)
	manager.snapshots = make(map[string]*Snapshot)
	manager.baseFolder = baseFolder

	// Clean & init basefolder
	_ = os.RemoveAll(manager.baseFolder)
	_ = os.MkdirAll(manager.baseFolder, os.ModePerm)
	catalog, err := NewLocalCatalog(manager.baseFolder)
	manager.catalog = catalog
	if err != nil {
		// Preserve the constructor's historical no-error API. Operations will
		// return the filesystem error instead of panicking if initialization was
		// not possible.
		manager.catalog = &LocalCatalog{baseFolder: manager.baseFolder}
	}

	return manager
}

// AcquireSnapshot returns a snapshot for the specified revision if it is available.
func (mgr *SnapshotManager) AcquireSnapshot(revision string) (*Snapshot, error) {
	mgr.Lock()
	defer mgr.Unlock()

	descriptor, err := mgr.catalog.Get(revision)
	if err != nil {
		return nil, err
	}
	if snap, ok := mgr.snapshots[revision]; ok {
		return snap, nil
	}
	return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor), nil
}

// InitSnapshot initializes a snapshot by adding its metadata to the SnapshotManager. Once the snapshot has
// been created, CommitSnapshot must be run to finalize the snapshot creation and make the snapshot available for use.
func (mgr *SnapshotManager) InitSnapshot(revision, image string) (*Snapshot, error) {
	mgr.Lock()

	logger := log.WithFields(log.Fields{"revision": revision, "image": image})
	logger.Debug("Initializing snapshot corresponding to revision and image")

	if exists, err := mgr.catalog.Exists(revision); err != nil {
		mgr.Unlock()
		return nil, err
	} else if exists {
		mgr.Unlock()
		return nil, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists", revision))
	}

	// Create snapshot object and move into creating state
	descriptor, err := mgr.catalog.Begin(revision, image)
	if err != nil {
		mgr.Unlock()
		return nil, err
	}
	snap := NewSnapshotFromDescriptor(mgr.baseFolder, descriptor)
	mgr.snapshots[snap.GetId()] = snap
	mgr.Unlock()

	return snap, nil
}

// CommitSnapshot finalizes the snapshot creation and makes it available for use.
func (mgr *SnapshotManager) CommitSnapshot(revision string) error {
	mgr.Lock()
	defer mgr.Unlock()

	if err := mgr.catalog.Commit(revision); err != nil {
		return err
	}
	if snap, ok := mgr.snapshots[revision]; ok {
		snap.ready = true
	}
	return nil
}

// Catalog exposes the lifecycle port while SnapshotManager remains as a
// compatibility adapter for callers that need a local Snapshot object.
func (mgr *SnapshotManager) Catalog() Catalog {
	return mgr.catalog
}

// SnapshotForDescriptor returns the existing filesystem adapter for a catalog
// entry without performing another readiness lookup.
func (mgr *SnapshotManager) SnapshotForDescriptor(descriptor *SnapshotDescriptor) *Snapshot {
	return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor)
}
