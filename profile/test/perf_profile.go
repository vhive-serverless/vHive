package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	log "github.com/sirupsen/logrus"
	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

var (
	perfEvents = flag.String("events", "", "Perf events")
	funcNames  = flag.String("functions", "helloworld", "Name of the functions to benchmark")
)

func main() {
	// test args
	// *userPerfEvents = "instructions,LLC-loads,LLC-load-misses"
	// *userFuncNames = "rnn_serving"

	flag.Parse()
	createResultCSV()

	for i := 4; i <= 8; i += 4 {
		setupBenchRPS()
		runTestBenchRequestPerSecond(i)
		teardownBenchRPS()
	}

	plotResult()
}

func setupBenchRPS() {
	// sudo mkdir -m777 -p /tmp/ctrd-logs && sudo env "PATH=$PATH" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &
	const ctrdLogDir = "/tmp/ctrd-logs"

	if err := os.MkdirAll(ctrdLogDir, 0777); err != nil {
		log.Fatalf("Failed creating directory: %v\n", err)
	}

	envPath := "PATH=" + os.Getenv("PATH")
	commandString := "sudo env " + envPath + " /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &"
	cmd := exec.Command("sudo", "/bin/sh", "-c", commandString)

	if err := cmd.Run(); err != nil {
		log.Fatalf("Setup benchmark error: %v\n", err)
	}
}

func runTestBenchRequestPerSecond(vmNum int) {
	envPath := "PATH=" + os.Getenv("PATH")
	commandString := fmt.Sprintf("sudo env %s go test -v -run TestBenchRequestPerSecond -args -vm %d -funcNames %s -perfEvents %s",
		envPath,
		vmNum,
		*funcNames,
		*perfEvents)
	cmd := exec.Command("sudo", "/bin/sh", "-c", commandString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		log.Fatalf("Benchmark execution error: %v\n", err)
	}
}

func teardownBenchRPS() {
	// ./scripts/clean_fcctr.sh
	cmd := exec.Command("sudo", "/bin/sh", "-c", "./scripts/clean_fcctr.sh")

	if err := cmd.Run(); err != nil {
		log.Fatalf("Teardown benchmark error: %v\n", err)
	}

	// remove perf tmp file
	if err := os.Remove("perf-tmp.data"); err != nil {
		log.Warnf("Remove file error: %v", err)
	}
}

func createResultCSV() {
	f, err := os.Create("benchRPS.csv")
	if err != nil {
		log.Fatalf("Failed creating file: %v", err)
	}
	defer f.Close()

	writer := csv.NewWriter(f)

	// write titles
	titles := []string{"average-execution-time", "real-requests-per-second"}
	events := strings.Split(*perfEvents, ",")
	for _, e := range events {
		titles = append(titles, e)
	}

	writer.Write(titles)
	writer.Flush()
}

func plotResult() {
	var (
		records = readResultCSV()
		rows    = len(records)
		cols    = len(records[0])
	)

	for col := 0; col < cols; col++ {
		// create a new plot for a metric
		p, err := plot.New()
		if err != nil {
			log.Fatalf("Failed creating plot: %v", err)
		}

		p.X.Label.Text = "VM number"
		p.Y.Label.Text = records[0][col]

		// setup data
		pts := make(plotter.XYs, rows-1)
		vmNum := 4
		for row := 1; row < rows; row++ {
			pts[row-1].X = float64(vmNum)
			value, err := strconv.ParseFloat(records[row][col], 64)
			if err != nil {
				log.Fatalf("Failed parsing string to float: %v", err)
			}
			pts[row-1].Y = value
			vmNum += 4
		}

		// plot
		err = plotutil.AddLinePoints(p, pts)
		if err != nil {
			log.Fatalf("Failed plotting data: %v", err)
		}

		// save plot
		fileName := p.Y.Label.Text + ".png"
		if err := p.Save(4*vg.Inch, 4*vg.Inch, fileName); err != nil {
			log.Fatalf("Failed saving plot: %v", err)
		}
	}
}

// retrieve data from csv file
func readResultCSV() [][]string {
	f, err := os.Open("benchRPS.csv")
	if err != nil {
		log.Fatalf("Failed opening file: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)

	records, err := r.ReadAll()
	if err != nil {
		log.Fatalf("Failed reading file: %v", err)
	}

	return records
}
