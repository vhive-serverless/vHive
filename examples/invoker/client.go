// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov and EASE lab
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

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	log "github.com/sirupsen/logrus"
	pb "github.com/ustiugov/fccd-orchestrator/helloworld"
	"google.golang.org/grpc"
)

var (
	completed int64
	latSlice  LatencySlice
)

func main() {
	urlFile := flag.String("urlFile", "urls.txt", "File with functions' URLs")
	rps := flag.Int("rps", 1, "Target requests per second")
	runDuration := flag.Int("time", 5, "Run the benchmark for X seconds")
	latencyOutputFile := flag.String("latf", "lat.csv", "CSV file for the latency measurements in microseconds")

	flag.Parse()

	log.Infof("Reading the URLs from the file: %s", *urlFile)

	urls, err := readLines(*urlFile)
	if err != nil {
		log.Fatal("Failed to read the URL files:", err)
	}

	realRPS := runBenchmark(urls, *runDuration, *rps)

	writeLatencies(realRPS, *latencyOutputFile)
}

func readLines(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

func runBenchmark(urls []string, runDuration, targetRPS int) (realRPS float64) {
	timeout := time.After(time.Duration(runDuration) * time.Second)
	tick := time.Tick(time.Duration(1000/targetRPS) * time.Millisecond)

	var issued int
	start := time.Now()

	for {
		select {
		case <-timeout:
			duration := time.Since(start).Seconds()
			realRPS = float64(completed) / duration
			log.Infof("Real / target RPS : %.2f / %v", realRPS, targetRPS)

			log.Println("Benchmark finished!")

			return
		case <-tick:
			url := urls[issued%len(urls)]
			go invokeFunction(url)

			issued++
		}
	}
}

func invokeFunction(url string) {
	defer getDuration(startMeasurement(url)) // measure entire invocation time

	address := fmt.Sprintf("%s:%d", url, 80)

	conn, err := grpc.Dial(address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewGreeterClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = c.SayHello(ctx, &pb.HelloRequest{Name: "faas"})
	if err != nil {
		log.Warnf("Failed to invoke %v, err=%v", address, err)
	}

	atomic.AddInt64(&completed, 1)

	return
}

// LatencySlice is a thread-safe slice to hold a slice of latency measurements.
type LatencySlice struct {
	sync.Mutex
	slice []int64
}

func startMeasurement(msg string) (string, time.Time) {
	return msg, time.Now()
}

func getDuration(msg string, start time.Time) {
	latency := time.Since(start).Microseconds()
	log.Debugf("Invoked %v in %v usec\n", msg, latency)

	latSlice.Lock()
	latSlice.slice = append(latSlice.slice, latency)
	latSlice.Unlock()
}

func writeLatencies(rps float64, latencyOutputFile string) {
	latSlice.Lock()
	defer latSlice.Unlock()

	fileName := fmt.Sprintf("%s_%frps", latencyOutputFile, rps)
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_WRONLY, 0644)

	if err != nil {
		log.Fatalf("failed creating file: %s", err)
	}

	datawriter := bufio.NewWriter(file)

	for _, lat := range latSlice.slice {
		_, err := datawriter.WriteString(strconv.FormatInt(lat, 10) + "\n")
		if err != nil {
			log.Fatal("Failed to write the URLs to a file ", err)
		}
	}

	datawriter.Flush()
	file.Close()
	return
}
