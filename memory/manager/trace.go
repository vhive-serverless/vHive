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
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"sync"
)

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

func (t *Trace) WriteTrace() error {
	t.Lock()
	defer t.Unlock()

	file, err := os.Create(t.traceFileName)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	writer := csv.NewWriter(file)
	for _, rec := range t.trace {
		if err := writer.Write([]string{strconv.FormatUint(rec.offset, 16)}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}

//nolint:unused
func (t *Trace) readTrace() error {
	f, err := os.Open(t.traceFileName)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	lines, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return err
	}

	for _, line := range lines {
		rec, err := readRecord(line)
		if err != nil {
			return err
		}
		t.AppendRecord(rec)
	}
	return nil
}

//nolint:unused
func readRecord(line []string) (Record, error) {
	if len(line) == 0 {
		return Record{}, errors.New("empty trace record")
	}
	offset, err := strconv.ParseUint(line[0], 16, 64)
	if err != nil {
		return Record{}, err
	}

	return Record{offset: offset}, nil
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
	sort.Slice(t.trace, func(i, j int) bool {
		return t.trace[i].offset < t.trace[j].offset
	})

	t.regions = make(map[uint64]int)
	var (
		last        uint64
		regionStart uint64
	)
	for i, rec := range t.trace {
		if i == 0 || rec.offset != last+pageSize {
			regionStart = rec.offset
			t.regions[regionStart] = 1
		} else {
			t.regions[regionStart]++
		}
		last = rec.offset
	}

	return t.writeWorkingSetPagesToFileLocked(guestMemPath, workingSetPath, pageSize)
}

func (t *Trace) writeWorkingSetPagesToFileLocked(guestMemPath, workingSetPath string, pageSize uint64) error {
	fSrc, err := os.Open(guestMemPath)
	if err != nil {
		return err
	}
	defer func() { _ = fSrc.Close() }()

	fDst, err := os.Create(workingSetPath)
	if err != nil {
		return err
	}
	defer func() { _ = fDst.Close() }()

	keys := make([]uint64, 0, len(t.regions))
	for k := range t.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

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

	return fDst.Sync()
}
