package power_manager

import (
	"encoding/csv"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	SpinningURL       = "spinning-go.default.192.168.1.240.sslip.io"
	SleepingURL       = "sleeping-go.default.192.168.1.240.sslip.io"
	AesURL            = "aes-python.default.192.168.1.240.sslip.io"
	AuthURL           = "auth-python.default.192.168.1.240.sslip.io"
	Node1Name = "node-1.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us"  // to be replaced by your node name 
	Node2Name = "node-2.kt-cluster.ntu-cloud-pg0.utah.cloudlab.us" // to be replaced by your node name 
	HighFrequencyPowerProfile = "performance" 
	LowFrequencyPowerProfile = "shared"
)

func SetPowerProfileToNode(powerprofileName string, nodeName string, freq int64) error {
	// powerConfig
	command := fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerConfig\nmetadata:\n  name: power-config\n  namespace: intel-power\nspec:\n powerNodeSelector:\n     kubernetes.io/os: linux\n powerProfiles:\n    - \"performance\"\nEOF")
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// performanceProfile w freq
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerProfile\nmetadata:\n  name: %s\n  namespace: intel-power\nspec:\n  name: \"%s\"\n  max: %d\n  min: %d\n  shared: true\n  governor: \"performance\"\nEOF", powerprofileName, powerprofileName, freq, freq)
	cmd = exec.Command("bash", "-c", command)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}

	// apply to node
	command = fmt.Sprintf("kubectl apply -f - <<EOF\napiVersion: \"power.intel.com/v1\"\nkind: PowerWorkload\nmetadata:\n  name: %s-%s-workload\n  namespace: intel-power\nspec:\n  name: \"%s-%s-workload\"\n  allCores: true\n  powerNodeSelector:\n    kubernetes.io/hostname: %s\n  powerProfile: \"%s\"\nEOF", powerprofileName, nodeName, powerprofileName, nodeName, nodeName, powerprofileName)
	cmd = exec.Command("bash", "-c", command)

	_, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	return nil
}

func Invoke(url string) (int64, int64, int64, error) {
	command := fmt.Sprintf("cd $HOME/vSwarm/tools/test-client && ./test-client --addr %s:80 --name \"allow\"", url)
	startInvoke := time.Now().UTC().UnixMilli()
	cmd := exec.Command("bash", "-c", command)
	_, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, 0, err
	}
	endInvoke := time.Now().UTC().UnixMilli()
	latency := endInvoke - startInvoke
	return startInvoke, endInvoke, latency , nil
}

func InvokeConcurrently(n int, url string, ch chan<- []string, ch_latency_spinning chan<- int64, ch_latency_sleeping chan<- int64, spinning bool) {
	for i := 0; i < n; i++ {
		go func() {
			startInvoke, endInvoke, latency, err := Invoke(url)
			if err != nil {
				fmt.Printf("Error invoking benchmark: %v\n", err)
			}
			if spinning {
				ch_latency_spinning <- latency
				ch <- []string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), strconv.FormatInt(latency, 10), "-"}
			} else {
				ch_latency_sleeping <- latency
				ch <- []string{strconv.FormatInt(startInvoke, 10), strconv.FormatInt(endInvoke, 10), "-", strconv.FormatInt(latency, 10)}
			}
		}()
	}
}

func WriteToCSV(writer *csv.Writer, ch <-chan []string, wg *sync.WaitGroup) {
	defer wg.Done()
	for record := range ch {
		if err := writer.Write(record); err != nil {
			fmt.Printf("Error writing to CSV file: %v\n", err)
		}
	}
}

func GetDataAtPercentile(data []int64, percentile float64) int64 {
	if len(data) == 0 {
		return 0
	}
	sort.Slice(data, func(i, j int) bool { return data[i] < data[j] })
	n := (percentile / 100) * float64(len(data)-1)
	index := int(n)

	if index < 0 {
		index = 0
	} else if index >= len(data) {
		index = len(data) - 1
	}
	return data[index]
}