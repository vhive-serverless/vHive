package kube

import (
	"flag"
	"fmt"
	"os"
	"strings"

	configs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/configs"
	logs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/logs"
	system "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/system"
)

// Parse parameters for subcommand `kube`
func ParseSubcommandKube(args []string) {
	nodeRole := args[0]
	operation := args[1]
	var help bool
	// Add parameters to flag set
	kubeFlagsName := fmt.Sprintf("%s kube %s %s", os.Args[0], nodeRole, operation)
	kubeFlags := flag.NewFlagSet(kubeFlagsName, flag.ExitOnError)
	kubeFlags.BoolVar(&help, "help", false, "Show help")
	kubeFlags.BoolVar(&help, "h", false, "Show help")
	switch nodeRole {
	case "master":
		// Parse parameters for `kube master init`
		if operation != "init" {
			logs.InfoPrintf("Usage: %s %s %s init [parameters...]\n", os.Args[0], os.Args[1], nodeRole)
			logs.FatalPrintf("Invalid operation: <operation> -> %s\n", operation)
		}
		kubeFlags.StringVar(&configs.Kube.K8sVersion, "k8s-version", configs.Kube.K8sVersion, "Kubernetes version")
		kubeFlags.StringVar(&configs.Kube.AlternativeImageRepo, "alternative-image-repo", configs.Kube.AlternativeImageRepo, "Alternative image repository")
		kubeFlags.StringVar(&configs.Kube.ApiserverAdvertiseAddress, "apiserver-advertise-address", configs.Kube.ApiserverAdvertiseAddress, "Kubernetes API server advertise address")
		kubeFlags.Parse(args[2:])
		// Show help
		if help {
			kubeFlags.Usage()
			os.Exit(0)
		}
		kube_master_init()
		logs.SuccessPrintf("Master node key information has been written to %s/masterKey.yaml! Check for details.\n", configs.System.CurrentDir)
	case "worker":
		// Parse parameters for `kube worker join`
		if operation != "join" {
			logs.InfoPrintf("Usage: %s %s %s join [parameters...]\n", os.Args[0], os.Args[1], nodeRole)
			logs.FatalPrintf("Invalid operation: <operation> -> %s\n", operation)
		}
		kubeFlags.StringVar(&configs.Kube.ApiserverAdvertiseAddress, "apiserver-advertise-address", configs.Kube.ApiserverAdvertiseAddress, "Kubernetes API server advertise address (**REQUIRED**)")
		kubeFlags.StringVar(&configs.Kube.ApiserverPort, "apiserver-port", configs.Kube.ApiserverPort, "Kubernetes API server port")
		kubeFlags.StringVar(&configs.Kube.ApiserverToken, "apiserver-token", configs.Kube.ApiserverToken, "Kubernetes API server token (**REQUIRED**)")
		kubeFlags.StringVar(&configs.Kube.ApiserverTokenHash, "apiserver-token-hash", configs.Kube.ApiserverTokenHash, "Kubernetes API server token hash (**REQUIRED**)")
		kubeFlags.Parse(args[2:])
		// Show help
		if help {
			kubeFlags.Usage()
			os.Exit(0)
		}
		// Check required parameters
		if len(configs.Kube.ApiserverAdvertiseAddress) == 0 {
			kubeFlags.Usage()
			logs.FatalPrintf("Parameter --apiserver-advertise-address needed!\n")
		}
		if len(configs.Kube.ApiserverToken) == 0 {
			kubeFlags.Usage()
			logs.FatalPrintf("Parameter --apiserver-token needed!\n")
		}
		if len(configs.Kube.ApiserverTokenHash) == 0 {
			kubeFlags.Usage()
			logs.FatalPrintf("Parameter --apiserver-token-hash needed!\n")
		}
		kube_worker_join()
		logs.SuccessPrintf("Successfully joined Kubernetes cluster!\n")
	default:
		logs.InfoPrintf("Usage: %s %s <master | worker> <init | join> [parameters...]\n", os.Args[0], os.Args[1])
		logs.FatalPrintf("Invalid nodeRole: <nodeRole> -> %s\n", nodeRole)
	}
}

