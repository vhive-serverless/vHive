// MIT License
//
// Copyright (c) 2023 Georgiy Lebedev and vHive team
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

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/sfreiberg/simplessh"
	log "github.com/sirupsen/logrus"
	"os"
	"sync"
)

type RunnerConf struct {
	Type    string `json:"type"`
	Sandbox string `json:"sandbox,omitempty"`
	Num     uint   `json:"num,omitempty"`
	Restart bool   `json:"restart,omitempty"`
}

type DeployerConf struct {
	GhOrg        string                `json:"ghOrg"`
	GhPat        string                `json:"ghPat"`
	HostUsername string                `json:"hostUsername"`
	RunnerConfs  map[string]RunnerConf `json:"runners"`
}

func main() {
	var (
		deployerConf = flag.String("conf", "conf.json",
			`Configuration file with the following structure:
			{
			  "ghOrg": "<GitHub account>",
			  "ghPat": "<GitHub PAT>",
			  "hostUsername": "ubuntu",
			  "runners": {
				"pc91.cloudlab.umass.edu": {
				  "type": "cri",
				  "sandbox": "firecracker"
				},
				"pc101.cloudlab.umass.edu": {
				  "type": "cri",
				  "sandbox": "gvisor"
				},
				"pc72.cloudlab.umass.edu": {
				  "type": "integ",
				  "num": 2
				}
				"pc75.cloudlab.umass.edu": {
				  "type": "integ",
				  "num": 6,
      			  "restart": true
				}
			  }
			}
			`)
		logLvl = flag.String("loglvl", "info", "Debug level: 'info' or 'debug'")
	)
	flag.Parse()
	log.SetOutput(os.Stdout)
	switch *logLvl {
	case "info":
		log.SetLevel(log.InfoLevel)
	case "debug":
		log.SetLevel(log.DebugLevel)
		log.Debug("Debug mode is enabled")
	default:
		log.Fatalf("Invalid log level: '%s', expected 'info' or 'debug'", *logLvl)
	}
	deployRunners(*deployerConf)
}

func deployRunners(deployerConfFile string) {
	log.Debugf("Opening deployer configuration JSON: %s", deployerConfFile)
	deployerConfJSON, err := os.ReadFile(deployerConfFile)
	if err != nil {
		log.Fatalf("Failed to open configuration file: %s", err)
	}

	log.Debugf("Unmarshaling deployer configuration JSON: %s", deployerConfJSON)
	var deployerConf DeployerConf
	err = json.Unmarshal(deployerConfJSON, &deployerConf)
	if err != nil {
		log.Fatalf("Failed to unmarshal runners JSON: %s", err)
	}

	var wg sync.WaitGroup
	for host, runnerConf := range deployerConf.RunnerConfs {
		wg.Add(1)
		go deployRunner(host, runnerConf, &deployerConf, &wg)
	}
	wg.Wait()
}

func deployRunner(host string, runnerConf RunnerConf, deployerConf *DeployerConf, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Debugf("Connecting to %s@%s", deployerConf.HostUsername, host)
	client, err := simplessh.ConnectWithAgent(host, deployerConf.HostUsername)
	if err != nil {
		log.Fatalf("Failed to connect to %s@%s: %s", deployerConf.HostUsername, host, err)
	}

	log.Debugf("Cloning vHive repository on %s@%s", deployerConf.HostUsername, host)
	out, err := client.Exec(fmt.Sprintf("rm -rf ./vhive ./runner && git clone --depth=1 -b ci_fix https://github.com/%s/vhive", deployerConf.GhOrg))
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to clone vHive repository on %s@%s: %s", deployerConf.HostUsername, host, err)
	}

	var setupCmd string
	switch runnerConf.Type {
	case "cri":
		setupCmd = fmt.Sprintf("cd vhive && ./scripts/github_runner/setup_bare_metal_runner.sh %s %s %s", deployerConf.GhOrg,
			deployerConf.GhPat, runnerConf.Sandbox)
	case "integ":
		var restart string
		if runnerConf.Restart {
			restart = "restart"
		} else {
			restart = ""
		}
		if !runnerConf.Restart {
			log.Debugf("Setting up integration runners host on %s@%s", deployerConf.HostUsername, host)
			out, err = client.Exec("cd vhive && ./scripts/github_runner/setup_integ_runners_host.sh")
			log.Debug(string(out))
			if err != nil {
				log.Fatalf("Failed to setup integration runners host on %s@%s: %s", deployerConf.HostUsername, host, err)
			}
		}

		setupCmd = fmt.Sprintf("cd vhive && ./scripts/github_runner/setup_integ_runners.sh %d %s %s %s", runnerConf.Num,
			deployerConf.GhOrg, deployerConf.GhPat, restart)
	default:
		log.Fatalf("Invalid runner type: '%s', expected 'cri' or 'integ'", runnerConf.Type)
	}

	log.Debugf("Setting up runner on %s@%s", deployerConf.HostUsername, host)
	out, err = client.Exec(setupCmd)
	log.Debug(string(out))
	if err != nil {
		log.Fatalf("Failed to setup runner on %s@%s: %s", deployerConf.HostUsername, host, err)
	}

	err = client.Close()
	if err != nil {
		log.Fatalf("Failed to close connection to %s@%s: %s", deployerConf.HostUsername, host, err)
	}
}
