package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

func setPowerProfileToNode(nodeNum, freq string) error {
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n    kubernetes.io/hostname: node-%s.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n powerProfiles:\n    - \"performance\"\nEOF", nodeNum)
	cmd := exec.Command("bash", "-c", command)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Println(string(output))

	// sharedProfile w freq
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: shared\n  namespace: intel-power\nspec:\n  name: \"shared\"\n  max: %s\n  min: %s\n  shared: true\n  governor: \"powersave\"\nEOF", freq, freq)
	cmd = exec.Command("bash", "-c", command)

	output, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	fmt.Println(string(output))

	// apply to node
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: shared-node-%s.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  name: \"shared-node-%s.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: node-%s.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n  powerProfile: \"shared\"\nEOF", nodeNum, nodeNum, nodeNum)
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

	frequencies := []string{"1200", "1400", "1600", "1800", "2000", "2200", "2400", "2600", "2800", "3000", "3200", "3400"}
	for i := 0; i < len(frequencies); i++ {
		err := setPowerProfileToNode("1", frequencies[i])
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR :%+v", err))
		}
		var totalTime time.Duration
		var totalFreq float64
		for sample := 0; sample < 10; sample++ {
			// apply to node
			url := ""
			startTime := time.Now()
			command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"Example text for AES\"", url)
			cmd := exec.Command("bash", "-c", command)
			freq, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERR :%+v", err))
			}
			totalTime += time.Since(startTime)

			freqValue, err := strconv.ParseFloat(string(freq), 64)
			if err != nil {
				fmt.Println("Error:", err)
				return
			}
			totalFreq += freqValue
		}
		averageLatency := totalTime / 10
		averageFreq := totalFreq / 10

		// Write metrics to the CSV file
		err = writer.Write(append([]string{strconv.FormatFloat(averageFreq, 'f', -1, 64)}, averageLatency.String()))
		if err != nil {
			fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
		}
		fmt.Println("done")
	}
}
