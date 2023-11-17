package node

import (
	"fmt"

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
	// yurtFile :=  utils.InfoPrintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.YurtAppSetYamlFile)

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
	utils.CheckErrorWithTagAndMsg(err, "Failed to apply edge.yaml\n")
}
