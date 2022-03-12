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
	"encoding/gob"
	"fmt"
	"github.com/pkg/errors"
	"golang.org/x/sys/unix"
	"math"
	"os"
	"path/filepath"
	"time"
)

// Snapshot identified by revision
// Only capitalized fields are serialised / deserialised
type Snapshot struct {
	id                string // id for deduplicated
	ContainerSnapName string
	snapDir           string
	Image             string
	MemSizeMib        uint32
	VCPUCount         uint32
	sparse            bool
}

func NewSnapshot(id, baseFolder, image string, memSizeMib, vCPUCount uint32, sparse bool) *Snapshot {
	s := &Snapshot{
		id:                id,
		snapDir:           filepath.Join(baseFolder, id),
		ContainerSnapName: fmt.Sprintf("%s%s", id, time.Now().Format("20060102150405")),
		Image:             image,
		MemSizeMib:        memSizeMib,
		VCPUCount:         vCPUCount,
		sparse:            sparse,
	}

	return s
}

func (snp *Snapshot) CalculateDiskSize() int64 {
	return getRealSizeMib(snp.GetMemFilePath()) + getRealSizeMib(snp.GetSnapFilePath()) + getRealSizeMib(snp.GetInfoFilePath()) + getRealSizeMib(snp.GetPatchFilePath())
}

// getRealSizeMib returns the disk space used by a certain file
func getRealSizeMib(filePath string) int64 {
	var st unix.Stat_t
	if err := unix.Stat(filePath, &st); err != nil {
		return 1
	}
	realSize := int64(math.Ceil((float64(st.Blocks) * 512) / (1024 * 1024)))
	// Mainly for unit tests where real disk size = 0
	if realSize == 0 {
		return 1
	}
	return realSize
}

func (snp *Snapshot) CreateSnapDir() error {
	return os.Mkdir(snp.snapDir, 0755)
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

func (snp *Snapshot) GetSnapFilePath() string {
	return filepath.Join(snp.snapDir, "snapfile")
}

func (snp *Snapshot) GetSnapType() string {
	var snapType string
	if snp.sparse {
		snapType = "Diff"
	} else {
		snapType = "Full"
	}
	return snapType
}

func (snp *Snapshot) GetMemFilePath() string {
	return filepath.Join(snp.snapDir, "memfile")
}

func (snp *Snapshot) GetPatchFilePath() string {
	return filepath.Join(snp.snapDir, "patchfile")
}

func (snp *Snapshot) GetInfoFilePath() string {
	return filepath.Join(snp.snapDir, "infofile")
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

	return  nil
}

func (snp *Snapshot) Cleanup() error {
	return os.RemoveAll(snp.snapDir)
}