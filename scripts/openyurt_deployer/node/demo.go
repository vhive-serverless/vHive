package node

import (
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/template"
)

func (masterNode *Node) BuildDemo(workerNodes []Node) {

	var err error
	// cloud.yaml
	fromTmplate := template.GetMasterNpTemplateConfig()
	cloudFile := "cloud.yaml"
	_, err = masterNode.ExecShellCmd("echo '%s' > %s", fromTmplate, cloudFile)
	logs.CheckErrorWithMsg(err, "Failed to cp cloud\n")

	// edge.yaml
	fromTmplate = template.GetEdgeNpTemplateConfig()
	edgeFile := "edge.yaml"
	_, err = masterNode.ExecShellCmd("echo '%s' > %s", fromTmplate, edgeFile)
	logs.CheckErrorWithMsg(err, "Failed to cp edge\n")

	//label master as cloud TODO not just master, but all cloud nodes
	_, err = masterNode.ExecShellCmd(`kubectl label node %s apps.openyurt.io/desired-nodepool=%s`, masterNode.Configs.System.NodeHostName, masterNode.NodeRole)
	logs.CheckErrorWithMsg(err, "Master Cloud label fail\n")

	//label edge
	for _, worker := range workerNodes {
		_, err = masterNode.ExecShellCmd("kubectl label node %s apps.openyurt.io/desired-nodepool=%s", worker.Configs.System.NodeHostName, worker.NodeRole)
		logs.CheckErrorWithMsg(err, "worker label fail\n")
	}
	logs.SuccessPrintf("Label success\n")

	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", cloudFile)
	logs.CheckErrorWithMsg(err, "Failed to apply cloud.yaml\n")

	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", edgeFile)
	logs.CheckErrorWithMsg(err, "Failed to apply edge.yaml\n")

	// run demo app on nodes - NOT WORKING
	// fromTmplate = template.GetYurtAppSetTemplate()
	// appFile := "yurtSet.yaml"
	// masterNode.ExecShellCmd("echo '%s' > %s", fromTmplate, appFile)
	// _, err = masterNode.ExecShellCmd("kubectl apply -f %s", appFile)
	// logs.CheckErrorWithTagAndMsg(err, "Failed to yurt set\n")
	// logs.SuccessPrintf("yurt set success\n")
}
