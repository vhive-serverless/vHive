// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov, Plamen Petrov and EASE lab
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

package manager

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const workingSetTraceVersion = 1

// Record identifies one guest memory page by its offset in the full memory file.
type Record struct {
	offset uint64
}

// Trace stores recorded guest memory page offsets and replay regions.
type Trace struct {
	sync.Mutex
	traceFileName string

	pageSize         uint64
	containedOffsets map[uint64]struct{}
	trace            []Record
	regions          map[uint64]int
}

type workingSetTraceMetadata struct {
	Version  int      `json:"version"`
	PageSize uint64   `json:"page_size"`
	Offsets  []uint64 `json:"offsets"`
}

func initTrace(traceFileName string) *Trace {
	return &Trace{
		traceFileName:    traceFileName,
		containedOffsets: make(map[uint64]struct{}),
		trace:            make([]Record, 0),
		regions:          make(map[uint64]int),
	}
}

func (t *Trace) AppendRecord(r Record) {
	t.Lock()
	defer t.Unlock()

	if _, ok := t.containedOffsets[r.offset]; ok {
		return
	}
	t.trace = append(t.trace, r)
	t.containedOffsets[r.offset] = struct{}{}
}

func (t *Trace) readTrace() error {
	data, err := os.ReadFile(t.traceFileName)
	if err != nil {
		return err
	}

	var metadata workingSetTraceMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return err
	}
	if metadata.Version != workingSetTraceVersion {
		return fmt.Errorf("unsupported working set trace version: %d", metadata.Version)
	}
	if metadata.PageSize == 0 {
		return errInvalidGuestRegionPageSize
	}

	containedOffsets := make(map[uint64]struct{}, len(metadata.Offsets))
	records := make([]Record, 0, len(metadata.Offsets))
	for _, offset := range metadata.Offsets {
		if _, ok := containedOffsets[offset]; ok {
			return fmt.Errorf("duplicate working set trace offset: %#x", offset)
		}
		records = append(records, Record{offset: offset})
		containedOffsets[offset] = struct{}{}
	}

	t.Lock()
	defer t.Unlock()

	t.pageSize = metadata.PageSize
	t.containedOffsets = containedOffsets
	t.trace = records
	t.buildRegionsLocked()

	return nil
}

func (t *Trace) reset() {
	t.Lock()
	defer t.Unlock()

	t.pageSize = 0
	t.containedOffsets = make(map[uint64]struct{})
	t.trace = make([]Record, 0)
	t.regions = make(map[uint64]int)
}

func (t *Trace) containsRecord(rec Record) bool {
	_, ok := t.containedOffsets[rec.offset]
	return ok
}

func (t *Trace) ProcessRecord(guestMemPath, workingSetPath string, pageSize uint64) error {
	if pageSize == 0 {
		return errInvalidGuestRegionPageSize
	}

	t.Lock()
	defer t.Unlock()

	t.pageSize = pageSize
	t.buildRegionsLocked()

	if err := t.writeWorkingSetPagesToFileLocked(guestMemPath, workingSetPath, pageSize); err != nil {
		return err
	}
	return t.writeTraceLocked()
}

func (t *Trace) buildRegionsLocked() {
	sort.Slice(t.trace, func(i, j int) bool {
		return t.trace[i].offset < t.trace[j].offset
	})

	t.regions = make(map[uint64]int)
	var (
		last        uint64
		regionStart uint64
	)
	for i, rec := range t.trace {
		if i == 0 || rec.offset != last+t.pageSize {
			regionStart = rec.offset
			t.regions[regionStart] = 1
		} else {
			t.regions[regionStart]++
		}
		last = rec.offset
	}
}

func (t *Trace) writeTraceLocked() error {
	offsets := make([]uint64, len(t.trace))
	for i, rec := range t.trace {
		offsets[i] = rec.offset
	}

	data, err := json.Marshal(workingSetTraceMetadata{
		Version:  workingSetTraceVersion,
		PageSize: t.pageSize,
		Offsets:  offsets,
	})
	if err != nil {
		return err
	}

	return writeFileAtomically(t.traceFileName, func(file *os.File) error {
		_, err := file.Write(data)
		return err
	})
}

func (t *Trace) writeWorkingSetPagesToFileLocked(guestMemPath, workingSetPath string, pageSize uint64) error {
	fSrc, err := os.Open(guestMemPath)
	if err != nil {
		return err
	}
	defer func() { _ = fSrc.Close() }()

	keys := make([]uint64, 0, len(t.regions))
	for k := range t.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	return writeFileAtomically(workingSetPath, func(fDst *os.File) error {
		var dstOffset int64
		for _, offset := range keys {
			copyLen := uint64(t.regions[offset]) * pageSize
			if copyLen > uint64(int(^uint(0)>>1)) {
				return fmt.Errorf("working set region too large: %#x", copyLen)
			}

			buf := make([]byte, int(copyLen))
			n, err := fSrc.ReadAt(buf, int64(offset))
			if err != nil && err != io.EOF {
				return err
			}
			if n != len(buf) {
				return io.ErrUnexpectedEOF
			}

			n, err = fDst.WriteAt(buf, dstOffset)
			if err != nil {
				return err
			}
			if n != len(buf) {
				return io.ErrShortWrite
			}
			dstOffset += int64(copyLen)
		}

		return nil
	})
}

func writeFileAtomically(path string, write func(*os.File) error) error {
	tempFile, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if err := write(tempFile); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}

	return os.Rename(tempPath, path)
}
