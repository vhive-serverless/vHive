package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"sync"
	"time"

	powermanager "github.com/vhive-serverless/vhive/power_manager"
)

func main() {
	file, err := os.Create("metrics1.csv")
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

	frequencies := map[string]int64{
		powermanager.LowFrequencyPowerProfile:  1200,
		powermanager.HighFrequencyPowerProfile: 2400,
	} // for 50/50, need to manually tune the frequency of the individual node
	for powerProfile, freq := range frequencies {
		err := powermanager.SetPowerProfileToNode(powerProfile, powermanager.Node1Name, freq)
		if err != nil {
			fmt.Printf(fmt.Sprintf("Error setting up power profile for node1: %+v", err))
		}
		err = powermanager.SetPowerProfileToNode(powerProfile, powermanager.Node2Name, freq)
		if err != nil {
			fmt.Printf(fmt.Sprintf("Error setting up power profile for node2: %+v", err))
		}

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
}
