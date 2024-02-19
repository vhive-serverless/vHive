// MIT License
//
// Copyright (c) 2023 Haoyuan Ma and vHive team
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

package cluster

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func CreateMultinodeCluster(stockContainerd string, rawHaReplicaCount string) error {
	// Original Bash Scripts: scripts/cluster/create_multinode_cluster.sh

	haReplicaCount, err := strconv.Atoi(rawHaReplicaCount)
	if err != nil {
		return err
	}

	if err := CreateMasterKubeletService(); err != nil {
		return err
	}

	if err := DeployKubernetes(haReplicaCount); err != nil {
		return err
	}

	if err := KubectlForNonRoot(); err != nil {
		return err
	}

	if err := ExtractMasterNodeInfo(); err != nil {
		return err
	}

	if err := WaitForWorkerNodes(); err != nil {
		return err
	}

	if configs.System.LogVerbosity != 0 {
		if err := IncreaseLogSizePerContainer(); err != nil {
			return err
		}
	}

	// Set up master node
	utils.InfoPrintf("Set up master node\n")
	if err := SetupMasterNode(stockContainerd); err != nil {
		utils.FatalPrintf("Failed to set up master node!\n")
		return err
	}

	return nil
}

// Create kubelet service on master node
func CreateMasterKubeletService() error {
	utils.WaitPrintf("Creating kubelet service")
	// Create service directory if not exist
	_, err := utils.ExecShellCmd("sudo mkdir -p /etc/default")
	if !utils.CheckErrorWithMsg(err, "Failed to create kubelet service!\n") {
		return err
	}
	bashCmd := `sudo sh -c 'cat <<EOF > /etc/default/kubelet
KUBELET_EXTRA_ARGS="--v=%d --runtime-request-timeout=15m --container-runtime-endpoint=unix:///run/containerd/containerd.sock"
EOF'`
	_, err = utils.ExecShellCmd(bashCmd, configs.System.LogVerbosity)
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
func DeployKubernetes(haReplicaCount int) error {
	utils.WaitPrintf("Deploying Kubernetes(version %s)", configs.Kube.K8sVersion)
	masterNodeIp, iperr := utils.ExecShellCmd(`ip route | awk '{print $(NF)}' | awk '/^10\..*/'`)
	if iperr != nil {
		return iperr
	}

	command := `sudo kubeadm init --v=%d \
--apiserver-advertise-address=%s \
--cri-socket unix:///run/containerd/containerd.sock \
--kubernetes-version %s \
--pod-network-cidr="%s" `
	args := []any{configs.System.LogVerbosity, masterNodeIp, configs.Kube.K8sVersion, configs.Kube.PodNetworkCidr}

	if haReplicaCount > 0 {
		command += ` \
--control-plane-endpoint "%s:%s" \
--upload-certs`
		args = append(args, configs.Kube.CPHAEndpoint, configs.Kube.CPHAPort)
	}

	shellCmd := fmt.Sprintf(command, args)
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

	// API Server address, port, token
	shellOut, err := utils.ExecShellCmd("sed -n '/.*kubeadm join.*/p' < %s/masterNodeInfo | sed -n 's/.*join \\(.*\\):\\(\\S*\\) --token \\(\\S*\\).*/\\1 \\2 \\3/p'", configs.System.TmpDir)
	if !utils.CheckErrorWithMsg(err, "Failed to extract API Server address, port, and token from logs!\n") {
		return err
	}
	splittedOut := strings.Split(shellOut, " ")
	configs.Kube.ApiserverAdvertiseAddress = splittedOut[0]
	configs.Kube.ApiserverPort = splittedOut[1]
	configs.Kube.ApiserverToken = splittedOut[2]

	// API Server discovery token
	shellOut, err = utils.ExecShellCmd("sed -n '/.*sha256:.*/p' < %s/masterNodeInfo | sed -n 's/.*\\(sha256:\\S*\\).*/\\1/p'", configs.System.TmpDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract API Server discovery token from logs!\n") {
		return err
	}
	configs.Kube.ApiserverDiscoveryToken = shellOut

	// API Server certificate key
	shellOut, err = utils.ExecShellCmd("sed -n 's/^.*--certificate-key //p' < %s/masterNodeInfo", configs.System.TmpDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract API Server certificate key from logs!\n") {
		return err
	}
	configs.Kube.ApiserverCertificateKey = shellOut

	masterKeyYamlTemplate :=
		"ApiserverAdvertiseAddress: %s\n" +
			"ApiserverPort: %s\n" +
			"ApiserverToken: %s\n" +
			"ApiserverDiscoveryToken: %s\n" +
			"ApiserverCertificateKey: %s"

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
		configs.Kube.ApiserverDiscoveryToken)
	_, err = masterKeyYamlFile.WriteString(masterKeyYaml)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create masterKey.yaml with master node information!\n") {
		return err
	}

	utils.SuccessPrintf("Join cluster from worker nodes as a new control plane node with command: sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s --control-plane --certificate-key %s\n",
		configs.Kube.ApiserverAdvertiseAddress, configs.Kube.ApiserverPort, configs.Kube.ApiserverToken, configs.Kube.ApiserverDiscoveryToken, configs.Kube.ApiserverCertificateKey)

	utils.SuccessPrintf("Join cluster from worker nodes with command: sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s\n",
		configs.Kube.ApiserverAdvertiseAddress, configs.Kube.ApiserverPort, configs.Kube.ApiserverToken, configs.Kube.ApiserverDiscoveryToken)

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

// Increase log size per container
func IncreaseLogSizePerContainer() error {
	_, err := utils.ExecShellCmd(`sudo echo "containerLogMaxSize: 512Mi" > >(sudo tee -a /var/lib/kubelet/config.yaml >/dev/null)`)
	if err != nil {
		return err
	}

	_, err = utils.ExecShellCmd(`sudo systemctl restart kubelet`)
	if err != nil {
		return err
	}

	time.Sleep(15 * time.Second)
	return nil
}
