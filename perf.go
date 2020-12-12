package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
)

const (
	AllCPUs  = "-a"
	Output   = "-o"
	Detailed = "-d"
	Event    = "-e"
	SEP      = "-x"
)

// PerfStat A instance of perf stat command
type PerfStat struct {
	args    string
	sep     string
	outFile string
	run     chan bool
	stop    chan bool
}

// NewPerfStat returns a new instance for perf stat
func NewPerfStat(args ...string) *PerfStat {
	perfStat := new(PerfStat)
	perfStat.run = make(chan bool, 1)
	perfStat.stop = make(chan bool, 1)
	perfStat.args = ""
	perfStat.sep = "|"

	delimiter := ""
	for i, arg := range args {
		perfStat.args += delimiter + arg
		delimiter = " "

		switch arg {
		case Output:
			perfStat.outFile = args[i+1]
		case SEP:
			perfStat.sep = args[i+1]
		}
	}

	if perfStat.sep == "" {
		perfStat.sep = "|"
		perfStat.args += delimiter + SEP + perfStat.sep
	}

	go func() {
		for {
			select {
			case <-perfStat.run:
				commandString := fmt.Sprintf("perf stat %s", perfStat.args)
				log.Debugf("Perf command: %s", commandString)
				cmd := exec.Command("sudo", "/bin/sh", "-c", commandString) // TODO: FIX Perf not running

				if err := cmd.Run(); err != nil {
					log.Fatalf("Perf stat returned error %v", err)
				}
			case <-perfStat.stop:
				return
			}
		}
	}()

	return perfStat
}

// RunPerfStat runs perf stat command
func (p *PerfStat) RunPerfStat() {
	p.run <- true
}

// StopPerfStat stops perf stat command and returns the result in map[event]value
func (p *PerfStat) StopPerfStat() map[string]float64 {
	p.stop <- true
	return p.parseResult()
}

func (p *PerfStat) parseResult() map[string]float64 {
	result := make(map[string]float64)

	file, err := os.Open(p.outFile)
	if err != nil {
		log.Fatal(err)
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
			log.Fatal(err)
		}
		result[eventName] = value
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

	return result
}
