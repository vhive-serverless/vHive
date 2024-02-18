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

func main() {
	file1, err := os.Create("metrics1.csv")
	if err != nil {
		panic(err)
	}
	defer file1.Close()
	writer1 := csv.NewWriter(file1)
	defer writer1.Flush()
	err = writer1.Write(append([]string{"startTime", "endTime", "sleepingLatency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}

	file2, err := os.Create("metrics2.csv")
	if err != nil {
		panic(err)
	}
	defer file2.Close()
	writer2 := csv.NewWriter(file2)
	defer writer2.Flush()
	err = writer2.Write(append([]string{"startTime", "endTime", "spinningLatency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}

	ch1 := make(chan [][]string)
	ch2 := make(chan [][]string)

	now := time.Now()
	for time.Since(now) < (time.Second * 10) {
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

	for records := range ch1 {
		for _, record := range records {
			if err := writer1.Write(record); err != nil {
				fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
			}
		}
	}

	for records := range ch2 {
		for _, record := range records {
			if err := writer2.Write(record); err != nil {
				fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
			}
		}
	}

	close(ch1)
	close(ch2)

	err = writer1.Write(append([]string{"-", "-", "-"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}
	err = writer2.Write(append([]string{"-", "-", "-"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}
	fmt.Println("done")
}
