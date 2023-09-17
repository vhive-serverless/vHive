package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

)

func setPowerProfileToNodes(freq1 int64, freq2 int64) error {
	fmt.Printf("node1:%d, node2:%d\n", freq1, freq2)
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n    kubernetes.io/os: linux\n powerProfiles:\n    - \"performance\"\nEOF")
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	//fmt.Println(string(output))

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
	//fmt.Println(string(output))

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
	//fmt.Println(string(output))
	return nil
}

func spinning(wg *sync.WaitGroup, resultChan chan int64) {
	defer wg.Done()

	url := "aes-python.default.192.168.1.240.sslip.io"
	command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)

	startInvoke := time.Now().UTC().UnixMilli()
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERR2: %+v", err))
		return
	}
	endInvoke := time.Now().UTC().UnixMilli()
	latency := endInvoke-startInvoke
	resultChan <- latency
	fmt.Println(latency)
	return
}

func sleeping(wg *sync.WaitGroup, resultChan chan int64) {
	defer wg.Done()

	url := "auth-python.default.192.168.1.240.sslip.io"
	command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)
	
	startInvoke := time.Now().UTC().UnixMilli()
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf(fmt.Sprintf("ERR2: %+v", err))
		return
	}
	endInvoke := time.Now().UTC().UnixMilli()
	latency := endInvoke-startInvoke
	resultChan <- latency
	fmt.Println(latency)
	return
}

func main() {
	file, err := os.Create("metrics.csv")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	writer1 := csv.NewWriter(file)
	defer writer1.Flush()

	err = writer1.Write(append([]string{"startTime", "endTime", "spinningLatency", "sleepingLatency"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}

	// frequencies := [][]int64{{1200,1200},{1200,2400},{2400,2400}}
	// for i := 0; i < len(frequencies); i++ {
	// 	err := setPowerProfileToNodes(frequencies[i][0], frequencies[i][1])
	// 	if err != nil {
	// 		fmt.Printf(fmt.Sprintf("ERR1 :%+v", err))
	// 	}

	now := time.Now()
	for time.Since(now) < (time.Minute*2) {
		// Create channels to receive results from goroutines
		spinningResultChan := make(chan int64)
		sleepingResultChan := make(chan int64)

		startInvoke := time.Now().UTC().UnixMilli()
		go func() {
			url := "auth-python.default.192.168.1.240.sslip.io"
			command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)
			
			startInvoke := time.Now().UTC().UnixMilli()
			cmd := exec.Command("bash", "-c", command)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERR2: %+v", err))
				return
			}
			endInvoke := time.Now().UTC().UnixMilli()
			latency := endInvoke-startInvoke
			sleepingResultChan <- latency
		}()

		go func() {
			url := "aes-python.default.192.168.1.240.sslip.io"
			command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)
			
			startInvoke := time.Now().UTC().UnixMilli()
			cmd := exec.Command("bash", "-c", command)
			_, err := cmd.CombinedOutput()
			if err != nil {
				fmt.Printf(fmt.Sprintf("ERR2: %+v", err))
				return
			}
			endInvoke := time.Now().UTC().UnixMilli()
			latency := endInvoke-startInvoke
			spinningResultChan <- latency
		}()

		endInvoke := time.Now().UTC().UnixMilli()
		spinningLatency := <-spinningResultChan
		sleepingLatency := <-sleepingResultChan
		err = writer1.Write(append([]string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), strconv.FormatInt(spinningLatency, 10), strconv.FormatInt(sleepingLatency, 10)}))
		if err != nil {
			fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
		}
	}

	err = writer1.Write(append([]string{"-","-","-","-"}))
	if err != nil {
		fmt.Printf("Error writing metrics to the CSV file: %v\n", err)
	}
	fmt.Println("done")
	// }
}
