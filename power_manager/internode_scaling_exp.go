package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	SpinningURL       = "spinning-go.default.192.168.1.240.sslip.io"
	SleepingURL       = "sleeping-go.default.192.168.1.240.sslip.io"
	AesURL            = "aes-python.default.192.168.1.240.sslip.io"
	AuthURL           = "auth-python.default.192.168.1.240.sslip.io"
	ServiceAssignment = map[string]bool{
		"spinning-go": false,
		"sleeping-go": false,
		"aes-python":  false,
		"auth-python": false,
	}
)

func invoke(n int, url string, ch chan<- []string, ch_latency_spinning chan<- int64, ch_latency_sleeping chan<- int64, spinning bool) {
	for i := 0; i < n; i++ {
		go func() {
			command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)
			startInvoke := time.Now().UTC().UnixMilli()
			cmd := exec.Command("bash", "-c", command)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERR2: %+v", err))
				return
			}
			endInvoke := time.Now().UTC().UnixMilli()
			latency := endInvoke - startInvoke
			if spinning {
				ch_latency_spinning <- latency
				ch <- []string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), strconv.FormatInt(latency, 10), "-"}
			} else {
				ch_latency_sleeping <- latency
				ch <- []string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), "-", strconv.FormatInt(latency, 10)}
			}
		}()
	}
}

func writeToCSV(writer *csv.Writer, ch <-chan []string, wg *sync.WaitGroup) {
	defer wg.Done()
	for record := range ch {
		if err := writer.Write(record); err != nil {
			fmt.Printf("Error writing to CSV file: %v\n", err)
		}
	}
}

func assignWorkload(ch_latency <-chan int64, serviceName string, wg *sync.WaitGroup) {
	defer wg.Done()

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	var records []int64

	for {
		select {
		case record, ok := <-ch_latency:
			if !ok {
				// Channel is closed, process remaining data
				processLatencies(records, serviceName)
				return
			}
			records = append(records, record)
		case <-ticker.C:
			// Time to process the data
			processLatencies(records, serviceName)
		}
	}
}

func processLatencies(records []int64, serviceName string) {
	if len(records) == 0 {
		fmt.Println("No data to process")
		return
	}

	fifthPercentile := percentile(records, 5)
	ninetiethPercentile := percentile(records, 95)
	difference := float64(ninetiethPercentile-fifthPercentile) / float64(fifthPercentile)
	fmt.Println(serviceName, difference)
	if difference > 0.30 && !ServiceAssignment[serviceName] { // Assign to high performance class
		fmt.Println("Assigning to high performance class")
		command := fmt.Sprintf("kubectl patch service.serving.knative.dev %s --type merge --patch '{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"loader-nodetype\":\"worker-high\"}}}}}' --namespace default", serviceName)
		cmd := exec.Command("bash", "-c", command)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR3: %+v", err))
			return
		}
		ServiceAssignment[serviceName] = true
	}
	if difference < 0.10 && !ServiceAssignment[serviceName] { // Assign to low performance class
		fmt.Println("Assigning to low performance class")
		command := fmt.Sprintf("kubectl patch service.serving.knative.dev %s --type merge --patch '{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"loader-nodetype\":\"worker-low\"}}}}}' --namespace default", serviceName)
		cmd := exec.Command("bash", "-c", command)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR3: %+v", err))
			return
		}
		ServiceAssignment[serviceName] = true
	}
}

func percentile(data []int64, p float64) int64 {
	if len(data) == 0 {
		return 0
	}
	sort.Slice(data, func(i, j int) bool { return data[i] < data[j] })
	n := (p / 100) * float64(len(data)-1)
	index := int(n)

	if index < 0 {
		index = 0
	} else if index >= len(data) {
		index = len(data) - 1
	}
	return data[index]
}

func main() {
	file, err := os.Create("metrics2.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write(append([]string{"startTime", "endTime", "spinningLatency", "sleepingLatency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}

	ch := make(chan []string)
	ch_latency_spinning := make(chan int64)
	ch_latency_sleeping := make(chan int64)

	var wg sync.WaitGroup
	wg.Add(3)
	go writeToCSV(writer, ch, &wg)
	go assignWorkload(ch_latency_spinning, "spinning-go", &wg)
	go assignWorkload(ch_latency_sleeping, "sleeping-go", &wg)

	now := time.Now()
	for time.Since(now) < (time.Minute * 5) {
		go invoke(5, SleepingURL, ch, ch_latency_spinning, ch_latency_sleeping, false)
		go invoke(5, SpinningURL, ch, ch_latency_spinning, ch_latency_sleeping, true)

		time.Sleep(1 * time.Second) // Wait for 1 second before invoking again
	}
	close(ch)
	close(ch_latency_spinning)
	close(ch_latency_sleeping)
	wg.Wait()

	err = writer.Write(append([]string{"-", "-", "-", "-"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}
	fmt.Println("done")
}
