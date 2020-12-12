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

// PerfProfileRequestsPerSeondBench profiles requests per second benchmark
func PerfProfileRequestsPerSeondBench() error {
	var (
		rpsInits   = getRPSInits()
		delayInits = getDelayInits()
		delayIncs  = getDelayIncs()
	)

	// get selected images and perf events from user
	funcName, _, err := setFunction()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", err)
		return err
	}
	events := []string{"instructions", "LLC-loads", "LLC-load-misses", "LLC-stores", "LLC-store-misses"}
	// events, err := setEvents()
	// if err != nil {
	// 	fmt.Printf("Prompt failed %v\n", err)
	// 	return err
	// }

	// Sweep the vm number from 4 to 100 with step size 4
	for vmNum := 100; vmNum <= 100; vmNum += 4 {
		// Get related RPS and delay according to vmNum and image
		rps := vmNum * rpsInits[funcName]
		delay := delayInits[funcName] + 0*delayIncs[funcName]

		// run bench rps and perf profile
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
		cmd := exec.Command("sudo", "/bin/sh", "-c", "cat perf_stat.log")
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
	images := getImages()
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
	// perf stat -D $DELAY -a -e instructions,LLC-loads,LLC-load-misses,LLC-stores,LLC-store-misses --output test.log sudo env "PATH=$PATH" go test -v -run TestBenchRequestPerSecond -args -vm $VM_NUM -requestPerSec $RPS -executionTime 1
	var eventsString, delimiter string
	for _, event := range events {
		eventsString += delimiter + event
		delimiter = ","
	}

	commandString := "perf stat -D " + fmt.Sprint(delay) + " -a -e " + eventsString + " --output perf_stat.log sudo env " + envPath + " go test -v -timeout 99999s -run TestBenchRequestPerSecond -args -executionTime 1 -vm " + fmt.Sprint(vmNum) + " -requestPerSec " + fmt.Sprint(rps) + " -functions " + funcName

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

func getImages() map[string]string {
	return map[string]string{
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
}

func getRPSInits() map[string]int {
	return map[string]int{
		// "helloworld":
		// "chameleon":
		// "pyaes":
		// "image_rotate":
		// "json_serdes":
		// "lr_serving":
		// "cnn_serving":
		// "rnn_serving":
		"lr_training": 1,
	}
}

func getDelayIncs() map[string]int {
	return map[string]int{
		// "helloworld":
		// "chameleon":
		// "pyaes":
		// "image_rotate":
		// "json_serdes":
		// "lr_serving":
		// "cnn_serving":
		// "rnn_serving":
		"lr_training": 50000,
	}
}

func getDelayInits() map[string]int {
	return map[string]int{
		// "helloworld":
		// "chameleon":
		// "pyaes":
		// "image_rotate":
		// "json_serdes":
		// "lr_serving":
		// "cnn_serving":
		// "rnn_serving":
		"lr_training": 70000,
	}
}

func getKeys(m map[string]string) []string {
	var keys []string
	for key := range m {
		keys = append(keys, key)
	}

	return keys
}
