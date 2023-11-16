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

	fmt.Println(string(output)) /////
	return nil
}

func main() {
	//file, err := os.Create("metrics.csv")
	//if err != nil {
	//	panic(err)
	//}
	//defer file.Close()
	//
	//writer := csv.NewWriter(file)
	//defer writer.Flush()

	command := "curl -sG 'http://127.0.0.1:9090/api/v1/query?' --data-urlencode 'query=(avg by(instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[2m])) * 100)' | jq -r '.data.result[1].value[1]'"
	thresholdHigh := 50.0 // > Half is idle => decrease frequency

	start := time.Now()
	for time.Since(start) < (5 * time.Minute) {
		cmd := exec.Command("bash", "-c", command)
		output, err := cmd.CombinedOutput()
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR :%+v", err))
		}

		resultString := strings.ReplaceAll(string(output), "\n", "")
		metricValue, err := strconv.ParseFloat(resultString, 64)
		if err != nil {
			fmt.Printf("Error converting to float: %v\n", err)
		}

		fmt.Println(metricValue) /////
		if metricValue > thresholdHigh {
			if err := setCPUFrequency(Low); err != nil {
				fmt.Println("Failed to set low CPU frequency:", err)
			}
		} else {
			if err := setCPUFrequency(High); err != nil {
				fmt.Println("Failed to set high CPU frequency:", err)
			}
		}

		//// Run the turbostat command
		//cmd = exec.Command("bash", "-c", "sudo turbostat --Summary --quiet --show Busy%,Avg_MHz,PkgTmp,PkgWatt --interval 1")
		//output, err = cmd.CombinedOutput()
		//if err != nil {
		//	fmt.Printf("Error running the turbostat command: %v\n", err)
		//}
		//fmt.Println(string(output)) /////
		//
		//// Parse and extract relevant metrics from the command output
		//lines := strings.Split(string(output), "\n")
		//// You may need to adjust the line index and parsing based on the actual output format
		//metricsLine := lines[2]
		//metrics := strings.Fields(metricsLine)
		//fmt.Printf(fmt.Sprintf("metrics collected=%v", metrics)) /////
		//
		//// Write metrics to the CSV file
		//err = writer.Write(append([]string{time.Now().Format("2006-01-02 15:04:05")}, metrics...))
		//if err != nil {
		//	fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
		//}
	}
}
