package snapshotting

import (
	"fmt"
	"github.com/pkg/errors"
	"io/ioutil"
	"os"
)

// Use K_REVISION environment variable as identifier for snapshot (https://github.com/amohoste/podspeed-vhive/blob/c74ca6ced1579d1c4f5414f3a28a8ffceb7b544f/pkg/pod/types/vhive.go#L46)
type SnapshotManager struct {
	snapshots        map[string]Snapshot // maps revision id to snapshot
	availableSizeMiB string
	BasePath         string
}

func NewSnapshotManager(baseFolder string) *SnapshotManager {
	manager := new(SnapshotManager)
	manager.snapshots = make(map[string]Snapshot)
	manager.BasePath = baseFolder
	return manager
}

func (mgr *SnapshotManager) GetSnapshot(revision string) (*Snapshot, error) {
	snap, present := mgr.snapshots[revision]
	if present {
		return &snap, nil
	} else {
		return nil, errors.New(fmt.Sprintf("Get: Snapshot for revision %s does not exist", revision))
	}
}

func (mgr *SnapshotManager) RegisterSnap(revision string) (*Snapshot, error) {
	if _, present := mgr.snapshots[revision]; present {
		return nil, errors.New(fmt.Sprintf("Add: Snapshot for revision %s already exists", revision))
	}
	snap := NewSnapshot(revision, mgr.BasePath)

	err := os.Mkdir(snap.GetBaseFolder(), 0755)
	if err != nil {
		return nil, errors.Wrapf(err, "creating folder for snapshots %s", revision)
	}

	mgr.snapshots[revision] = snap
	return &snap, nil
}

func (mgr *SnapshotManager) RemoveSnapshot(revision string) error {
	snapshot, present := mgr.snapshots[revision]
	if !present {
		return errors.New(fmt.Sprintf("Delete: Snapshot for revision %s does not exist", revision))
	}

	err := os.RemoveAll(snapshot.GetBaseFolder())
	delete(mgr.snapshots, revision)

	if err != nil {
		return errors.Wrapf(err, "removing snapshot folder %s", snapshot.GetBaseFolder())
	}

	return nil
}

// Doesn't check if correct files in folders!
func (mgr *SnapshotManager) RecoverSnapshots(basePath string) error {
	files, err := ioutil.ReadDir(basePath)
	if err != nil {
		return errors.Wrapf(err, "reading folders in %s", basePath)
	}

	for _, f := range files {
		if f.IsDir() {
			revision := f.Name()
			mgr.snapshots[revision] = NewSnapshot(revision, mgr.BasePath)
			if err != nil {
				return errors.Wrapf(err, "recovering snapshot %s", f.Name())
			}
		}
	}
	return nil
}
