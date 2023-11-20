package node

import (
	"fmt"
	"strings"

	"github.com/vhive-serverless/vHive/scripts/utils"
)

// Initialize the master node of Kubernetes cluster
func (node *Node) KubeMasterInit() (string, string, string, string) {

	// Initialize
	var err error
	node.check_kube_environment()
	node.CreateTmpDir()
	defer node.CleanUpTmpDir()

	// Pre-pull Image
	utils.WaitPrintf("Pre-Pulling required images")
	shellCmd := fmt.Sprintf("sudo kubeadm config images pull --kubernetes-version %s ", node.Configs.Kube.K8sVersion)
	if len(node.Configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", node.Configs.Kube.AlternativeImageRepo)
	}
	_, err = node.ExecShellCmd(shellCmd)
	utils.CheckErrorWithTagAndMsg(err, "Failed to pre-pull required images!\n")

	// Deploy Kubernetes
	utils.WaitPrintf("Deploying Kubernetes(version %s)", node.Configs.Kube.K8sVersion)
	shellCmd = fmt.Sprintf("sudo kubeadm init --kubernetes-version %s --pod-network-cidr=\"%s\" ", node.Configs.Kube.K8sVersion, node.Configs.Kube.PodNetworkCidr)
	if len(node.Configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", node.Configs.Kube.AlternativeImageRepo)
	}
	if len(node.Configs.Kube.ApiserverAdvertiseAddress) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--apiserver-advertise-address=%s ", node.Configs.Kube.ApiserverAdvertiseAddress)
	}
	shellCmd = fmt.Sprintf(shellCmd+"| tee %s/masterNodeInfo", node.Configs.System.TmpDir)
	_, err = node.ExecShellCmd(shellCmd)
	utils.CheckErrorWithTagAndMsg(err, "Failed to deploy Kubernetes(version %s)!\n", node.Configs.Kube.K8sVersion)

	// Make kubectl work for non-root user
	utils.WaitPrintf("Making kubectl work for non-root user")
	_, err = node.ExecShellCmd("mkdir -p %s/.kube && sudo cp -i /etc/kubernetes/admin.conf %s/.kube/config && sudo chown $(id -u):$(id -g) %s/.kube/config",
		node.Configs.System.UserHomeDir,
		node.Configs.System.UserHomeDir,
		node.Configs.System.UserHomeDir)
	utils.CheckErrorWithTagAndMsg(err, "Failed to make kubectl work for non-root user!\n")

	// Install Calico network add-on
	utils.WaitPrintf("Installing pod network")
	_, err = node.ExecShellCmd("kubectl apply -f %s", node.Configs.Kube.PodNetworkAddonConfigURL)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install pod network!\n")

	// Extract master node information from logs
	utils.WaitPrintf("Extracting master node information from logs")
	shellOut, err := node.ExecShellCmd("sed -n '/.*kubeadm join.*/p' < %s/masterNodeInfo | sed -n 's/.*join \\(.*\\):\\(\\S*\\) --token \\(\\S*\\).*/\\1 \\2 \\3/p'", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n")
	splittedOut := strings.Split(shellOut, " ")
	node.Configs.Kube.ApiserverAdvertiseAddress = splittedOut[0]
	node.Configs.Kube.ApiserverPort = splittedOut[1]
	node.Configs.Kube.ApiserverToken = splittedOut[2]
	shellOut, err = node.ExecShellCmd("sed -n '/.*sha256:.*/p' < %s/masterNodeInfo | sed -n 's/.*\\(sha256:\\S*\\).*/\\1/p'", node.Configs.System.TmpDir)
	utils.CheckErrorWithTagAndMsg(err, "Failed to extract master node information from logs!\n")
	node.Configs.Kube.ApiserverTokenHash = shellOut

	return node.Configs.Kube.ApiserverAdvertiseAddress,
		node.Configs.Kube.ApiserverPort,
		node.Configs.Kube.ApiserverToken,
		node.Configs.Kube.ApiserverTokenHash

}

func (node *Node) KubeClean() {
	utils.InfoPrintf("Cleaning Kube in node: %s\n", node.Name)
	var err error
	if node.NodeRole == "master" {
		// kubectl cordon {workerNodeName}
		// kubectl drain {NodeName} --delete-local-data --force --ignore-daemonsets
		// kubectl delete node {NodeName}

		utils.WaitPrintf("Reseting kube cluster and rm .kube file")
		// TODO: delete master last, need to check defer can work or not
		defer node.ExecShellCmd("sudo kubeadm reset -f && rm -rf $HOME/.kube  && rm -rf /etc/cni/net.d")
		// The reset process does not clean CNI configuration. To do so, you must remove /etc/cni/net.d
	} else {

		utils.WaitPrintf("Reseting kube cluster")
		_, err = node.ExecShellCmd("sudo kubeadm reset -f && rm -rf /etc/cni/net.d")
	}
	utils.CheckErrorWithTagAndMsg(err, "Failed to clean kube cluster!\n")

}

// Join worker node to Kubernetes cluster
func (node *Node) KubeWorkerJoin(apiServerAddr string, apiServerPort string, apiServerToken string, apiServerTokenHash string) {

	// Initialize
	var err error

	// Join Kubernetes cluster
	utils.WaitPrintf("Joining Kubernetes cluster")
	_, err = node.ExecShellCmd("sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s", apiServerAddr, apiServerPort, apiServerToken, apiServerTokenHash)
	utils.CheckErrorWithTagAndMsg(err, "Failed to join Kubernetes cluster!\n")
}

func (node *Node) check_kube_environment() {
	// Temporarily unused
}

func (node *Node) GetAllNodes() []string {
	utils.WaitPrintf("Get all nodes...")
	if node.NodeRole != "master" {
		utils.ErrorPrintf("GetAllNodes can only be executed on master node!\n")
		return []string{}
	}
	out, err := node.ExecShellCmd("kubectl get nodes | awk 'NR>1 {print $1}'")
	utils.CheckErrorWithMsg(err, "Failed to get nodes from cluster!\n")
	nodeNames := strings.Split(out, "\n")
	return nodeNames
}
