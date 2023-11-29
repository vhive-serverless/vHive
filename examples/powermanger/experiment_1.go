package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func setPowerProfileToNode(freq int64) error {
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n    kubernetes.io/hostname: node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n powerProfiles:\n    - \"performance\"\nEOF")
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Println(string(output))

	// performanceProfile w freq
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: performance\n  namespace: intel-power\nspec:\n  name: \"performance\"\n  max: %d\n  min: %d\n  shared: true\n  governor: \"performance\"\nEOF", freq, freq)
	cmd = exec.Command("bash", "-c", command)

	output, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Println(string(output))

	// apply to node
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: performance-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  name: \"performance-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n  powerProfile: \"performance\"\nEOF")
	cmd = exec.Command("bash", "-c", command)

	output, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Println(string(output))
	return nil
}

func main() {
	file, err := os.Create("metrics.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	err = writer.Write(append([]string{"startTime", "endTime", "avgLatency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}
	frequencies := []int64{1200, 1400, 1600, 1800, 2000, 2200, 2400, 2600, 2800}
	for i := 0; i < len(frequencies); i++ {
		err := setPowerProfileToNode(frequencies[i])
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR :%+v", err))
		}

		var totalTime time.Duration
		i, startExp := float64(0), time.Now()
		formattedStartTime := startExp.UTC().Format("2006-01-02 15:04:05 MST")
		for time.Since(startExp) < 10*time.Second {
			url := "auth-python.default.192.168.1.240.sslip.io"
			command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)

			startInvoke := time.Now()
			cmd := exec.Command("bash", "-c", command)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERR: %+v", err))
				return
			}
			elapsedTime := time.Since(startInvoke)
			totalTime += elapsedTime
			i += 1
		}

		averageLatency := totalTime.Seconds() / i
		formattedEndTime := time.Now().UTC().Format("2006-01-02 15:04:05 MST")

		err = writer.Write(append([]string{formattedStartTime, formattedEndTime, strconv.FormatFloat(averageLatency, 'f', -1, 64)}))
		if err != nil {
			fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
		}

		fmt.Println("done")
	}
}
