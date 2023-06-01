package cluster

import (
	"fmt"
	"os"
	"strings"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func CreateMultinodeCluster(stockContainerd string) error {
	// Original Bash Scripts: scripts/cluster/create_multinode_cluster.sh

	err := CreateMasterKubeletService()
	if err != nil {
		return err
	}

	err = DeployKubernetes()
	if err != nil {
		return err
	}

	err = KubectlForNonRoot()
	if err != nil {
		return err
	}

	err = ExtractMasterNodeInfo()
	if err != nil {
		return err
	}

	err = WaitForWorkerNodes()
	if err != nil {
		return err
	}

	// Set up Master Node
	utils.WaitPrintf("Setting up master node")
	err = SetupMasterNode(stockContainerd)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to set up Master Node!\n") {
		return err
	}

	return nil
}

// Create kubelet service on master node
func CreateMasterKubeletService() error {
	utils.WaitPrintf("Creating kubelet service")
	bashCmd := `sudo sh -c 'cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF'`
	_, err := utils.ExecShellCmd(bashCmd)
	if !utils.CheckErrorWithMsg(err, "Failed to create kubelet service!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("sudo systemctl daemon-reload")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create kubelet service!\n") {
		return err
	}

	return nil
}

// Deploy Kubernetes
func DeployKubernetes() error {

	utils.WaitPrintf("Deploying Kubernetes(version %s)", configs.Kube.K8sVersion)
	shellCmd := fmt.Sprintf("sudo kubeadm init --kubernetes-version %s --pod-network-cidr=\"%s\" ", configs.Kube.K8sVersion, configs.Kube.PodNetworkCidr)
	if len(configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", configs.Kube.AlternativeImageRepo)
	}
	if len(configs.Kube.ApiserverAdvertiseAddress) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--apiserver-advertise-address=%s ", configs.Kube.ApiserverAdvertiseAddress)
	}
	shellCmd = fmt.Sprintf(shellCmd+"| tee %s/masterNodeInfo", configs.System.TmpDir)
	_, err := utils.ExecShellCmd(shellCmd)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy Kubernetes(version %s)!\n", configs.Kube.K8sVersion) {
		return err
	}

	return nil
}

// Make kubectl work for non-root user
func KubectlForNonRoot() error {
	utils.WaitPrintf("Making kubectl work for non-root user")
	_, err := utils.ExecShellCmd("mkdir -p %s/.kube && sudo cp -i /etc/kubernetes/admin.conf %s/.kube/config && sudo chown $(id -u):$(id -g) %s/.kube/config",
		configs.System.UserHomeDir,
		configs.System.UserHomeDir,
		configs.System.UserHomeDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to make kubectl work for non-root user!\n") {
		return err
	}

	return nil
}

// Extract master node information from logs && Create masterKey.yaml with master node information
func ExtractMasterNodeInfo() error {
	// Extract master node information from logs
	utils.WaitPrintf("Extracting master node information from logs")
	shellOut, err := utils.ExecShellCmd("sed -n '/.*kubeadm join.*/p' < %s/masterNodeInfo | sed -n 's/.*join \\(.*\\):\\(\\S*\\) --token \\(\\S*\\).*/\\1 \\2 \\3/p'", configs.System.TmpDir)
	if !utils.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n") {
		return err
	}
	splittedOut := strings.Split(shellOut, " ")
	configs.Kube.ApiserverAdvertiseAddress = splittedOut[0]
	configs.Kube.ApiserverPort = splittedOut[1]
	configs.Kube.ApiserverToken = splittedOut[2]
	shellOut, err = utils.ExecShellCmd("sed -n '/.*sha256:.*/p' < %s/masterNodeInfo | sed -n 's/.*\\(sha256:\\S*\\).*/\\1/p'", configs.System.TmpDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract master node information from logs!\n") {
		return err
	}
	configs.Kube.ApiserverTokenHash = shellOut
	masterKeyYamlTemplate := `ApiserverAdvertiseAddress: %s
ApiserverPort: %s
ApiserverToken: %s
ApiserverTokenHash: %s`

	// Create masterKey.yaml with master node information
	utils.WaitPrintf("Creating masterKey.yaml with master node information")
	masterKeyYamlFile, err := os.OpenFile(configs.System.CurrentDir+"/masterKey.yaml", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	if !utils.CheckErrorWithMsg(err, "Failed to create masterKey.yaml with master node information!\n") {
		return err
	}
	defer masterKeyYamlFile.Close()
	masterKeyYaml := fmt.Sprintf(
		masterKeyYamlTemplate,
		configs.Kube.ApiserverAdvertiseAddress,
		configs.Kube.ApiserverPort,
		configs.Kube.ApiserverToken,
		configs.Kube.ApiserverTokenHash)
	_, err = masterKeyYamlFile.WriteString(masterKeyYaml)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create masterKey.yaml with master node information!\n") {
		return err
	}

	utils.SuccessPrintf("Master node key information has been written to %s/masterKey.yaml! Check for details.\n", configs.System.CurrentDir)

	return nil
}

// Wait until all workers are connected
func WaitForWorkerNodes() error {
	for {
		utils.WarnPrintf("All nodes need to be joined in the cluster. Have you joined all nodes? (y/n): ")
		var userInput string
		var allNodesJoined = false
		_, err := fmt.Scanln(&userInput)
		if err != nil {
			utils.FatalPrintf("Unexpected Error!\n")
			return err
		}

		switch userInput {
		case "N":
		case "n":
		case "Y":
			allNodesJoined = true
		case "y":
			allNodesJoined = true
		default:
			utils.WarnPrintf("Please answer yes or no (y/n):")
		}

		if allNodesJoined {
			break
		}
	}
	utils.SuccessPrintf("All nodes successfully joined!(user confirmed)\n")
	return nil
}
