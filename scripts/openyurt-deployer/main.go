package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"

	"github.com/vhive-serverless/vHive/scripts/utils"

	log "github.com/sirupsen/logrus"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/configs"
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
	case "deploy":
		deployNodes(*deployerConf)
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
	case "deploy-yurt":
		deployOpenYurt(*deployerConf)
	case "help":
		utils.InfoPrintf("Usage: %s  <operation: deploy | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
	default:
		utils.InfoPrintf("Usage: %s  <operation: deploy | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
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

func parseNodeInfo(nodesInfo NodesInfo) []node.Node {
	masterName := nodesInfo.Master
	cloudNames := nodesInfo.Workers.Cloud
	edgeNames := nodesInfo.Workers.Edge
	nodeList := []node.Node{}

	// Load configs from configs/setup.json
	configs.System.LoadConfig()
	configs.Knative.LoadConfig()
	configs.Kube.LoadConfig()

	masterNode := node.Node{Name: masterName, Client: SetupSSHConn(masterName), NodeRole: "master", Configs: &node.NodeConfig{
		System:  configs.System,
		Kube:    configs.Kube,
		Knative: configs.Knative,
		Yurt:    configs.Yurt,
		Demo:    configs.Demo}}
	nodeList = append(nodeList, masterNode)
	for _, name := range cloudNames {
		nodeList = append(nodeList, node.Node{Name: name, Client: SetupSSHConn(name), NodeRole: "cloud", Configs: &node.NodeConfig{
			System:  configs.System,
			Kube:    configs.Kube,
			Knative: configs.Knative,
			Yurt:    configs.Yurt,
			Demo:    configs.Demo}})
	}

	for _, name := range edgeNames {
		nodeList = append(nodeList, node.Node{Name: name, Client: SetupSSHConn(name), NodeRole: "edge", Configs: &node.NodeConfig{
			System:  configs.System,
			Kube:    configs.Kube,
			Knative: configs.Knative,
			Yurt:    configs.Yurt,
			Demo:    configs.Demo}})
	}

	for _, node := range nodeList {
		node.ReadSystemInfo()
		utils.SuccessPrintf("Read system info on node:%s success!\n", node.Name)
	}

	return nodeList
}

func initializeNodes(nodesInfo NodesInfo) []node.Node {
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

	// init knative
	utils.SuccessPrintf("Start to init knative\n")
	masterNode.InstallKnativeServing()
	masterNode.InstallKnativeEventing()
	utils.SuccessPrintf("Knative has been installed!\n")

	// init demo environment
	masterNode.BuildDemo(workerNodes)
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
