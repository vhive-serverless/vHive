package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"

	powermanager "github.com/vhive-serverless/vhive/power_manager"
)

func main() {
	file, err := os.Create("metrics.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write(append([]string{"startTime", "endTime", "latency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}

	frequencies := []int64{1200, 1400, 1600, 1800, 2000, 2200, 2400, 2600}
	for i := 0; i < len(frequencies); i++ {
		err := powermanager.SetPowerProfileToNode(powermanager.Node1Name, powermanager.HighFrequencyPowerProfile, frequencies[i])
		if err != nil {
			fmt.Printf(fmt.Sprintf("Error setting up power profile: %+v", err))
		}

		for j := 0; j < 1000; j++ {
			startInvoke, endInvoke, latency, err := powermanager.Invoke(powermanager.AuthURL)
			if err != nil {
				fmt.Printf("Error invoking benchmark: %v\n", err)
			}
			err = writer.Write(append([]string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), strconv.FormatInt(latency, 10)}))
			if err != nil {
				fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
			}
		}

		err = writer.Write(append([]string{"-", "-", "-"}))
		if err != nil {
			fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
		}
		fmt.Println("done")
	}
}