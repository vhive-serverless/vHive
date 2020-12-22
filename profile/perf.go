package profile

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// PerfStat A instance of perf stat command
type PerfStat struct {
	cmd      *exec.Cmd
	tStart   time.Time
	execTime float64
	outFile  string
	sep      string
}

// NewPerfStat returns a new instance for perf stat
func NewPerfStat(events string, executionTime, repeat float64) *PerfStat {
	perfStat := new(PerfStat)
	perfStat.sep = "|"
	perfStat.outFile = "perf-tmp.data"
	perfStat.execTime = executionTime

	perfStat.cmd = exec.Command("perf", "stat", "-a",
		"-e", events,
		"-r", strconv.FormatFloat(repeat, 'f', -1, 64),
		"-x", perfStat.sep,
		"-o", perfStat.outFile,
		"--", "sleep", strconv.FormatFloat((executionTime/repeat), 'f', -1, 64))
	log.Debugf("Perf command: %s", perfStat.cmd)

	return perfStat
}

// Run executes perf stat command
func (p *PerfStat) Run() error {
	if err := p.cmd.Start(); err != nil {
		return err
	}
	p.tStart = time.Now()

	return nil
}

// GetResult returns the counters of perf stat
func (p *PerfStat) GetResult() (map[string]float64, error) {
	if p.tStart.IsZero() {
		return nil, errors.New("perf was not executed")
	}

	// wait for perf command finish
	timeLeft := p.execTime - time.Since(p.tStart).Seconds()
	time.Sleep(time.Duration(timeLeft + 1))

	return p.parseResult()
}

func (p *PerfStat) parseResult() (map[string]float64, error) {
	result := make(map[string]float64)

	file, err := os.Open(p.outFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()
	scanner.Scan()
	for scanner.Scan() {
		line := scanner.Text()
		tokens := strings.Split(line, p.sep+p.sep)
		eventName := strings.Split(tokens[1], p.sep)[0]
		valueStr := tokens[0]
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return nil, err
		}
		result[eventName] = value
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return result, nil
}
