package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	High int = 1
	Low      = 2
)

func setCPUFrequency(frequency int) error {
	m := map[int]string{
		High: "performance",
		Low:  "shared",
	}
	fmt.Printf("applying %s profile...\n", m[frequency])
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: shared-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  name: \"shared-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n  powerProfile: \"%s\"\nEOF", m[frequency])
	cmd := exec.Command("bash", "-c", command)

	// Capture and check for any errors.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	fmt.Println(output)
	return nil
}

func main() {
	// Define your Prometheus query and threshold values
	command := "curl -sG 'http://127.0.0.1:9090/api/v1/query?' --data-urlencode 'query=(avg by(instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[2m])) * 100)' | jq -r '.data.result[1].value[1]'"
	thresholdHigh := 80.0 // Mostly idle => decrease frequency
	thresholdLow := 20.0  // Mostly CPU bound => increase frequency

	for {
		cmd := exec.Command("bash", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR :%+v", err))
		}

		resultString := strings.ReplaceAll(string(output), "\n", "")
		metricValue, err := strconv.ParseFloat(resultString, 64)
		if err != nil {
			fmt.Printf("Error converting to float: %v\n", err)
			return
		}

		fmt.Println(metricValue)
		if metricValue > thresholdHigh {
			if err := setCPUFrequency(Low); err != nil {
				fmt.Println("Failed to set low CPU frequency:", err)
			}
		} else if metricValue != 0 && metricValue < thresholdLow {
			if err := setCPUFrequency(High); err != nil {
				fmt.Println("Failed to set high CPU frequency:", err)
			}
		}
		time.Sleep(60 * time.Second) // Adjust the polling interval as needed
	}
}
