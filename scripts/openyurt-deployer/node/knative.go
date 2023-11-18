package node

import (
	"fmt"

	"github.com/vhive-serverless/vHive/scripts/utils"
)

// Install Knative Serving
func (node *Node) InstallKnativeServing() {
	node.OnlyExecByMaster()
	var err error

	node.CreateTmpDir()

	// Install and configure MetalLB
	utils.WaitPrintf("Installing and configuring MetalLB")
	_, err = node.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml | sed -e "s/strictARP: false/strictARP: true/" | kubectl apply -f - -n kube-system`)
	utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = node.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", node.Configs.Knative.MetalLBVersion)
	utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = node.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=90s --for=condition=Available")
	utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	for _, value := range node.Configs.Knative.MetalLBConfigURLArray {
		_, err = node.ExecShellCmd("kubectl apply -f %s", value)
		utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	}
	utils.SuccessPrintf("\n")

	// Install istio
	// Download istio
	utils.WaitPrintf("Downloading istio")
	istioFilePath, err := node.DownloadToTmpDir(node.GetIstioDownloadUrl())
	utils.CheckErrorWithTagAndMsg(err, "Failed to download istio!")
	// Extract istio
	utils.WaitPrintf("Extracting istio")
	err = node.ExtractToDir(istioFilePath, "/usr/local", true)
	utils.CheckErrorWithTagAndMsg(err, "Failed to extract istio!")
	// Update PATH
	err = node.AppendDirToPath("/usr/local/istio-%s/bin", node.Configs.Knative.IstioVersion)
	utils.CheckErrorWithMsg(err, "Failed to update PATH!")
	// Deploy istio operator
	utils.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := node.DownloadToTmpDir(node.Configs.Knative.IstioOperatorConfigUrl)
	utils.CheckErrorWithMsg(err, "Failed to deploy istio operator!")
	_, err = node.ExecShellCmd("/usr/local/istio-%s/bin/istioctl install -y -f %s", node.Configs.Knative.IstioVersion, operatorConfigPath)
	utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio operator!")

	// Install Knative Serving component
	utils.WaitPrintf("Installing Knative Serving component")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Serving component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!")

	// Install local cluster registry
	utils.WaitPrintf("Installing local cluster registry")
	_, err = node.ExecShellCmd("kubectl create namespace registry")
	utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	configFilePath, err := node.DownloadToTmpDir("%s", node.Configs.Knative.LocalRegistryVolumeConfigUrl)
	utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = node.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -", node.Configs.Knative.LocalRegistryRepoVolumeSize, configFilePath)
	utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = node.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s", node.Configs.Knative.LocalRegistryDockerRegistryConfigUrl, node.Configs.Knative.LocalRegistryHostUpdateConfigUrl)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install local cluster registry!")

	// Configure Magic DNS
	utils.WaitPrintf("Configuring Magic DNS")
	_, err = node.ExecShellCmd("kubectl apply -f %s", node.Configs.Knative.MagicDNSConfigUrl)
	utils.CheckErrorWithTagAndMsg(err, "Failed to configure Magic DNS!")

	// Install networking layer
	utils.WaitPrintf("Installing networking layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v%s/net-istio.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install networking layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-serving")
	utils.CheckErrorWithMsg(err, "Verification Failed!")

	// // Configure DNS
	// logs.WaitPrintf("Configuring DNS")
	// _, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-default-domain.yaml", node.Configs.Knative.KnativeVersion)
	// logs.CheckErrorWithTagAndMsg(err, "Failed to configure DNS!")

	// enable node selector
	utils.WaitPrintf("Enable node selector in knative serving")
	_, err = node.ExecShellCmd(`kubectl patch cm config-features -n knative-serving \
  --type merge \
  -p '{"data":{"kubernetes.podspec-nodeselector":"enabled"}}'
`)
	utils.CheckErrorWithTagAndMsg(err, "Failed to enable node selector in knative serving")
	// node.enableNodeSelect()
}

// Install Knative Eventing
func (node *Node) InstallKnativeEventing() {
	// Install Knative Eventing component
	utils.WaitPrintf("Installing Knative Eventing component")
	_, err := node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-crds.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-core.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Eventing component!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-eventing")
	utils.CheckErrorWithMsg(err, "Verification Failed!")

	// Install a default Channel (messaging) layer
	utils.WaitPrintf("Installing a default Channel (messaging) layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/in-memory-channel.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install a default Channel (messaging) layer!")

	// Install a Broker layer
	utils.WaitPrintf("Installing a Broker layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/mt-channel-broker.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithTagAndMsg(err, "Failed to install a Broker layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl --namespace istio-system get service istio-ingressgateway")
	utils.CheckErrorWithMsg(err, "Verification Failed!")
}

// get istio download URL
func (node *Node) GetIstioDownloadUrl() string {
	knative := node.Configs.Knative
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, node.Configs.System.CurrentArch)
}
