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
	revisionId             string
	containerSnapName      string
	snapDir                string
	Image                  string
	MemSizeMib             uint32
	VCPUCount              uint32
	usable                 bool
	sparse                 bool

	// Eviction
	numUsing               uint32
	TotalSizeMiB           int64
	freq                   int64
	coldStartTimeMs        int64
	lastUsedClock          int64
	score                  int64
}

func NewSnapshot(revisionId, baseFolder, image string, sizeMiB, coldStartTimeMs, lastUsed int64, memSizeMib, vCPUCount uint32, sparse bool) *Snapshot {
	s := &Snapshot{
		revisionId:             revisionId,
		snapDir:                filepath.Join(baseFolder, revisionId),
		containerSnapName:      fmt.Sprintf("%s%s", revisionId, time.Now().Format("20060102150405")),
		Image:                  image,
		MemSizeMib:             memSizeMib,
		VCPUCount:              vCPUCount,
		usable:                 false,
		numUsing:               0,
		TotalSizeMiB:           sizeMiB,
		coldStartTimeMs:        coldStartTimeMs,
		lastUsedClock:          lastUsed, // Initialize with used now to avoid immediately removing
		sparse:                 sparse,
	}

	return s
}

// UpdateDiskSize Updates the estimated disk size to real disk size in use by snapshot
func (snp *Snapshot) UpdateDiskSize() {
	snp.TotalSizeMiB = getRealSizeMib(snp.GetMemFilePath()) + getRealSizeMib(snp.GetSnapFilePath()) + getRealSizeMib(snp.GetInfoFilePath()) + getRealSizeMib(snp.GetPatchFilePath())
}

// getRealSizeMib returns the disk space used by a certain file
func getRealSizeMib(filePath string) int64 {
	var st unix.Stat_t
	if err := unix.Stat(filePath, &st); err != nil {
		return 0
	}
	return int64(math.Ceil((float64(st.Blocks) * 512) / (1024 * 1024)))
}

// UpdateScore updates the score of the snapshot used by the keepalive policy
func (snp *Snapshot) UpdateScore() {
	snp.score = snp.lastUsedClock + (snp.freq * snp.coldStartTimeMs) / snp.TotalSizeMiB
}

func (snp *Snapshot) GetImage() string {
	return snp.Image
}

func (snp *Snapshot) GetRevisionId() string {
	return snp.revisionId
}

func (snp *Snapshot) GetContainerSnapName() string {
	return snp.containerSnapName
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