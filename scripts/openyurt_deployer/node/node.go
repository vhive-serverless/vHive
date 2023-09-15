package node

import (
	"fmt"
	"strings"

	"github.com/sfreiberg/simplessh"
	"github.com/vhive-serverless/vHive/scripts/utils"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/configs"
)

type NodeConfig struct {
	System  configs.SystemEnvironmentStruct
	Kube    configs.KubeConfigStruct
	Knative configs.KnativeConfigStruct
	Yurt    configs.YurtEnvironment
	Demo    configs.DemoEnvironment
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
		utils.WarnPrintf("node: [%s] failed to exec: \n%s\nerror:%s\n", node.Name, shellCmd, out)
	}
	return strings.TrimSuffix(string(out), "\n"), err
}

func (node *Node) OnlyExecByMaster() {
	if node.NodeRole != "master" {
		utils.FatalPrintf("This function can only be executed by master node!\n")
	}
}

func (node *Node) OnlyExecByWorker() {
	if node.NodeRole == "master" {
		utils.FatalPrintf("This function can only be executed by worker node!\n")
	}
}

func (node *Node) SetMasterAsCloud(asCloud bool) {
	node.OnlyExecByMaster()
	node.Configs.Yurt.MasterAsCloud = asCloud
}
