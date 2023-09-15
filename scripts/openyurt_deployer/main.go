// Author: Haoyuan Ma <flyinghorse0510@zju.edu.cn>
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/configs"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/node"
)

type NodesInfo struct {
	Master  string  `json:"master"`
	Workers Workers `json:"workers"`
}

type Workers struct {
	Cloud []string `json:"cloud"`
	Edge  []string `json:"edge"`
}

func initNodeList(nodesInfo NodesInfo) []node.Node {
	masterName := nodesInfo.Master
	cloudNames := nodesInfo.Workers.Cloud
	edgeNames := nodesInfo.Workers.Edge
	nodeList := []node.Node{}
	masterNode := node.Node{Name: masterName, Client: SetupSSHConn(masterName), NodeRole: "master", Configs: &node.NodeConfig{
		System:  configs.System,
		Kube:    configs.Kube,
		Knative: configs.Knative,
		Yurt:    configs.Yurt,
		Demo:	 configs.Demo}}
	nodeList = append(nodeList, masterNode)
	for _, name := range cloudNames {
		nodeList = append(nodeList, node.Node{Name: name, Client: SetupSSHConn(name), NodeRole: "cloud", Configs: &node.NodeConfig{
			System:  configs.System,
			Kube:    configs.Kube,
			Knative: configs.Knative,
			Yurt:    configs.Yurt,
			Demo: 	 configs.Demo}})
	}

	for _, name := range edgeNames {
		nodeList = append(nodeList, node.Node{Name: name, Client: SetupSSHConn(name), NodeRole: "edge", Configs: &node.NodeConfig{
			System:  configs.System,
			Kube:    configs.Kube,
			Knative: configs.Knative,
			Yurt:    configs.Yurt,
			Demo:    configs.Demo}})
	}
	return nodeList
}

func main() {
	if len(os.Args) != 2 {
		logs.PrintGeneralUsage()
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
	case "deploy":
		deployNodes(*deployerConf)
	case "clean":
		cleanNodes(*deployerConf)
	case "demo-e":
		demo(*deployerConf, false)
	case "demo-c":
		demo(*deployerConf, true)
	case "demo-clear":
		delDemo(*deployerConf)
	case "demo-print":
		printDemo(*deployerConf)
	default:
		logs.PrintGeneralUsage()
		os.Exit(-1)
	}

}

