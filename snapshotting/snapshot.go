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
	id                string
	ContainerSnapName string
	snapDir           string
	Image             string
}

func NewSnapshot(id, baseFolder, image string) *Snapshot {
	s := &Snapshot{
		id:                id,
		snapDir:           filepath.Join(baseFolder, id),
		ContainerSnapName: fmt.Sprintf("%s%s", id, time.Now().Format("20060102150405")),
		Image:             image,
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
