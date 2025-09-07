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
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
)

// Snapshot identified by revision
// Only capitalized fields are serialised / deserialised
type Snapshot struct {
	id                    string
	ready                 bool
	ContainerSnapName     string
	snapDir               string
	Image                 string
	LastAccessedTimestamp time.Time // For LRU eviction
	SizeInBytes           int64     // Size of the snapshot on disk
}

func NewSnapshot(id, baseFolder, image string) *Snapshot {
	s := &Snapshot{
		id:                    id,
		ready:                 false,
		snapDir:               filepath.Join(baseFolder, id),
		ContainerSnapName:     fmt.Sprintf("%s%s", id, time.Now().Format("20060102150405")),
		Image:                 image,
		LastAccessedTimestamp: time.Now(),
		SizeInBytes:           0, // Will be calculated after snapshot creation
	}

	return s
}

func (snp *Snapshot) CreateSnapDir() error {
	err := os.Mkdir(snp.snapDir, 0755)
	if err != nil && os.IsExist(err) {
		return nil
	}
	return err
}

func (snp *Snapshot) GetImage() string {
	return snp.Image
}

func (snp *Snapshot) GetId() string {
	return snp.id
}

func (snp *Snapshot) GetContainerSnapName() string {
	return snp.ContainerSnapName
}

func (snp *Snapshot) GetSnapshotFilePath() string {
	return filepath.Join(snp.snapDir, "snap_file")
}

func (snp *Snapshot) GetMemFilePath() string {
	return filepath.Join(snp.snapDir, "mem_file")
}

func (snp *Snapshot) GetPatchFilePath() string {
	return filepath.Join(snp.snapDir, "patch_file")
}

func (snp *Snapshot) GetInfoFilePath() string {
	return filepath.Join(snp.snapDir, "info_file")
}

// SerializeSnapInfo serializes the snapshot info using gob. This can be useful for remote snapshots
func (snp *Snapshot) SerializeSnapInfo() error {
	file, err := os.Create(snp.GetInfoFilePath())
	if err != nil {
		return errors.Wrapf(err, "failed to create snapinfo file")
	}
	defer file.Close()

	encoder := gob.NewEncoder(file)

	err = encoder.Encode(*snp)
	if err != nil {
		return errors.Wrapf(err, "failed to encode snapinfo")
	}
	return nil
}

// LoadSnapInfo loads the snapshot info from a file. This can be useful for remote snapshots.
func (snp *Snapshot) LoadSnapInfo(infoPath string) error {
	file, err := os.Open(infoPath)
	if err != nil {
		return errors.Wrapf(err, "failed to open snapinfo file")
	}
	defer file.Close()

	encoder := gob.NewDecoder(file)

	err = encoder.Decode(snp)
	if err != nil {
		return errors.Wrapf(err, "failed to decode snapinfo")
	}

	return nil
}

func (snp *Snapshot) Cleanup() error {
	return os.RemoveAll(snp.snapDir)
}

// UpdateLastAccessedTimestamp updates the last accessed timestamp for LRU tracking
func (snp *Snapshot) UpdateLastAccessedTimestamp() {
	snp.LastAccessedTimestamp = time.Now()
}

// GetLastAccessedTimestamp returns the last accessed timestamp
func (snp *Snapshot) GetLastAccessedTimestamp() time.Time {
	return snp.LastAccessedTimestamp
}

// GetSizeInBytes returns the size of the snapshot on disk
func (snp *Snapshot) GetSizeInBytes() int64 {
	return snp.SizeInBytes
}

// CalculateAndSetSize calculates and sets the size of the snapshot on disk
func (snp *Snapshot) CalculateAndSetSize() error {
	totalSize, err := snp.calculateDirectorySize(snp.snapDir)
	if err != nil {
		return errors.Wrapf(err, "failed to calculate snapshot size for %s", snp.id)
	}
	snp.SizeInBytes = totalSize
	return nil
}

// calculateDirectorySize recursively calculates the total size of a directory
func (snp *Snapshot) calculateDirectorySize(dirPath string) (int64, error) {
	var totalSize int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize, err
}

// EstimateSnapshotSize estimates the size of a snapshot based on memory size
// This is used before creating the snapshot to check if there's enough space
func (snp *Snapshot) EstimateSnapshotSize(memorySize int64) int64 {
	// Rough estimation: memory file size + VM state file + patch file
	// Memory file is typically the largest component
	// VM state file is usually small (few MB)
	// Patch file size varies but is typically smaller than memory
	
	vmStateSize := int64(50 * 1024 * 1024)  // ~50MB for VM state

	return memorySize + vmStateSize
}
