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

func NewProfiler(executionTime float64, printInterval uint64, level int, nodes, outFile string) *Profiler {
	profiler := new(Profiler)
	profiler.execTime = executionTime
	profiler.interval = printInterval
	profiler.cores = make(map[string]bool)
	profiler.bottlenecks = make(map[string]float64)

	if outFile == "" {
		outFile = "profile"
	}
	profiler.outFile = outFile + ".csv"

	profiler.cmd = exec.Command("toplev.py", "-v", "--no-desc", "--no-util",
		"-x", ",",
		"--idle-threshold", "30",
		"-l", strconv.Itoa(level),
		"-I", strconv.FormatUint(printInterval, 10),
		"-o", profiler.outFile)

	if nodes != "" {
		profiler.cmd.Args = append(profiler.cmd.Args, "--nodes", nodes)
	}

	profiler.cmd.Args = append(profiler.cmd.Args, "sleep", strconv.FormatFloat(executionTime, 'f', -1, 64))

	log.Infof("Profiler command: %s", profiler.cmd)

	return profiler
}

func (p *Profiler) Run() error {
	if !isPerfInstalled() {
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

func (p *Profiler) SetWarmTime() {
	p.warmTime = time.Since(p.tStart).Seconds()

	if p.execTime > 0 && p.warmTime > p.execTime {
		log.Warn("System warmup time is longer than perf execution time.")
	}
}

func (p *Profiler) GetWarmupTime() float64 {
	return p.warmTime
}

func (p *Profiler) SetTearDownTime() {
	p.teardown.Do(func() {
		p.tearDownTime = time.Since(p.tStart).Seconds()
	})
}

func (p *Profiler) GetTearDownTime() float64 {
	return p.tearDownTime
}

func (p *Profiler) GetResult() (map[string]float64, error) {
	if p.tStart.IsZero() {
		return nil, errors.New("pmu tool was not executed, run it first")
	}

	// wait for profiler command finish
	timeLeft := (p.execTime - time.Since(p.tStart).Seconds()) * 1e+9
	time.Sleep(time.Duration(timeLeft))

	log.Debugf("Warm time: %f, Teardown time: %f", p.warmTime, p.tearDownTime)
	return p.readCSV()
}

func (p *Profiler) PrintBottlenecks() {
	for k, v := range p.bottlenecks {
		log.Infof("Bottleneck %s with value %f", k, v)
	}
}

func (p *Profiler) GetCores() map[string]bool {
	return p.cores
}

// PMU Tool

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
	reader.Read() // skip headers
	for {
		line, err := reader.Read()
		if err == io.EOF {
			break
		}
		if strings.HasPrefix(line[0], "#") {
			break
		}
		record, err := p.splitLine(line)
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

	// if err := os.Remove(p.outFile); err != nil {
	// 	return nil, err
	// }
	if len(records) == 0 {
		return nil, errors.New("empty file")
	}

	return p.parseMetric(records), nil
}

func (p *Profiler) splitLine(line []string) (pmuLine, error) {
	ts, err := strconv.ParseFloat(line[0], 64)
	if err != nil {
		return pmuLine{}, err
	}

	if ts < p.warmTime {
		return pmuLine{}, nil
	} else if ts > p.tearDownTime {
		return pmuLine{timestamp: -1}, nil
	}

	v, err := strconv.ParseFloat(line[3], 64)
	if err != nil {
		return pmuLine{}, err
	}
	data := pmuLine{
		timestamp:    ts,
		cpu:          line[1],
		area:         line[2],
		value:        v,
		unit:         line[4],
		isBottleneck: line[9] != "",
	}

	return data, nil
}

func (p *Profiler) parseMetric(lines []pmuLine) map[string]float64 {
	var (
		epoch    float64
		prevTime float64
		prevCPU  string
		results  = make(map[string]float64)
		initTime = lines[0].timestamp
		initCPU  = lines[0].cpu
	)
	// TODO: measured core number == vm number
	for _, line := range lines {
		if line.timestamp != prevTime || line.cpu != prevCPU {
			epoch++
		}
		if line.timestamp == initTime && line.cpu == initCPU {
			results[line.area] = line.value
		} else {
			results[line.area] += line.value
		}
		prevTime, prevCPU = line.timestamp, line.cpu
		p.cores[line.cpu] = true
	}
	for k, v := range results {
		results[k] = v / epoch
	}
	for k := range p.bottlenecks {
		p.bottlenecks[k] = results[k]
	}
	return results
}

func isInstalled() bool {
	cmd := exec.Command("toplev.py", "--version")
	b, _ := cmd.Output()

	return len(b) != 0
}
