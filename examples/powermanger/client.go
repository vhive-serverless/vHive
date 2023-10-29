package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os/exec"
	"time"

	"github.com/prometheus/common/model"
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
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  # Replace <NODE_NAME> with the Node you intend this PowerWorkload to be associated with\n  name: shared-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  # Replace <NODE_NAME> with the Node you intend this PowerWorkload to be associated with\n  name: \"shared-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    # The label must be as below, as this workload will be specific to the Node\n    kubernetes.io/hostname: node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n powerProfile: \"%s\"\nEOF", m[frequency])
	cmd := exec.Command("bash", "-c", command)

	// Capture and check for any errors.
	output, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	fmt.Println(output)
	return nil
}

func fetchPrometheusMetric(prometheusURL, prometheusQuery string) (float64, error) {
	url := fmt.Sprintf("%s?query=%s", prometheusURL, prometheusQuery)
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("Prometheus request failed with status: %s", resp.Status)
	}

	// Read and parse the response body.
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	// Parse the Prometheus response.
	result := model.Vector{}
	if err := json.Unmarshal(body, &result); err != nil {
		return 0, err
	}

	// want the second result of the query.
	if len(result) > 1 {
		return float64(result[1].Value), nil
	}

	return 0, fmt.Errorf("No data found in Prometheus response")
}

func main() {
	// Define your Prometheus query and threshold values
	prometheusURL := "http://127.0.0.1:9090/api/v1/query"
	prometheusQuery := "(avg by(instance) (rate(node_cpu_seconds_total{mode=\"idle\"}[2m])) * 100)"
	thresholdHigh := 80.0 // Mostly idle => decrease frequency
	thresholdLow := 20.0  // Mostly CPU bound => increase frequency

	for {
		metricValue, err := fetchPrometheusMetric(prometheusURL, prometheusQuery)
		if err != nil {
			fmt.Printf(fmt.Sprintf("ERR :%+v", err))
		}

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
