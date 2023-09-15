package node

import (
	"fmt"

	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
)

// Install Knative Serving
func (node *Node) InstallKnativeServing() {
	node.OnlyExecByMaster()
	var err error

	node.CreateTmpDir()
	defer node.CleanUpTmpDir()

	// Install and configure MetalLB
	logs.WaitPrintf("Installing and configuring MetalLB")
	_, err = node.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml | sed -e "s/strictARP: false/strictARP: true/" | kubectl apply -f - -n kube-system`)
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = node.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", node.Configs.Knative.MetalLBVersion)
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = node.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=90s --for=condition=Available")
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	for _, value := range node.Configs.Knative.MetalLBConfigURLArray {
		_, err = node.ExecShellCmd("kubectl apply -f %s", value)
		logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	}
	logs.SuccessPrintf("\n")

	// Install istio
	// Download istio
	logs.WaitPrintf("Downloading istio")
	istioFilePath, err := node.DownloadToTmpDir(node.GetIstioDownloadUrl())
	logs.CheckErrorWithTagAndMsg(err, "Failed to download istio!")
	// Extract istio
	logs.WaitPrintf("Extracting istio")
	err = node.ExtractToDir(istioFilePath, "/usr/local", true)
	logs.CheckErrorWithTagAndMsg(err, "Failed to extract istio!")
	// Update PATH
	err = node.AppendDirToPath("/usr/local/istio-%s/bin", node.Configs.Knative.IstioVersion)
	logs.CheckErrorWithMsg(err, "Failed to update PATH!")
	// Deploy istio operator
	logs.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := node.DownloadToTmpDir(node.Configs.Knative.IstioOperatorConfigUrl)
	logs.CheckErrorWithMsg(err, "Failed to deploy istio operator!")
	_, err = node.ExecShellCmd("/usr/local/istio-%s/bin/istioctl install -y -f %s", node.Configs.Knative.IstioVersion, operatorConfigPath)
	logs.CheckErrorWithTagAndMsg(err, "Failed to deploy istio operator!")

	// Install Knative Serving component
	logs.WaitPrintf("Installing Knative Serving component")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithMsg(err, "Failed to install Knative Serving component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!")

	// Install local cluster registry
	logs.WaitPrintf("Installing local cluster registry")
	_, err = node.ExecShellCmd("kubectl create namespace registry")
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	configFilePath, err := node.DownloadToTmpDir("%s", node.Configs.Knative.LocalRegistryVolumeConfigUrl)
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = node.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -", node.Configs.Knative.LocalRegistryRepoVolumeSize, configFilePath)
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = node.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s", node.Configs.Knative.LocalRegistryDockerRegistryConfigUrl, node.Configs.Knative.LocalRegistryHostUpdateConfigUrl)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install local cluster registry!")

	// Configure Magic DNS
	logs.WaitPrintf("Configuring Magic DNS")
	_, err = node.ExecShellCmd("kubectl apply -f %s", node.Configs.Knative.MagicDNSConfigUrl)
	logs.CheckErrorWithTagAndMsg(err, "Failed to configure Magic DNS!")

	// Install networking layer
	logs.WaitPrintf("Installing networking layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v%s/net-istio.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install networking layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-serving")
	logs.CheckErrorWithMsg(err, "Verification Failed!")

	// // Configure DNS
	// logs.WaitPrintf("Configuring DNS")
	// _, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-default-domain.yaml", node.Configs.Knative.KnativeVersion)
	// logs.CheckErrorWithTagAndMsg(err, "Failed to configure DNS!")

	// enable node selector
	logs.WaitPrintf("Enable node selector in knative serving")
	_, err = node.ExecShellCmd(`kubectl patch cm config-features -n knative-serving \
  --type merge \
  -p '{"data":{"kubernetes.podspec-nodeselector":"enabled"}}'
`)
	logs.CheckErrorWithTagAndMsg(err, "Failed to enable node selector in knative serving")
	// node.enableNodeSelect()
}

// Install Knative Eventing
func (node *Node) InstallKnativeEventing() {
	// Install Knative Eventing component
	logs.WaitPrintf("Installing Knative Eventing component")
	_, err := node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-crds.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-core.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install Knative Eventing component!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-eventing")
	logs.CheckErrorWithMsg(err, "Verification Failed!")

	// Install a default Channel (messaging) layer
	logs.WaitPrintf("Installing a default Channel (messaging) layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/in-memory-channel.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install a default Channel (messaging) layer!")

	// Install a Broker layer
	logs.WaitPrintf("Installing a Broker layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/mt-channel-broker.yaml", node.Configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install a Broker layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl --namespace istio-system get service istio-ingressgateway")
	logs.CheckErrorWithMsg(err, "Verification Failed!")
}

// get istio download URL
func (node *Node) GetIstioDownloadUrl() string {
	knative := node.Configs.Knative
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, node.Configs.System.CurrentArch)
}

// enable nodeselector and tolerations in knative serving
// func (node *Node) enableNodeSelect() {
// 	var err error
// 	tmp_config_fname := "tmp_config.yaml"
// 	logs.WaitPrintf("Extracting kn-serving config")
// 	_, err = node.ExecShellCmd("kubectl -n knative-serving get cm config-features -o  yaml > %s", tmp_config_fname)
// 	logs.CheckErrorWithTagAndMsg(err, "Failed to extract kn-serving config")

// 	logs.WaitPrintf("Writing new cnfigs into tmp config file")
// 	_, err = node.ExecShellCmd(`sed -i '/_example:/i \ \ kubernetes.podspec-nodeselector: "enabled"\n\ \ kubernetes.podspec-tolerations: "enabled"' %s`, tmp_config_fname)
// 	logs.CheckErrorWithTagAndMsg(err, "Failed to write new configs into tmp config file!")

// 	logs.WaitPrintf("Writing config file back to kn-serving config")
// 	_, err = node.ExecShellCmd("kubectl -n knative-serving apply -f %s", tmp_config_fname)
// 	logs.CheckErrorWithTagAndMsg(err, "Failed to write config file back to kn-serving config")

// 	logs.WaitPrintf("Cleaning the tmp config file")
// 	_, err = node.ExecShellCmd("rm %s", tmp_config_fname)
// 	logs.CheckErrorWithTagAndMsg(err, "Failed to clean tmp config file!")

// }