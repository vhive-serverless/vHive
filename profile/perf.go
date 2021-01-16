package profile

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// PerfStat A instance of perf stat command
type PerfStat struct {
	once         sync.Once
	cmd          *exec.Cmd
	tStart       time.Time
	interval     uint64
	execTime     float64
	warmTime     float64
	tearDownTime float64
	outFile      string
	sep          string
	metrics      []string
	eventSet     map[string]float64
}

// NewPerfStat returns a new instance for perf stat
func NewPerfStat(executionTime float64, printInterval uint64, eventStr, metricStr, outFile string) *PerfStat {
	perfStat := new(PerfStat)
	perfStat.sep = "|"
	perfStat.outFile = outFile
	perfStat.execTime = executionTime
	perfStat.interval = printInterval
	if outFile == "" {
		perfStat.outFile = "perf-tmp.data"
	}

	perfStat.cmd = exec.Command("perf", "stat", "-a",
		"-I", strconv.FormatUint(printInterval, 10),
		"-x", perfStat.sep,
		"-o", perfStat.outFile)

	// create a events set from events and metrics
	perfStat.eventSet = make(map[string]float64)
	if len(eventStr) > 0 {
		events := strings.Split(eventStr, ",")
		for _, e := range events {
			perfStat.eventSet[e] = 0
		}
	}

	// break metrics into events
	if len(metricStr) > 0 {
		perfStat.metrics = strings.Split(metricStr, ",")
		for _, metric := range perfStat.metrics {
			metricEvents, err := getEvents(metric)
			if err != nil {
				log.Warnf("skip invalid matric %s: %v", metric, err)
				continue
			}
			for _, e := range metricEvents {
				perfStat.eventSet[e] = 0
			}
		}
	}

	var delimiter, newEventStr string
	for event := range perfStat.eventSet {
		newEventStr += delimiter + event
		delimiter = ","
	}
	if newEventStr != "" {
		perfStat.cmd.Args = append(perfStat.cmd.Args, "-e", newEventStr)
	}

	perfStat.cmd.Args = append(perfStat.cmd.Args, "--", "sleep", strconv.FormatFloat(executionTime, 'f', -1, 64))

	log.Debugf("Perf command: %s", perfStat.cmd)

	return perfStat
}

// Run executes perf stat command
func (p *PerfStat) Run() error {
	if !isPerfInstalled() {
		return errors.New("perf is not installed")
	}

	if p.execTime < 0 {
		return errors.New("perf execution time is less than 0s")
	}

	if p.interval < 10 {
		return errors.New("perf print interval is less than 10ms")
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
func (p *PerfStat) SetWarmTime() {
	p.once.Do(func() {
		p.warmTime = time.Since(p.tStart).Seconds()

		if p.execTime > 0 && p.warmTime > p.execTime {
			log.Warn("System warmup time is longer than perf execution time.")
		}
	})
}

// SetTearDownTime sets the time duration until system tears down.
func (p *PerfStat) SetTearDownTime() {
	p.tearDownTime = time.Since(p.tStart).Seconds()
}

// GetResult returns the counters of perf stat
func (p *PerfStat) GetResult() (map[string]float64, error) {
	if p.tStart.IsZero() {
		return nil, errors.New("Perf was not executed, run perf first")
	}

	// wait for perf command finish
	timeLeft := (p.execTime - time.Since(p.tStart).Seconds()) * 1e+9
	time.Sleep(time.Duration(timeLeft))

	log.Debugf("Warm time: %f, Teardown time: %f", p.warmTime, p.tearDownTime)
	return p.parseResult()
}

func (p *PerfStat) parseResult() (map[string]float64, error) {
	data, err := p.readPerfData()
	if err != nil {
		return nil, err
	}

	for _, m := range data {
		for k, v := range m {
			p.eventSet[k] += v
		}
	}

	for k := range p.eventSet {
		p.eventSet[k] /= float64(len(data))
	}

	for _, metric := range p.metrics {
		events, err := getEvents(metric)
		if err != nil {
			return nil, err
		}

		var params []float64
		for _, event := range events {
			params = append(params, p.eventSet[event])
		}

		result, err := calculateMetric(metric, params...)
		if err != nil {
			return nil, err
		}

		p.eventSet[metric] = result
	}

	return p.eventSet, nil
}

func (p *PerfStat) readPerfData() ([]map[string]float64, error) {
	file, err := os.Open(p.outFile)
	if err != nil {
		return nil, errors.New("Perf was failed to execute, check perf events")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	scanner.Scan()

	var (
		prevTimestamp float64
		results       []map[string]float64
		epoch         = make(map[string]float64)
	)
	for scanner.Scan() {
		line := scanner.Text()
		tokens := strings.Split(line, p.sep+p.sep)

		timeStr := strings.ReplaceAll(strings.Split(tokens[0], p.sep)[0], " ", "")
		timestamp, err := strconv.ParseFloat(timeStr, 64)
		if err != nil {
			return nil, err
		}

		// omitting warm and tear down period
		if timestamp < p.warmTime {
			continue
		} else if timestamp > p.tearDownTime {
			break
		} else if prevTimestamp != 0 && timestamp != prevTimestamp {
			results = append(results, epoch)
			epoch = make(map[string]float64)
		}

		eventName := strings.Split(tokens[1], p.sep)[0]

		valueStr := strings.Split(tokens[0], p.sep)[1]
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return nil, err
		}

		epoch[eventName] = value
		prevTimestamp = timestamp
	}
	results = append(results, epoch)

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	if err := os.Remove(p.outFile); err != nil {
		return nil, err
	}

	return results, nil
}

func isPerfInstalled() bool {
	cmd := exec.Command("perf", "--version")
	b, _ := cmd.Output()

	return len(b) != 0
}
