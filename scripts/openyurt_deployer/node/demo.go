package node

import (
	"fmt"
	"strings"

	"github.com/vhive-serverless/vHive/scripts/utils"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/template"
)

// Builds cloud and edge nodepools
func (masterNode *Node) BuildDemo(workerNodes []Node) {

	masterNode.GetUserHomeDir()
	masterNode.GetNodeHostName()

	var err error
	// cloud.yaml
	cloudPoolName := masterNode.Configs.Demo.CloudPoolName
	edgePoolName := masterNode.Configs.Demo.EdgePoolName

	cloudFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.CloudYamlFile)
	edgeFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.EdgeYamlFile)
	// yurtFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.YurtAppSetYamlFile)

	createCloudNpTemplate := template.CreateCloudNpTemplate()
	utils.WaitPrintf("Creating yaml files for cloud nodepool")
	_, err = masterNode.ExecShellCmd(createCloudNpTemplate, cloudPoolName, cloudFile)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create yaml for cloud\n")

	// edge.yaml
	createEdgeNpTemplate := template.CreateEdgeNpTemplate()
	utils.WaitPrintf("Creating yaml files for edge nodepool")
	_, err = masterNode.ExecShellCmd(createEdgeNpTemplate, edgePoolName, edgeFile)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create yaml for edge\n")

	//label master as cloud TODO not just master, but all cloud nodes
	utils.WaitPrintf("Labeling master")
	_, err = masterNode.ExecShellCmd(`kubectl label node %s apps.openyurt.io/desired-nodepool=%s`, masterNode.Configs.System.NodeHostName, cloudPoolName)
	utils.CheckErrorWithTagAndMsg(err, "Master Cloud label fail\n")

	//label edge
	utils.WaitPrintf("Labeling workers")
	for _, worker := range workerNodes {
		worker.GetNodeHostName()
		var desiredNpName string
		if worker.NodeRole == "cloud" {
			desiredNpName = cloudPoolName
		} else {
			desiredNpName = edgePoolName
		}
		_, err = masterNode.ExecShellCmd("kubectl label node %s apps.openyurt.io/desired-nodepool=%s", worker.Configs.System.NodeHostName, desiredNpName)
		utils.CheckErrorWithTagAndMsg(err, "worker label fail\n")
	}
	utils.SuccessPrintf("Label success\n")

	utils.WaitPrintf("Apply cloud.yaml")
	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", cloudFile)
	utils.CheckErrorWithTagAndMsg(err, "Failed to apply cloud.yaml\n")

	utils.WaitPrintf("Apply edge.yaml")
	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", edgeFile)
	logs.CheckErrorWithTagAndMsg(err, "Failed to apply edge.yaml\n")
}

func (masterNode *Node) Demo(isCloud bool) {

	masterNode.GetUserHomeDir()
	masterNode.GetNodeHostName()

	var err error
	cloudPoolName := masterNode.Configs.Demo.CloudPoolName
	edgePoolName := masterNode.Configs.Demo.EdgePoolName

	utils.WaitPrintf("Creating benchmark's yaml file and apply it")
	benchmarkTemplate := template.GetBenchmarkTemplate()
	if isCloud {
		_, err = masterNode.ExecShellCmd(benchmarkTemplate, "cloud", cloudPoolName, "cloud", masterNode.Configs.Demo.CloudBenchYamlFile)
		_, err = masterNode.ExecShellCmd("kubectl apply -f %s", masterNode.Configs.Demo.CloudBenchYamlFile)
	} else {
		_, err = masterNode.ExecShellCmd(benchmarkTemplate, "edge", edgePoolName, "edge", masterNode.Configs.Demo.EdgeBenchYamlFile)
		_, err = masterNode.ExecShellCmd("kubectl apply -f %s", masterNode.Configs.Demo.EdgeBenchYamlFile)
	}
	utils.CheckErrorWithTagAndMsg(err, "Failed to create benchmark's yaml file and apply it")

}

func (masterNode *Node) PrintDemoInfo(workerNodes []Node, isCloud bool) {
	utils.InfoPrintf("NodePool Information:\n")
	utils.InfoPrintf("+--------------------------------------------------------------------+\n")
	npType := "cloud"
	if !isCloud {
		npType = "edge"
	}

	poolName := masterNode.Configs.Demo.CloudPoolName
	if !isCloud {
		poolName = masterNode.Configs.Demo.EdgePoolName
	}

	utils.InfoPrintf("+%s Nodepool %s:\n", npType, poolName)
	utils.InfoPrintf("+Nodes:\n")
	if isCloud {
		utils.InfoPrintf("+\tnode: %s <- Master\n", masterNode.Configs.System.NodeHostName)
	}
	for _, worker := range workerNodes {
		worker.GetNodeHostName()
		if worker.NodeRole == npType {
			utils.InfoPrintf("+\tnode: %s\n", worker.Configs.System.NodeHostName)
		}
	}

	shellOut, _ := masterNode.ExecShellCmd("kubectl get ksvc | grep '\\-%s' | awk '{print $1, substr($2, 8)}'", npType)
	var serviceName string
	var serviceURL string
	splittedOut := strings.Split(shellOut, " ")
	if len(splittedOut) != 2 {
		serviceName = "Null"
		serviceURL = "Null"
	} else {
		serviceName = splittedOut[0]
		serviceURL = splittedOut[1]
	}
	utils.SuccessPrintf("+Service: Name: [%s] with URL [%s]\n", serviceName, serviceURL)
	utils.InfoPrintf("+--------------------------------------------------------------------+\n")

}

func (masterNode *Node) DeleteDemo(nodeList []Node) {

	masterNode.GetUserHomeDir()
	masterNode.GetNodeHostName()

	utils.WaitPrintf("Clear all services")
	masterNode.ExecShellCmd("kubectl delete ksvc --all")

}
