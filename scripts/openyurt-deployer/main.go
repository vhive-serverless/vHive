// MIT License
//
// Copyright (c) 2023 Jason Chua, Ruiqi Lai and vHive team
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
	"os"
	"strings"

	"github.com/vhive-serverless/vHive/scripts/utils"

	log "github.com/sirupsen/logrus"
)

type NodesInfo struct {
	Master  string  `json:"master"`
	Workers Workers `json:"workers"`
}

type Workers struct {
	Cloud []string `json:"cloud"`
	Edge  []string `json:"edge"`
}

func main() {
	if len(os.Args) != 2 {
		utils.InfoPrintf("Usage: %s  <operation: deploy | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
		os.Exit(-1)
	}

	var (
		deployerConf = flag.String("conf", "conf.json",
			`Configuration file with the following structure:
			{
				"master": "user@master",
				"workers": {
					"cloud": [
						"user@cloud-0"
					],
					"edge": [
						"user@edge-0"
					]
				}
			}
			`)
		logLvl = flag.String("loglvl", "debug", "Debug level: 'info' or 'debug'")
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
	operation := os.Args[1]
	switch operation {
	case "k8s":
		deployNodes(*deployerConf)
	case "knative":
		deployKnative(*deployerConf)
	// case "clean":
	// 	cleanNodes(*deployerConf)
	// case "demo-e":
	// 	demo(*deployerConf, false)
	// case "demo-c":
	// 	demo(*deployerConf, true)
	// case "demo-clear":
	// 	delDemo(*deployerConf)
	// case "demo-print":
	// 	printDemo(*deployerConf)
	case "openyurt":
		deployOpenYurt(*deployerConf)
	case "help":
		utils.InfoPrintf("Usage: %s  <operation: k8s | knative | openyurt | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
	default:
		utils.InfoPrintf("Usage: %s  <operation: k8s | knative | openyurt | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
		os.Exit(-1)
	}

}

func readAndUnMarshall(deployerConfFile string) (NodesInfo, error) {
	log.Debugf("Opening openyurt deployer configuration JSON: %s", deployerConfFile)
	deployerConfJSON, err := os.ReadFile(deployerConfFile)
	if err != nil {
		log.Fatalf("Failed to open configuration file :%s", err)
	}

	log.Debugf("Unmarshaling nodes JSON: %s", deployerConfJSON)
	var nodesInfo NodesInfo
	err = json.Unmarshal(deployerConfJSON, &nodesInfo)
	if err != nil {
		log.Fatalf("Failed to unmarshal nodes JSON: %s", err)
	}
	return nodesInfo, nil
}

func parseNodeInfo(nodesInfo NodesInfo) []Node {
	masterName := nodesInfo.Master
	cloudNames := nodesInfo.Workers.Cloud
	edgeNames := nodesInfo.Workers.Edge
	nodeList := []Node{}

	// Load configs from configs/setup.json
	System.LoadConfig()
	Knative.LoadConfig()
	Kube.LoadConfig()

	masterNode := Node{Name: masterName, Client: SetupSSHConn(masterName), NodeRole: "master", Configs: &NodeConfig{
		System:  System,
		Kube:    Kube,
		Knative: Knative,
		Yurt:    Yurt,
		Demo:    Demo}}
	nodeList = append(nodeList, masterNode)
	for _, name := range cloudNames {
		nodeList = append(nodeList, Node{Name: name, Client: SetupSSHConn(name), NodeRole: "cloud", Configs: &NodeConfig{
			System:  System,
			Kube:    Kube,
			Knative: Knative,
			Yurt:    Yurt,
			Demo:    Demo}})
	}

	for _, name := range edgeNames {
		nodeList = append(nodeList, Node{Name: name, Client: SetupSSHConn(name), NodeRole: "edge", Configs: &NodeConfig{
			System:  System,
			Kube:    Kube,
			Knative: Knative,
			Yurt:    Yurt,
			Demo:    Demo}})
	}

	for _, node := range nodeList {
		node.ReadSystemInfo()
		utils.SuccessPrintf("Read system info on node:%s success!\n", node.Name)
	}

	return nodeList
}

func initializeNodes(nodesInfo NodesInfo) []Node {
	nodeList := parseNodeInfo(nodesInfo)

	// init system, all nodes are the same
	for _, node := range nodeList {
		node.SystemInit()
		utils.SuccessPrintf("Init system environment on node: %s success!\n", node.Name)
	}
	return nodeList
}

func deployNodes(deployerConfFile string) {

	nodesInfo, err := readAndUnMarshall(deployerConfFile)
	utils.CheckErrorWithMsg(err, "Failed to read and unmarshal deployer configuration JSON")
	nodeList := initializeNodes(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]

	// init kube cluster
	utils.InfoPrintf("Start to init kube cluster!\n")
	addr, port, token, hash := masterNode.KubeMasterInit()
	utils.SuccessPrintf("Master init success, join the cluster with following command:\n sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s\n",
		addr, port, token, hash)
	for _, worker := range workerNodes {
		worker.KubeWorkerJoin(addr, port, token, hash)
		utils.InfoPrintf("worker %s joined cluster!\n", worker.Name)
	}
	nodesName := masterNode.GetAllNodes()
	utils.InfoPrintf("All nodes within the cluster: [")
	for _, name := range nodesName {
		utils.InfoPrintf(name)
	}
	utils.InfoPrintf("]\n")
}

func deployKnative(deployerConfFile string) {

	nodesInfo, err := readAndUnMarshall(deployerConfFile)
	utils.CheckErrorWithMsg(err, "Failed to read and unmarshal deployer configuration JSON")
	nodeList := parseNodeInfo(nodesInfo)
	masterNode := nodeList[0]

	// init knative
	utils.SuccessPrintf("Start to init knative\n")
	masterNode.InstallKnativeServing()
	masterNode.InstallKnativeEventing()
	utils.SuccessPrintf("Knative has been installed!\n")
}

func deployOpenYurt(deployerConfFile string) {

	nodesInfo, err := readAndUnMarshall(deployerConfFile)
	utils.CheckErrorWithMsg(err, "Failed to read and unmarshal deployer configuration JSON")
	nodeList := initializeNodes(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]

	// init yurt cluster
	utils.SuccessPrintf("Start to init yurt cluster!\n")
	masterNode.YurtMasterInit()

	utils.WaitPrintf("Extracting master node information from logs")
	output, err := masterNode.ExecShellCmd("sed -n '1p;2p;3p;4p' %s/masterNodeValues", masterNode.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n")

	// Process the content and assign it to variables
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		utils.ErrorPrintf("Invalid file format")
		return
	}

	addr := lines[0]
	port := lines[1]
	token := lines[2]

	for _, worker := range workerNodes {
		worker.YurtWorkerJoin(addr, port, token)
		utils.InfoPrintf("worker %s joined yurt cluster!\n", worker.Configs.System.NodeHostName)
	}
	utils.SuccessPrintf("All nodes joined yurt cluster, start to expand\n")
	for _, worker := range workerNodes {
		masterNode.YurtMasterExpand(&worker)
		utils.InfoPrintf("Master has expanded to worker:%s\n", worker.Configs.System.NodeHostName)
	}
	utils.SuccessPrintf("Master has expaned to all nodes!\n")

	for _, node := range nodeList {
		utils.InfoPrintf("node: %s\n", node.Name)
		node.CleanUpTmpDir()
	}
	utils.SuccessPrintf(">>>>>>>>>>>>>>>>OpenYurt Cluster Deployment Finished!<<<<<<<<<<<<<<<\n")
}
