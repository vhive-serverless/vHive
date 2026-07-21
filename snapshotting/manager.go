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
	"context"
	"errors"
	"fmt"
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
	remote     *remoteSnapshotTransfer
}

// EnableRemoteTransfer makes committed snapshots available to other workers
// through store.  It is deliberately opt-in: without this call the manager
// retains the original local-only behaviour.
func (mgr *SnapshotManager) EnableRemoteTransfer(store ArtifactStore, cacheSnaps bool) {
	mgr.Lock()
	defer mgr.Unlock()
	if store == nil {
		mgr.remote = nil
		return
	}
	mgr.remote = newRemoteSnapshotTransfer(store, cacheSnaps)
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
	return mgr.AcquireSnapshotContext(context.Background(), revision)
}

// AcquireSnapshotContext returns a local snapshot, fetching a committed remote
// copy on a local cache miss when remote transfer has been enabled.
func (mgr *SnapshotManager) AcquireSnapshotContext(ctx context.Context, revision string) (*Snapshot, error) {
	mgr.Lock()
	descriptor, err := mgr.catalog.Get(revision)
	if err == nil {
		if snap, ok := mgr.snapshots[revision]; ok {
			mgr.Unlock()
			return snap, nil
		}
		mgr.Unlock()
		return NewSnapshotFromDescriptor(mgr.baseFolder, descriptor), nil
	}
	remote := mgr.remote
	mgr.Unlock()
	if remote == nil || (!errors.Is(err, ErrSnapshotNotFound) && !(errors.Is(err, ErrSnapshotNotReady) && remote.hasDownload(revision))) {
		return nil, err
	}

	descriptor, err = remote.download(ctx, mgr.catalog, mgr.baseFolder, revision)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("add: snapshot for revision %s already exists", revision)
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

// PublishSnapshot uploads a locally committed snapshot.  The remote descriptor
// is published last, so a remote reader never accepts a partial transfer.
// When cacheSnaps is false, successful publication removes the local copy.
func (mgr *SnapshotManager) PublishSnapshot(ctx context.Context, revision string) error {
	mgr.Lock()
	remote := mgr.remote
	catalog := mgr.catalog
	baseFolder := mgr.baseFolder
	mgr.Unlock()
	if remote == nil {
		return nil
	}
	if err := remote.publish(ctx, catalog, baseFolder, revision); err != nil {
		return err
	}
	if !remote.cacheSnaps {
		mgr.Lock()
		delete(mgr.snapshots, revision)
		mgr.Unlock()
		if err := catalog.Delete(revision); err != nil {
			return fmt.Errorf("remove published local snapshot %s: %w", revision, err)
		}
	}
	return nil
}

func (mgr *SnapshotManager) RemoteTransferEnabled() bool {
	mgr.Lock()
	defer mgr.Unlock()
	return mgr.remote != nil
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