func cleanNodes(deployerConfFile string) {
	logs.SuccessPrintf("Start cleaning on nodes")
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

	nodeList := initNodeList(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]

	for _, node := range nodeList {
		node.SystemInit()
		logs.SuccessPrintf("Init system environment on node:%s success!\n", node.Name)
	}

	for _, worker := range workerNodes {
		// Deactivate node autonomous mode FOR WOKRER NODES
		logs.WaitPrintf("Deactivating the worker node %s autonomous mode", worker.Configs.System.NodeHostName)
		_, err = masterNode.ExecShellCmd("kubectl annotate node %s node.beta.openyurt.io/autonomy-", worker.Configs.System.NodeHostName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to deactivate the node autonomous mode!\n")

		worker.YurtWorkerClean()
	}

	_, err = masterNode.ExecShellCmd("helm uninstall yurt-app-manager -n kube-system")
	logs.CheckErrorWithTagAndMsg(err, "Failed to helm uninstall yurt app manager!\n")
	logs.SuccessPrintf("Helm uninstall!\n")

	_, err = masterNode.ExecShellCmd("helm uninstall openyurt -n kube-system")
	logs.CheckErrorWithTagAndMsg(err, "Failed to helm uninstall openyurt!\n")
	logs.SuccessPrintf("Helm uninstall!\n")

	for _, node := range nodeList {
		node.KubeClean()
	}

}

func deployNodes(deployerConfFile string) {
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

	nodeList := initNodeList(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]

	// init system, all nodes are the same
	for _, node := range nodeList {
		node.SystemInit()
		logs.SuccessPrintf("Init system environment on node:%s success!\n", node.Name)
	}

	// init kube cluster
	logs.InfoPrintf("Start to init kube cluster!\n")
	addr, port, token, hash := masterNode.KubeMasterInit()
	logs.SuccessPrintf("Master init success, join the cluster with following command:\n sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s\n",
		addr, port, token, hash)
	for _, worker := range workerNodes {
		worker.KubeWorkerJoin(addr, port, token, hash)
		logs.InfoPrintf("worker %s joined cluster!\n", worker.Name)
	}
	nodesName := masterNode.GetAllNodes()
	logs.InfoPrintf("All nodes within the cluster: [")
	for _, name := range nodesName {
		fmt.Printf(name)
	}
	fmt.Printf("]\n")

	// init yurt cluster
	logs.SuccessPrintf("Start to init yurt cluster!\n")
	masterNode.YurtMasterInit()
	for _, worker := range workerNodes {
		worker.YurtWorkerJoin(addr, port, token)
		logs.InfoPrintf("worker %s joined yurt cluster!\n", worker.Configs.System.NodeHostName)
	}
	logs.SuccessPrintf("All nodes joined yurt cluster, start to expand\n")
	for _, worker := range workerNodes {
		masterNode.YurtMasterExpand(&worker)
		logs.InfoPrintf("Master has expanded to worker:%s\n", worker.Configs.System.NodeHostName)
	}
	logs.SuccessPrintf("Master has expaned to all nodes!\n")

	// // init knative
	logs.SuccessPrintf("Start to init knative\n")
	masterNode.InstallKnativeServing()
	masterNode.InstallKnativeEventing()
	logs.SuccessPrintf("Knative has been installed!\n")

	// init demo environment
	masterNode.BuildDemo(workerNodes)

	logs.SuccessPrintf(">>>>>>>>>>>>>>>>OpenYurt Cluster Deployment Finished!<<<<<<<<<<<<<<<\n")


}

func demo(deployerConfFile string, isCloud bool){
	demoEnv := "Cloud"
	if !isCloud { demoEnv = "Edge" }	
	logs.SuccessPrintf(">>>>>>>>>>>>>>>>Entering openyurt demo for [%s Node Pool]<<<<<<<<<<<<<<<\n", demoEnv)
	deployerConfJSON, err := os.ReadFile(deployerConfFile)
	if err != nil {
		log.Fatalf("Failed to open configuration file :%s", err)
	}

	var nodesInfo NodesInfo
	err = json.Unmarshal(deployerConfJSON, &nodesInfo)
	if err != nil {
		log.Fatalf("Failed to unmarshal nodes JSON: %s", err)
	}

	nodeList := initNodeList(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]
	// run demo, should only be executed after deployment
	logs.SuccessPrintf("Start to init demo\n")
	masterNode.Demo(isCloud)
	logs.SuccessPrintf("Demo finished!\n")
	masterNode.PrintDemoInfo(workerNodes, isCloud)
}

func printDemo(deployerConfFile string){

	deployerConfJSON, err := os.ReadFile(deployerConfFile)
	if err != nil {
		log.Fatalf("Failed to open configuration file :%s", err)
	}
	var nodesInfo NodesInfo
	err = json.Unmarshal(deployerConfJSON, &nodesInfo)
	if err != nil {
		log.Fatalf("Failed to unmarshal nodes JSON: %s", err)
	}

	nodeList := initNodeList(nodesInfo)
	masterNode := nodeList[0]
	workerNodes := nodeList[1:]
	masterNode.GetNodeHostName()
	masterNode.PrintDemoInfo(workerNodes, true)
	masterNode.PrintDemoInfo(workerNodes, false)
}


func delDemo(deployerConfFile string) {

	logs.SuccessPrintf("Clean the demo files")

	deployerConfJSON, err := os.ReadFile(deployerConfFile)
	if err != nil {
		log.Fatalf("Failed to open configuration file :%s", err)
	}

	var nodesInfo NodesInfo
	err = json.Unmarshal(deployerConfJSON, &nodesInfo)
	if err != nil {
		log.Fatalf("Failed to unmarshal nodes JSON: %s", err)
	}

	nodeList := initNodeList(nodesInfo)
	masterNode := nodeList[0]
	masterNode.DeleteDemo(nodeList)
	logs.SuccessPrintf("Delete the demo success!\n")
}

