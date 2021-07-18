// MIT License
//
// Copyright (c) 2020 Dmitrii Ustiugov and EASE lab
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
	"io/ioutil"
	"os/exec"
	"path/filepath"

	log "github.com/sirupsen/logrus"

	. "github.com/ease-lab/vhive/examples/endpoint"
)

// Functions is an object for unmarshalled JSON with functions to deploy.
type Functions struct {
	Functions []functionType `json:"functions"`
}

type functionType struct {
	Name string `json:"name"`
	File string `json:"file"`

	// number of functions to deploy from the same file (with different names)
	Count int `json:"count"`

	Eventing bool `json:"eventing"`
	ApplyScript string `json:"applyScript"`
}

var (
	gatewayURL    = flag.String("gatewayURL", "192.168.1.240.sslip.io", "URL of the gateway")
	namespaceName = flag.String("namespace", "default", "name of namespace in which services exists")
)

func main() {
	funcPath := flag.String("funcPath", "./configs/knative_workloads", "Path to the folder with *.yml files")
	funcJSONFile := flag.String("jsonFile", "./examples/deployer/functions.json", "Path to the JSON file with functions to deploy")
	endpointsFile := flag.String("endpointsFile", "endpoints.json", "File with endpoints' metadata")
	deploymentConcurrency := flag.Int("conc", 5, "Number of functions to deploy concurrently (for serving)")

	flag.Parse()

	log.Debug("Function files are taken from ", *funcPath)

	funcSlice := getFuncSlice(*funcJSONFile)

	urls := deploy(*funcPath, funcSlice, *deploymentConcurrency)

	writeEndpoints(*endpointsFile, urls)

	log.Infoln("Deployment finished")
}

func getFuncSlice(file string) []functionType {
	log.Debug("Opening JSON file with functions: ", file)
	byteValue, err := ioutil.ReadFile(file)
	if err != nil {
		log.Fatal(err)
	}
	var functions Functions
	if err := json.Unmarshal(byteValue, &functions); err != nil {
		log.Fatal(err)
	}
	return functions.Functions
}

func deploy(funcPath string, funcSlice []functionType, deploymentConcurrency int) []string {
	var urls []string
	sem := make(chan bool, deploymentConcurrency) // limit the number of parallel deployments

	for _, fType := range funcSlice {
		for i := 0; i < fType.Count; i++ {

			sem <- true

			funcName := fmt.Sprintf("%s-%d", fType.Name, i)
			url := fmt.Sprintf("%s.%s.%s", funcName, *namespaceName, *gatewayURL)
			urls = append(urls, url)

			filePath := filepath.Join(funcPath, fType.File)

			go func(funcName, filePath string) {
				defer func() { <-sem }()

				deployFunction(funcName, filePath)
			}(funcName, filePath)
		}
	}

	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	return urls
}

func deployFunction(funcName, filePath string) {
	cmd := exec.Command(
		"kn",
		"service",
		"apply",
		funcName,
		"-f",
		filePath,
		"--concurrency-target",
		"1",
	)
	log.Info(cmd.String())
	stdoutStderr, err := cmd.CombinedOutput()
	if err != nil {
		log.Warnf("Failed to deploy function %s, %s: %v\n%s\n", funcName, filePath, err, stdoutStderr)
	}

	log.Info("Deployed function ", funcName)
}

func writeEndpoints(filePath string, urls []string) {
	var endpoints []Endpoint
	for _, url := range urls {
		endpoints = append(endpoints, Endpoint{
			Hostname: url,
			Eventing: false,
			Matchers: nil,
		})
	}
	data, err := json.MarshalIndent(endpoints, "", "\t")
	if err != nil {
		log.Fatalln("failed to marshal", err)
	}
	if err := ioutil.WriteFile(filePath, data, 0644); err != nil {
		log.Fatalln("failed to write", err)
	}
}
