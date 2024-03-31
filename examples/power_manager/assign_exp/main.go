package powermanager

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	powermanager "github.com/vhive-serverless/vhive/examples/power_manager"
)

var (
	serviceAssignment = map[string]bool{
		"spinning-go": false,
		"sleeping-go": false,
		"aes-python":  false,
		"auth-python": false,
	}
)

func processLatencies(records []int64, serviceName string) {
	if len(records) == 0 {
		fmt.Println("No data to process")
		return
	}

	fifthPercentile := powermanager.GetDataAtPercentile(records, 5)
	ninetiethPercentile := powermanager.GetDataAtPercentile(records, 90)
	difference := float64(ninetiethPercentile-fifthPercentile) / float64(fifthPercentile)
	if difference >= 0.40 && !serviceAssignment[serviceName] { // Assign to high performance class
		fmt.Println("Assigning to high performance class")
		command := fmt.Sprintf("kubectl patch service.serving.knative.dev %s --type merge --patch '{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"loader-nodetype\":\"worker-high\"}}}}}' --namespace default", serviceName)
		cmd := exec.Command("bash", "-c", command)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("Error assigning to high performance class: %+v", err))
			return
		}
		serviceAssignment[serviceName] = true
	}
	if difference < 0.10 && !serviceAssignment[serviceName] { // Assign to low performance class
		fmt.Println("Assigning to low performance class")
		command := fmt.Sprintf("kubectl patch service.serving.knative.dev %s --type merge --patch '{\"spec\":{\"template\":{\"spec\":{\"nodeSelector\":{\"loader-nodetype\":\"worker-low\"}}}}}' --namespace default", serviceName)
		cmd := exec.Command("bash", "-c", command)
		_, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("Error assigning to low performance class: %+v", err))
			return
		}
		serviceAssignment[serviceName] = true
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
	go powermanager.WriteToCSV(writer, ch, &wg)
	go assignWorkload(ch_latency_spinning, "spinning-go", &wg)
	go assignWorkload(ch_latency_sleeping, "sleeping-go", &wg)

	now := time.Now()
	for time.Since(now) < (time.Minute * 5) {
		go powermanager.InvokeConcurrently(5, powermanager.SleepingURL, ch, ch_latency_spinning, ch_latency_sleeping, false)
		go powermanager.InvokeConcurrently(5, powermanager.SpinningURL, ch, ch_latency_spinning, ch_latency_sleeping, true)

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
