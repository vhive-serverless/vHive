// MIT License
//
// Copyright (c) 2021 Yuchen Niu and EASE lab
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

package profile

import (
	"bufio"
	"encoding/csv"
	"errors"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Profiler a instance of toplev command
type Profiler struct {
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
func NewProfiler(executionTime float64, printInterval uint64, level int, nodes, outFile string, socket, cpu int) (*Profiler, error) {
	profiler := new(Profiler)
	profiler.execTime = executionTime
	profiler.interval = printInterval
	profiler.cores = make(map[string]bool)
	profiler.bottlenecks = make(map[string]float64)

	if outFile == "" {
		outFile = "profile"
	}
	profiler.outFile = outFile + ".csv"

	profiler.cmd = exec.Command("/usr/local/pmu-tools/toplev",
		"-v",
		"--no-desc",
		"-x", ",",
		"-l", strconv.Itoa(level),
		"-I", strconv.FormatUint(printInterval, 10),
		"-o", profiler.outFile)

	if cpu > -1 {
		cpus, err := GetCPUInfo()
		if err != nil {
			return nil, err
		}
		core, err := cpus.GetSocketCoreInString(cpu)
		if err != nil {
			return nil, err
		}
		profiler.cmd.Args = append(profiler.cmd.Args, "--core", core)
	} else {
		if socket > -1 {
			// monitor the input socket only
			profiler.cmd.Args = append(profiler.cmd.Args, "--core", "S"+strconv.Itoa(socket))
		}
		// hide idle CPUs that are <50% of busiest.
		profiler.cmd.Args = append(profiler.cmd.Args, "--idle-threshold", "50")
	}

	// pass `profilerNodes` to pmu-tool if it is not empty, it controls specific metric/metrics to profile.
	if nodes != "" {
		profiler.cmd.Args = append(profiler.cmd.Args, "--nodes", nodes)
	}

	profiler.cmd.Args = append(profiler.cmd.Args, "sleep", strconv.FormatFloat(executionTime, 'f', -1, 64))

	log.Debugf("Profiler command: %s", profiler.cmd)

	return profiler, nil
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
		log.Warn("print interval < 100ms. The overhead may be high in some cases. Please proceed with caution.")
	}

	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.tStart = time.Now()

	return nil
}

// SetWarmUpTime sets the time duration until system is warm.
func (p *Profiler) SetWarmUpTime() {
	p.warmTime = time.Since(p.tStart).Seconds()

	if p.execTime > 0 && p.warmTime > p.execTime {
		log.Warn("System warmup time is longer than perf execution time.")
	}
}

// GetWarmUpTime returns the time duration until system is warm.
func (p *Profiler) GetWarmUpTime() float64 {
	return p.warmTime
}

// SetCoolDownTime sets the time duration until system starts tearing down.
func (p *Profiler) SetCoolDownTime() {
	p.tearDownTime = time.Since(p.tStart).Seconds()
}

// GetCoolDownTime returns the time duration until system starts tearing down.
func (p *Profiler) GetCoolDownTime() float64 {
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

	log.Debugf("Warm time since start: %.2fs, Teardown time since start: %.2fs", p.warmTime, p.tearDownTime)
	return p.readCSV()
}

// PrintBottlenecks prints the bottlenecks during profiling
func (p *Profiler) PrintBottlenecks() {
	for k, v := range p.bottlenecks {
		log.Infof("Bottleneck %s with value %.2f", k, v)
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
	reader.Comment = '#'
	lines, err := reader.ReadAll()
	headers := lines[0]
	if err != nil {
		return nil, err
	}
	headerIdxMap := headerPos(headers)
	for _, line := range lines[1:] {
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
	cmd := exec.Command("/usr/local/pmu-tools/toplev", "--version")
	b, err := cmd.Output()
	if err != nil {
		log.Error(err)
	}

	return len(b) != 0
}

func isPerfInstalled() bool {
	cmd := exec.Command("perf", "--version")
	b, err := cmd.Output()
	if err != nil {
		log.Error(err)
	}

	return len(b) != 0
}

// CPUInfo contains sockets and processor to socket and core map
type CPUInfo struct {
	sockets    []socket
	processors map[int]processor
}

// socket contains its cores ID
type socket struct {
	cores []core
}

// core contains its processors ID
type core struct {
	processors []int
}

// processor contains its socket ID and physical core ID
type processor struct {
	socket int
	core   int
}

// GetCPUInfo returns a instance of CPUInfo
func GetCPUInfo() (CPUInfo, error) {
	var (
		procID, socketID int
		cpuInfo          CPUInfo
	)

	cpuInfo.processors = make(map[int]processor)

	file, err := os.Open("/proc/cpuinfo")
	if err != nil {
		return cpuInfo, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "processor") {
			procID, err = strconv.Atoi(strings.TrimSpace(strings.Split(line, ":")[1]))
			if err != nil {
				return cpuInfo, err
			}
		} else if strings.HasPrefix(line, "physical id") {
			socketID, err = strconv.Atoi(strings.TrimSpace(strings.Split(line, ":")[1]))
			if err != nil {
				return cpuInfo, err
			}
			for i := len(cpuInfo.sockets); i <= socketID; i++ {
				cpuInfo.sockets = append(cpuInfo.sockets, socket{})
			}
		} else if strings.HasPrefix(line, "core id") {
			coreID, err := strconv.Atoi(strings.TrimSpace(strings.Split(line, ":")[1]))
			if err != nil {
				return cpuInfo, err
			}
			for i := len(cpuInfo.sockets[socketID].cores); i <= coreID; i++ {
				cpuInfo.sockets[socketID].cores = append(cpuInfo.sockets[socketID].cores, core{})
			}
			cpuInfo.sockets[socketID].cores[coreID].processors = append(cpuInfo.sockets[socketID].cores[coreID].processors, procID)
			cpuInfo.processors[procID] = processor{socket: socketID, core: coreID}
		}
	}

	if err := scanner.Err(); err != nil {
		return cpuInfo, err
	}

	return cpuInfo, nil
}

// GetSocketCoreInString returns the physical core ID in Sx-Cx format
func (c *CPUInfo) GetSocketCoreInString(processor int) (string, error) {
	proc, isPresent := c.processors[processor]
	if !isPresent {
		return "", errors.New("processor is not found")
	}
	return "S" + strconv.Itoa(proc.socket) + "-" + "C" + strconv.Itoa(proc.core), nil
}

// GetSocketID returns the socket ID
func (c *CPUInfo) GetSocketID(processor int) (int, error) {
	proc, isPresent := c.processors[processor]
	if !isPresent {
		return 0, errors.New("processor is not found")
	}
	return proc.socket, nil
}

// GetCoreID returns the physical core ID
func (c *CPUInfo) GetCoreID(processor int) (int, error) {
	proc, isPresent := c.processors[processor]
	if !isPresent {
		return 0, errors.New("processor is not found")
	}
	return proc.core, nil
}

// GetSibling returns the sibling processor
func (c *CPUInfo) GetSibling(processor int) (int, error) {
	proc, isPresent := c.processors[processor]
	if !isPresent {
		return 0, errors.New("processor is not found")
	}

	core := c.sockets[proc.socket].cores[proc.core]
	if len(core.processors) == 1 {
		return -1, errors.New("processor does not have a sibling")
	}

	if core.processors[0] == processor {
		return core.processors[1], nil
	}

	return core.processors[0], nil
}

// SocketCPUs returns a list of processors of the socket
func (c *CPUInfo) SocketCPUs(socket int) ([]int, error) {
	var result []int
	if socket >= len(c.sockets) || socket < 0 {
		return nil, errors.New("socket ID is out of bound")
	}

	for _, core := range c.sockets[socket].cores {
		for _, proc := range core.processors {
			result = append(result, proc)
		}
	}
	sort.Ints(result)
	return result, nil
}

// NumSocket returns the number of sockets.
func (c *CPUInfo) NumSocket() int {
	return len(c.sockets)
}

// AllCPUs returns the list of all logcial CPUs.
func (c *CPUInfo) AllCPUs() []int {
	var result []int

	for _, socket := range c.sockets {
		for _, core := range socket.cores {
			for _, proc := range core.processors {
				result = append(result, proc)
			}
		}
	}
	sort.Ints(result)
	return result
}

// NumCPU returns the number of logcial CPUs.
func (c *CPUInfo) NumCPU() int {
	return len(c.processors)
}
