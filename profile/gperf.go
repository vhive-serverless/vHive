package profile

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

// Common flags of perf
const (
	AllCPUs  = "-a"
	OutFile  = "-o"
	Detailed = "-d"
	Event    = "-e"
	SEP      = "-x"
)

// PerfStat A instance of perf stat command
type PerfStat struct {
	cmd     *exec.Cmd
	proc    int
	sep     string
	outFile string
}

// NewPerfStat returns a new instance for perf stat
func NewPerfStat(arguments ...string) *PerfStat {
	perfStat := new(PerfStat)

	var args, delimiter string
	for i, arg := range arguments {
		args += delimiter + arg
		delimiter = " "

		switch arg {
		case OutFile:
			perfStat.outFile = arguments[i+1]
		case SEP:
			perfStat.sep = arguments[i+1]
		}
	}

	if perfStat.sep == "" {
		perfStat.sep = "|"
		args += delimiter + SEP + delimiter + "\\" + perfStat.sep
	}

	args += delimiter + "&"

	commandString := fmt.Sprintf("perf stat %s", args)
	perfStat.cmd = exec.Command("/bin/sh", "-c", commandString) // TODO: FIX Perf not running
	log.Debugf("Perf command: %s", perfStat.cmd)

	return perfStat
}

// Run runs perf stat command, assumes only one perf process running
func (p *PerfStat) Run() error {
	log.Debug("Perf starts")
	if err := p.cmd.Run(); err != nil {
		return err
	}

	// get pid
	pidBytes, err := exec.Command("pidof", "perf").Output()
	if err != nil {
		log.Warnf("Failed to get perf PID: %v", err)
		return err
	}
	pid, err := strconv.Atoi(strings.TrimSpace(strings.Split(string(pidBytes), "\n")[0]))
	if err != nil {
		log.Warnf("Pid conversion failed: %v", err)
		return err
	}

	p.proc = pid

	time.Sleep(100 * time.Millisecond) // Wait for perf completely start
	return nil
}

// Stop stops perf stat command and returns the result in map[event]value
func (p *PerfStat) Stop() (map[string]float64, error) {
	log.Debug("Perf stops")

	// interrupt perf process
	log.Debugf("perf process: %d", p.proc)
	cmd := exec.Command("kill", "-2", strconv.Itoa(p.proc))
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	time.Sleep(1 * time.Second) // wait for perf write result into the file.

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
