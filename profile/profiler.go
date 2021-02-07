package profile

import (
	"encoding/csv"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Profiler a instance of toplev command
type Profiler struct {
	teardown     sync.Once
	cmd          *exec.Cmd
	tStart       time.Time
	interval     uint64
	execTime     float64
	warmTime     float64
	tearDownTime float64
	outFile      string
	cores        map[string]bool
	bottlenecks  map[string]float64
}

// NewProfiler returns a new instance of profiler
func NewProfiler(executionTime float64, printInterval uint64, vmNum, level int, nodes, outFile string) *Profiler {
	profiler := new(Profiler)
	profiler.execTime = executionTime
	profiler.interval = printInterval
	profiler.cores = make(map[string]bool)
	profiler.bottlenecks = make(map[string]float64)

	const coreNum = 20

	if outFile == "" {
		outFile = "profile"
	}
	profiler.outFile = outFile + ".csv"

	profiler.cmd = exec.Command("toplev.py",
		"-v",
		"--no-desc",
		"-x", ",",
		"-l", strconv.Itoa(level),
		"-I", strconv.FormatUint(printInterval, 10),
		"-o", profiler.outFile)

	if vmNum < coreNum {
		profiler.cmd.Args = append(profiler.cmd.Args, "--idle-threshold", "40")
	} else {
		profiler.cmd.Args = append(profiler.cmd.Args, "--idle-threshold", "0")
	}

	if nodes != "" {
		profiler.cmd.Args = append(profiler.cmd.Args, "--nodes", nodes)
	}

	profiler.cmd.Args = append(profiler.cmd.Args, "sleep", strconv.FormatFloat(executionTime, 'f', -1, 64))

	log.Debugf("Profiler command: %s", profiler.cmd)

	return profiler
}

// Run checks environment and arguments and executes command
func (p *Profiler) Run() error {
	if !isPmuToolInstalled() {
		return errors.New("pmu tool is not set")
	}

	if !isPerfInstalled() {
		return errors.New("perf is not installed")
	}

	if p.execTime < 0 {
		return errors.New("profiler execution time is less than 0s")
	}

	if p.interval < 10 {
		return errors.New("profiler print interval is less than 10ms")
	}

	if p.interval < 100 {
		log.Warn("print interval < 100ms. The overhead percentage could be high in some cases. Please proceed with caution.")
	}

	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.tStart = time.Now()

	return nil
}

// SetWarmTime sets the time duration until system is warm.
func (p *Profiler) SetWarmTime() {
	p.warmTime = time.Since(p.tStart).Seconds()

	if p.execTime > 0 && p.warmTime > p.execTime {
		log.Warn("System warmup time is longer than perf execution time.")
	}
}

// GetWarmupTime returns the time duration until system is warm.
func (p *Profiler) GetWarmupTime() float64 {
	return p.warmTime
}

// SetTearDownTime sets the time duration until system starts tearing down.
func (p *Profiler) SetTearDownTime() {
	p.teardown.Do(func() {
		p.tearDownTime = time.Since(p.tStart).Seconds()
	})
}

// GetTearDownTime returns the time duration until system starts tearing down.
func (p *Profiler) GetTearDownTime() float64 {
	return p.tearDownTime
}

// GetResult returns the counters of perf stat
func (p *Profiler) GetResult() (map[string]float64, error) {
	if p.tStart.IsZero() {
		return nil, errors.New("pmu tool was not executed, run it first")
	}

	// wait for profiler command finish
	timeLeft := p.execTime - time.Since(p.tStart).Seconds() + 5
	time.Sleep(time.Duration(timeLeft) * time.Second)

	log.Debugf("Warm time: %f, Teardown time: %f", p.warmTime, p.tearDownTime)
	return p.readCSV()
}

// PrintBottlenecks prints the bottlenecks during profiling
func (p *Profiler) PrintBottlenecks() {
	for k, v := range p.bottlenecks {
		log.Infof("Bottleneck %s with value %f", k, v)
	}
}

// GetCores returns measured core ID or thread ID
func (p *Profiler) GetCores() map[string]bool {
	return p.cores
}

type pmuLine struct {
	timestamp    float64
	cpu          string
	area         string
	value        float64
	unit         string
	isBottleneck bool
}

func (p *Profiler) readCSV() (map[string]float64, error) {
	var records []pmuLine

	// Open CSV file
	f, err := os.Open(p.outFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// Read File into a Variable
	reader := csv.NewReader(f)
	headers, err := reader.Read()
	if err != nil {
		return nil, err
	}
	headerIdxMap := headerPos(headers)
	for {
		line, err := reader.Read()
		if err != nil {
			if err == io.EOF || strings.HasPrefix(line[0], "#") {
				break
			}
			return nil, err
		}
		record, err := p.splitLine(headerIdxMap, line)
		if err != nil {
			return nil, err
		}
		if record != (pmuLine{}) {
			if record == (pmuLine{timestamp: -1}) {
				break
			}
			if record.isBottleneck {
				p.bottlenecks[record.area] = record.value
			}
			records = append(records, record)
		}
	}

	if err := os.Remove(p.outFile); err != nil {
		return nil, err
	}

	return p.parseMetric(records), nil
}

func (p *Profiler) splitLine(headers map[string]int, line []string) (pmuLine, error) {
	ts, err := strconv.ParseFloat(line[headers["Timestamp"]], 64)
	if err != nil {
		return pmuLine{}, err
	}

	if ts < p.warmTime {
		return pmuLine{}, nil
	} else if ts > p.tearDownTime {
		return pmuLine{timestamp: -1}, nil
	}

	value, err := strconv.ParseFloat(line[headers["Value"]], 64)
	if err != nil {
		log.Warnf("error line: %v", line)
		return pmuLine{}, err
	}

	idx, isCore := headers["CPUs"]
	var cpu string
	if !isCore {
		cpu = "uncore"
	} else {
		cpu = line[idx]
	}

	data := pmuLine{
		timestamp:    ts,
		cpu:          cpu,
		area:         line[headers["Area"]],
		value:        value,
		unit:         line[headers["Unit"]],
		isBottleneck: line[headers["Bottleneck"]] != "",
	}

	return data, nil
}

func headerPos(headers []string) map[string]int {
	result := make(map[string]int)
	for i, field := range headers {
		result[field] = i
	}
	return result
}

func (p *Profiler) parseMetric(lines []pmuLine) map[string]float64 {
	var (
		epochs  = make(map[string]float64)
		results = make(map[string]float64)
	)
	for _, line := range lines {
		results[line.area] += line.value
		epochs[line.area]++
		p.cores[line.cpu] = true
	}
	for k, v := range results {
		results[k] = v / epochs[k]
	}
	for k := range p.bottlenecks {
		p.bottlenecks[k] = results[k]
	}
	return results
}

func isPmuToolInstalled() bool {
	cmd := exec.Command("toplev.py", "--version")
	b, _ := cmd.Output()

	return len(b) != 0
}

func isPerfInstalled() bool {
	cmd := exec.Command("perf", "--version")
	b, _ := cmd.Output()

	return len(b) != 0
}
