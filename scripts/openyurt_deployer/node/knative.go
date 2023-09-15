package node

import (
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
)

// func (node *Node) ParseSubcommandKnative(args []string) {
// 	nodeRole := args[0]
// 	operation := args[1]

// 	// Check nodeRole
// 	if (nodeRole != "master") && (nodeRole != "worker") {
// 		logs.InfoPrintf("Usage: %s %s <master | worker> init [parameters...]\n", os.Args[0], os.Args[1])
// 		logs.FatalPrintf("Invalid nodeRole: <nodeRole> -> %s\n", nodeRole)
// 	}

// 	// Check operation
// 	if operation != "init" {
// 		logs.InfoPrintf("Usage: %s %s %s init [parameters...]\n", os.Args[0], os.Args[1], nodeRole)
// 		logs.FatalPrintf("Invalid operation: <operation> -> %s\n", operation)
// 	}

// 	// Parse parameters for `knative master/worker init`
// 	var help bool
// 	knativeFlagsName := fmt.Sprintf("%s yurt %s %s", os.Args[0], nodeRole, operation)
// 	knativeFlags := flag.NewFlagSet(knativeFlagsName, flag.ExitOnError)
// 	knativeFlags.BoolVar(&help, "help", false, "Show help")
// 	knativeFlags.BoolVar(&help, "h", false, "Show help")
// 	knativeFlags.StringVar(&node.Configs.Knative.KnativeVersion, "knative-version", node.Configs.Knative.KnativeVersion, "Knative version")
// 	knativeFlags.StringVar(&node.Configs.Knative.IstioVersion, "istio-version", node.Configs.Knative.IstioVersion, "Istio version")
// 	knativeFlags.StringVar(&node.Configs.Knative.MetalLBVersion, "metalLB-version", node.Configs.Knative.MetalLBVersion, "MetalLB version")
// 	knativeFlags.BoolVar(&node.Configs.Knative.VHiveMode, "vhive-mode", node.Configs.Knative.VHiveMode, "vHive mode")
// 	knativeFlags.Parse(args[2:])
// 	// Show help
// 	if help {
// 		knativeFlags.Usage()
// 		os.Exit(0)
// 	}

// 	var vHiveMode string
// 	if node.Configs.Knative.VHiveMode {
// 		vHiveMode = "true"
// 	} else {
// 		vHiveMode = "false"
// 	}

// 	node.InstallKnativeServing()
// 	node.InstallKnativeEventing()

// 	logs.SuccessPrintf("Init Knative Successfully! (vHive mode: %s)\n", vHiveMode)
// }

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
	istioFilePath, err := node.DownloadToTmpDir(node.Configs.Knative.GetIstioDownloadUrl())
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