// Initialize the master node of Kubernetes cluster
func kube_master_init() {

	// Initialize
	var err error
	check_kube_environment()
	system.CreateTmpDir()
	defer system.CleanUpTmpDir()

	// Pre-pull Image
	logs.WaitPrintf("Pre-Pulling required images")
	shellCmd := fmt.Sprintf("sudo kubeadm config images pull --kubernetes-version %s ", configs.Kube.K8sVersion)
	if len(configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", configs.Kube.AlternativeImageRepo)
	}
	_, err = system.ExecShellCmd(shellCmd)
	logs.CheckErrorWithTagAndMsg(err, "Failed to pre-pull required images!\n")

	// Deploy Kubernetes
	logs.WaitPrintf("Deploying Kubernetes(version %s)", configs.Kube.K8sVersion)
	shellCmd = fmt.Sprintf("sudo kubeadm init --kubernetes-version %s --pod-network-cidr=\"%s\" ", configs.Kube.K8sVersion, configs.Kube.PodNetworkCidr)
	if len(configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", configs.Kube.AlternativeImageRepo)
	}
	if len(configs.Kube.ApiserverAdvertiseAddress) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--apiserver-advertise-address=%s ", configs.Kube.ApiserverAdvertiseAddress)
	}
	shellCmd = fmt.Sprintf(shellCmd+"| tee %s/masterNodeInfo", configs.System.TmpDir)
	_, err = system.ExecShellCmd(shellCmd)
	logs.CheckErrorWithTagAndMsg(err, "Failed to deploy Kubernetes(version %s)!\n", configs.Kube.K8sVersion)

	// Make kubectl work for non-root user
	logs.WaitPrintf("Making kubectl work for non-root user")
	_, err = system.ExecShellCmd("mkdir -p %s/.kube && sudo cp -i /etc/kubernetes/admin.conf %s/.kube/config && sudo chown $(id -u):$(id -g) %s/.kube/config",
		configs.System.UserHomeDir,
		configs.System.UserHomeDir,
		configs.System.UserHomeDir)
	logs.CheckErrorWithTagAndMsg(err, "Failed to make kubectl work for non-root user!\n")

	// Install Calico network add-on
	logs.WaitPrintf("Installing pod network")
	_, err = system.ExecShellCmd("kubectl apply -f %s", configs.Kube.PodNetworkAddonConfigURL)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install pod network!\n")

	// Extract master node information from logs
	logs.WaitPrintf("Extracting master node information from logs")
	shellOut, err := system.ExecShellCmd("sed -n '/.*kubeadm join.*/p' < %s/masterNodeInfo | sed -n 's/.*join \\(.*\\):\\(\\S*\\) --token \\(\\S*\\).*/\\1 \\2 \\3/p'", configs.System.TmpDir)
	logs.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n")
	splittedOut := strings.Split(shellOut, " ")
	configs.Kube.ApiserverAdvertiseAddress = splittedOut[0]
	configs.Kube.ApiserverPort = splittedOut[1]
	configs.Kube.ApiserverToken = splittedOut[2]
	shellOut, err = system.ExecShellCmd("sed -n '/.*sha256:.*/p' < %s/masterNodeInfo | sed -n 's/.*\\(sha256:\\S*\\).*/\\1/p'", configs.System.TmpDir)
	logs.CheckErrorWithTagAndMsg(err, "Failed to extract master node information from logs!\n")
	configs.Kube.ApiserverTokenHash = shellOut
	masterKeyYamlTemplate := `ApiserverAdvertiseAddress: %s
ApiserverPort: %s
ApiserverToken: %s
ApiserverTokenHash: %s`

	// Create masterKey.yaml with master node information
	logs.WaitPrintf("Creating masterKey.yaml with master node information")
	masterKeyYamlFile, err := os.OpenFile(configs.System.CurrentDir+"/masterKey.yaml", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	logs.CheckErrorWithMsg(err, "Failed to create masterKey.yaml with master node information!\n")
	defer masterKeyYamlFile.Close()
	masterKeyYaml := fmt.Sprintf(
		masterKeyYamlTemplate,
		configs.Kube.ApiserverAdvertiseAddress,
		configs.Kube.ApiserverPort,
		configs.Kube.ApiserverToken,
		configs.Kube.ApiserverTokenHash)
	_, err = masterKeyYamlFile.WriteString(masterKeyYaml)
	logs.CheckErrorWithTagAndMsg(err, "Failed to create masterKey.yaml with master node information!\n")

}

// Join worker node to Kubernetes cluster
func kube_worker_join() {

	// Initialize
	var err error

	// Join Kubernetes cluster
	logs.WaitPrintf("Joining Kubernetes cluster")
	_, err = system.ExecShellCmd("sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s", configs.Kube.ApiserverAdvertiseAddress, configs.Kube.ApiserverPort, configs.Kube.ApiserverToken, configs.Kube.ApiserverTokenHash)
	logs.CheckErrorWithTagAndMsg(err, "Failed to join Kubernetes cluster!\n")
}

func check_kube_environment() {
	// Temporarily unused
}
