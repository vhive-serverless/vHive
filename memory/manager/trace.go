package manager

import (
	"encoding/csv"
	"os"
	"sort"
	"strconv"
	"sync"

	"github.com/prometheus/common/log"
)

// Record A tuple with an address and a timestamp
type Record struct {
	offset    uint64
	timestamp int64
	servedNum int
}

// Trace Contains records
type Trace struct {
	sync.Mutex
	traceFileName string

	isRecord bool

	trace   []Record
	regions map[uint64]int
}

func initTrace(traceFileName string) *Trace {
	t := new(Trace)

	t.traceFileName = traceFileName
	t.regions = make(map[uint64]int)
	t.wsCopyFileName = wsCopyFileName
	t.trace = make([]Record, 0)

	return t
}

// AppendRecord Appends a record to the trace
func (t *Trace) AppendRecord(r Record) {
	t.Lock()
	defer t.Unlock()

	t.trace = append(t.trace, r)
}

// WriteTrace Writes all the records to a file
func (t *Trace) WriteTrace() {
	t.Lock()
	defer t.Unlock()

	file, err := os.Create(t.traceFileName)
	if err != nil {
		log.Fatalf("Failed to open trace file for writing: %v", err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for _, rec := range t.trace {
		err := writer.Write([]string{
			strconv.FormatUint(rec.offset, 16),
			strconv.FormatInt(rec.timestamp, 10),
			strconv.Itoa(rec.servedNum)})
		if err != nil {
			log.Fatalf("Failed to write trace: %v", err)
		}
	}

	t.processRegions()
}

// readTrace Reads all the records from a CSV file
func (t *Trace) readTrace() {
	f, err := os.Open(t.traceFileName)
	if err != nil {
		log.Fatalf("Failed to open trace file for reading: %v", err)
	}
	defer f.Close()

	lines, err := csv.NewReader(f).ReadAll()
	if err != nil {
		log.Fatalf("Failed to read from the trace file: %v", err)
	}

	for _, line := range lines {
		offset, err := strconv.ParseUint(line[0], 16, 64)
		if err != nil {
			log.Fatalf("Failed to convert string to offset: %v", err)
		}
		timestamp, err := strconv.ParseInt(line[1], 10, 64)
		if err != nil {
			log.Fatalf("Failed to convert string to timestamp: %v", err)
		}
		servedNum, err := strconv.Atoi(line[2])
		if err != nil {
			log.Fatalf("Failed to convert string to servedNum: %v", err)
		}

		rec := Record{
			offset:    offset,
			timestamp: timestamp,
			servedNum: servedNum,
		}
		t.AppendRecord(rec)
	}

	t.processRegions()
}

// processRegions
func (t *Trace) processRegions() {

	sort.Slice(t.trace, func(i, j int) bool {
		return t.trace[i].offset < t.trace[j].offset
	})

	var last, regionStart uint64
	for _, rec := range t.trace {
		if rec.offset != last+uint64(os.Getpagesize()) {
			regionStart = rec.offset
			t.regions[regionStart] = 1
		} else {
			t.regions[regionStart]++
		}

		last = rec.offset
	}
}

func (t *Trace) writeWorkingSetPagesToFile(guestMemFileName, wsCopyFileName string) {
	fSrc, err := os.Open(guestMemFileName)
	if err != nil {
		log.Fatalf("Failed to open guest memory file for reading")
	}
	defer fSrc.Close()
	fDst, err := os.Create(wsCopyFileName)
	if err != nil {
		log.Fatalf("Failed to open ws file for writing")
	}
	defer fDst.Close()

	var (
		dstOffset int64
		count     int
	)

	// Form a sorted slice of keys to access the map in a predetermined order
	keys := make([]uint64, 0)
	for k := range t.regions {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, offset := range keys {
		regLength := t.regions[offset]
		copyLen := regLength * os.Getpagesize()

		buf := make([]byte, copyLen)

		if n, err := fSrc.ReadAt(buf, int64(offset)); n != copyLen || err != nil {
			log.Fatalf("Read file failed for src")
		}

		if n, err := fDst.WriteAt(buf, dstOffset); n != copyLen || err != nil {
			log.Fatalf("Write file failed for dst")
		}

		dstOffset += int64(copyLen)

		count += regLength
	}

	fDst.Sync()
}
