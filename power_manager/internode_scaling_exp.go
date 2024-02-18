package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
	"sync"
)

var (
	SpinningURL = "spinning-go.default.192.168.1.240.sslip.io"
	SleepingURL = "sleeping-go.default.192.168.1.240.sslip.io"
	AesURL      = "aes-python.default.192.168.1.240.sslip.io"
	AuthURL     = "auth-python.default.192.168.1.240.sslip.io"
)

func setPowerProfileToNodes(freq1 int64, freq2 int64) error {
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n    kubernetes.io/os: linux\n powerProfiles:\n    - \"performance\"\nEOF")
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// performanceProfile w freq
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: performance-node1\n  namespace: intel-power\nspec:\n  name: \"performance-node1\"\n  max: %d\n  min: %d\n  shared: true\n  governor: \"performance\"\nEOF", freq1, freq1)
	cmd = exec.Command("bash", "-c", command)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: performance-node2\n  namespace: intel-power\nspec:\n  name: \"performance-node2\"\n  max: %d\n  min: %d\n  shared: true\n  governor: \"performance\"\nEOF", freq2, freq2)
	cmd = exec.Command("bash", "-c", command)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// apply to node
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: performance-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  name: \"performance-node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n  powerProfile: \"performance-node1\"\nEOF")
	cmd = exec.Command("bash", "-c", command)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: performance-node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\n  namespace: intel-power\nspec:\n  name: \"performance-node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us\n  powerProfile: \"performance-node2\"\nEOF")
	cmd = exec.Command("bash", "-c", command)
	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func invoke(n int, url string, ch chan [][]string) {
	var data [][]string
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
			record := []string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), strconv.FormatInt(latency, 10)}
			data = append(data, record)
		}()
	}
	ch <- data
}

func writeToFile(filename string, ch chan [][]string) {
	file, err := os.Create(filename)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	for records := range ch {
		for _, record := range records {
			if err := writer.Write(record); err != nil {
				fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
			}
		}
	}
}

func main() {
	ch1 := make(chan [][]string)
	ch2 := make(chan [][]string)

	go writeToFile("metrics1.csv", ch1)
	go writeToFile("metrics2.csv", ch2)

	now := time.Now()
	for time.Since(now) < (time.Minute * 2) {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			invoke(5, SleepingURL, ch1)
		}()
		go func() {
			defer wg.Done()
			invoke(5, SpinningURL, ch2)
		}()
		wg.Wait()
	}

	close(ch1)
	close(ch2)
	fmt.Println("done")
}
