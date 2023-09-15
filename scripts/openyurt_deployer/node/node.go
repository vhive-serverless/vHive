package node

import (
	"fmt"
	"strings"

	"github.com/sfreiberg/simplessh"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/configs"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
)

type NodeConfig struct {
	System  configs.SystemEnvironmentStruct
	Kube    configs.KubeConfigStruct
	Knative configs.KnativeConfigStruct
	Yurt    configs.YurtEnvironment
}

type Node struct {
	Name     string
	Client   *simplessh.Client
	NodeRole string
	Configs  *NodeConfig
}

func (node *Node) ExecShellCmd(cmd string, pars ...any) (string, error) {
	shellCmd := fmt.Sprintf(cmd, pars...)
	out, err := node.Client.Exec(shellCmd)
	if err != nil {
		logs.WarnPrintf("node: [%s] failed to exec: \n%s\n", node.Name, shellCmd)
	}
	return strings.TrimSuffix(string(out), "\n"), err
}

func (node *Node) OnlyExecByMaster() {
	if node.NodeRole != "master" {
		logs.FatalPrintf("This function can only be executed by master node!\n")
	}
}

func (node *Node) OnlyExecByWorker() {
	if node.NodeRole == "master" {
		logs.FatalPrintf("This function can only be executed by worker node!\n")
	}
}

func (node *Node) SetMasterAsCloud(asCloud bool) {
	node.OnlyExecByMaster()
	node.Configs.Yurt.MasterAsCloud = asCloud
}
