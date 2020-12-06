// MIT License
//
// Copyright (c) 2020 Yuchen Niu and EASE lab
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package profile

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/manifoldco/promptui"
)

var (
	envPath = "PATH=" + os.Getenv("PATH")
)

// Create a perf instance type here and write some interfaces/methods
type PerfInstance struct {
	isAllVMsBooted int32
}

func NewPerfInstance() *PerfInstance {
	return nil
}

func (*PerfInstance) SetAllVMsBooted(vmsBooted int32) {

}

// PerfProfileRequestsPerSeondBench profiles requests per second benchmark
func PerfProfileRequestsPerSeondBench() error {
	// get selected images and perf events from user
	funcName, _, err := setFunction()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return err
	}
	events, err := setEvents()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return err
	}

	// Sweep the vm number from 4 to 100 with step size 4
	for _, vmNum := range [2]int{1, 10} {
		// Get related RPS and delay according to vmNum and image
		delay, rps := getArguments(vmNum, funcName)

		// run bench rps and perf profile
		//   wait for VMs boot
		//   perf stat
		if err = preCommands(); err != nil {
			fmt.Printf("Failed to run preCommands: %v\n", err)
		}
		if err = executePerf(delay, vmNum, rps, funcName, events); err != nil {
			fmt.Printf("Failed to run perf: %v\n", err)
		}
		if err = postCommands(); err != nil {
			fmt.Printf("Failed to run postCommands: %v\n", err)
		}

		// save result to a file (csv)
		// cat output file for now
		cmd := exec.Command("sudo", "/bin/sh", "-c", "cat test.log")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stdout

		if err = cmd.Run(); err != nil {
			fmt.Printf("Cannot cat log file: %v\n", err)
			return err
		}
	}

	// plot results (plot/plotter?)

	return nil
}

func setFunction() (string, string, error) {
	images := map[string]string{
		// "helloworld":   "ustiugov/helloworld:var_workload",
		// "chameleon":    "ustiugov/chameleon:var_workload",
		// "pyaes":        "ustiugov/pyaes:var_workload",
		// "image_rotate": "ustiugov/image_rotate:var_workload",
		// "json_serdes":  "ustiugov/json_serdes:var_workload",
		// "lr_serving":   "ustiugov/lr_serving:var_workload",
		// "cnn_serving":  "ustiugov/cnn_serving:var_workload",
		// "rnn_serving":  "ustiugov/rnn_serving:var_workload",
		"lr_training": "ustiugov/lr_training:var_workload",
	}

	funcs := getKeys(images)
	prompt := promptui.Select{
		Label: "Select a function",
		Items: funcs,
	}

	_, result, err := prompt.Run()

	if err != nil {
		return "", "", err
	}

	return result, images[result], nil
}

func setEvents() ([]string, error) {
	var events = []string{"instructions", "LLC-loads", "LLC-load-misses", "LLC-stores", "LLC-store-misses", "DONE"}

	prompt := promptui.Select{
		Label: "Select Perf Events (select DONE to quit)",
		Items: events,
	}

	var choice string
	var selectedEvents = []string{}
	for choice != "DONE" {
		_, result, err := prompt.Run()

		if err != nil {
			return nil, err
		}

		if result != "DONE" {
			selectedEvents = append(selectedEvents, result)
		}

		choice = result
	}

	return selectedEvents, nil
}

func preCommands() error {
	// sudo mkdir -m777 -p /tmp/ctrd-logs && sudo env "PATH=$PATH" /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &
	var CTRDLOGDIR = "/tmp/ctrd-logs"

	if err := os.MkdirAll(CTRDLOGDIR, 0777); err != nil {
		return err
	}

	commandString := "sudo env " + envPath + " /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.out 2>/tmp/ctrd-logs/fccd_orch_noupf_log_bench.err &"
	cmd := exec.Command("sudo", "/bin/sh", "-c", commandString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func executePerf(delay, vmNum, rps int, funcName string, events []string) error {
	// perf stat -D $DELAY -a -e instructions,LLC-loads,LLC-load-misses,LLC-stores,LLC-store-misses --output test.log sudo env "PATH=$PATH" go test -v -run TestBenchRequestPerSecond -args -vm $VM_NUM -requestPerSec $RPS -executionTime 2
	var eventsString, delimiter string
	for _, event := range events {
		eventsString += delimiter + event
		delimiter = ","
	}

	commandString := "perf stat -D " + fmt.Sprint(delay) + " -a -e " + eventsString + " --output test.log sudo env " + envPath + " go test -v -run TestBenchRequestPerSecond -args -vm " + fmt.Sprint(vmNum) + " -requestPerSec " + fmt.Sprint(rps) + " -functions " + funcName + " -executionTime 2"

	cmd := exec.Command("sudo", "/bin/sh", "-c", commandString)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func postCommands() error {
	// ./scripts/clean_fcctr.sh
	cmd := exec.Command("sudo", "/bin/sh", "-c", "./scripts/clean_fcctr.sh")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stdout

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func getArguments(vmNum int, funcName string) (int, int) {
	switch vmNum {
	case 1:
		return 35000, 5
	case 10:
		return 140900, 50
	}

	return 0, 0
}

func getKeys(m map[string]string) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}

	return keys
}
