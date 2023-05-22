package knative

import (
	"flag"
	"fmt"
	"os"

	configs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/configs"
	logs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/logs"
	system "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/system"
)

func ParseSubcommandKnative(args []string) {
	nodeRole := args[0]
	operation := args[1]

	// Check nodeRole
	if (nodeRole != "master") && (nodeRole != "worker") {
		logs.InfoPrintf("Usage: %s %s <master | worker> init [parameters...]\n", os.Args[0], os.Args[1])
		logs.FatalPrintf("Invalid nodeRole: <nodeRole> -> %s\n", nodeRole)
	}

	// Check operation
	if operation != "init" {
		logs.InfoPrintf("Usage: %s %s %s init [parameters...]\n", os.Args[0], os.Args[1], nodeRole)
		logs.FatalPrintf("Invalid operation: <operation> -> %s\n", operation)
	}

	// Parse parameters for `knative master/worker init`
	var help bool
	knativeFlagsName := fmt.Sprintf("%s yurt %s %s", os.Args[0], nodeRole, operation)
	knativeFlags := flag.NewFlagSet(knativeFlagsName, flag.ExitOnError)
	knativeFlags.BoolVar(&help, "help", false, "Show help")
	knativeFlags.BoolVar(&help, "h", false, "Show help")
	knativeFlags.StringVar(&configs.Knative.KnativeVersion, "knative-version", configs.Knative.KnativeVersion, "Knative version")
	knativeFlags.StringVar(&configs.Knative.IstioVersion, "istio-version", configs.Knative.IstioVersion, "Istio version")
	knativeFlags.StringVar(&configs.Knative.MetalLBVersion, "metalLB-version", configs.Knative.MetalLBVersion, "MetalLB version")
	knativeFlags.BoolVar(&configs.Knative.VHiveMode, "vhive-mode", configs.Knative.VHiveMode, "vHive mode")
	knativeFlags.Parse(args[2:])
	// Show help
	if help {
		knativeFlags.Usage()
		os.Exit(0)
	}

	var vHiveMode string
	if configs.Knative.VHiveMode {
		vHiveMode = "true"
	} else {
		vHiveMode = "false"
	}

	InstallKnativeServing()
	InstallKnativeEventing()

	logs.SuccessPrintf("Init Knative Successfully! (vHive mode: %s)\n", vHiveMode)
}

// Install Knative Serving
func InstallKnativeServing() {
	var err error

	system.CreateTmpDir()
	defer system.CleanUpTmpDir()

	// Install and configure MetalLB
	logs.WaitPrintf("Installing and configuring MetalLB")
	_, err = system.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml | sed -e "s/strictARP: false/strictARP: true/" | kubectl apply -f - -n kube-system`)
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = system.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", configs.Knative.MetalLBVersion)
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = system.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=90s --for=condition=Available")
	logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	for _, value := range configs.Knative.MetalLBConfigURLArray {
		_, err = system.ExecShellCmd("kubectl apply -f %s", value)
		logs.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	}
	logs.SuccessPrintf("\n")

	// Install istio
	// Download istio
	logs.WaitPrintf("Downloading istio")
	istioFilePath, err := system.DownloadToTmpDir(configs.Knative.GetIstioDownloadUrl())
	logs.CheckErrorWithTagAndMsg(err, "Failed to download istio!")
	// Extract istio
	logs.WaitPrintf("Extracting istio")
	err = system.ExtractToDir(istioFilePath, "/usr/local", true)
	logs.CheckErrorWithTagAndMsg(err, "Failed to extract istio!")
	// Update PATH
	err = system.AppendDirToPath("/usr/local/istio-%s/bin", configs.Knative.IstioVersion)
	logs.CheckErrorWithMsg(err, "Failed to update PATH!")
	// Deploy istio operator
	logs.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := system.DownloadToTmpDir(configs.Knative.IstioOperatorConfigUrl)
	logs.CheckErrorWithMsg(err, "Failed to deploy istio operator!")
	_, err = system.ExecShellCmd("/usr/local/istio-%s/bin/istioctl install -y -f %s", configs.Knative.IstioVersion, operatorConfigPath)
	logs.CheckErrorWithTagAndMsg(err, "Failed to deploy istio operator!")

	// Install Knative Serving component
	logs.WaitPrintf("Installing Knative Serving component")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithMsg(err, "Failed to install Knative Serving component!")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!")

	// Install local cluster registry
	logs.WaitPrintf("Installing local cluster registry")
	_, err = system.ExecShellCmd("kubectl create namespace registry")
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	configFilePath, err := system.DownloadToTmpDir("%s", configs.Knative.LocalRegistryVolumeConfigUrl)
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = system.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -", configs.Knative.LocalRegistryRepoVolumeSize, configFilePath)
	logs.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	_, err = system.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s", configs.Knative.LocalRegistryDockerRegistryConfigUrl, configs.Knative.LocalRegistryHostUpdateConfigUrl)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install local cluster registry!")

	// Configure Magic DNS
	logs.WaitPrintf("Configuring Magic DNS")
	_, err = system.ExecShellCmd("kubectl apply -f %s", configs.Knative.MagicDNSConfigUrl)
	logs.CheckErrorWithTagAndMsg(err, "Failed to configure Magic DNS!")

	// Install networking layer
	logs.WaitPrintf("Installing networking layer")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v%s/net-istio.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install networking layer!")

	// Logs for verification
	_, err = system.ExecShellCmd("kubectl get pods -n knative-serving")
	logs.CheckErrorWithMsg(err, "Verification Failed!")

	// // Configure DNS
	// logs.WaitPrintf("Configuring DNS")
	// _, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-default-domain.yaml", configs.Knative.KnativeVersion)
	// logs.CheckErrorWithTagAndMsg(err, "Failed to configure DNS!")
}

// Install Knative Eventing
func InstallKnativeEventing() {
	// Install Knative Eventing component
	logs.WaitPrintf("Installing Knative Eventing component")
	_, err := system.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-crds.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-core.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install Knative Eventing component!")

	// Logs for verification
	_, err = system.ExecShellCmd("kubectl get pods -n knative-eventing")
	logs.CheckErrorWithMsg(err, "Verification Failed!")

	// Install a default Channel (messaging) layer
	logs.WaitPrintf("Installing a default Channel (messaging) layer")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/in-memory-channel.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install a default Channel (messaging) layer!")

	// Install a Broker layer
	logs.WaitPrintf("Installing a Broker layer")
	_, err = system.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/mt-channel-broker.yaml", configs.Knative.KnativeVersion)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install a Broker layer!")

	// Logs for verification
	_, err = system.ExecShellCmd("kubectl --namespace istio-system get service istio-ingressgateway")
	logs.CheckErrorWithMsg(err, "Verification Failed!")
}
