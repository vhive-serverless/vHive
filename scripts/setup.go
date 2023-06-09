package main

import (
	"flag"
	"fmt"
	"os"

	cloudlab "github.com/vhive-serverless/vHive/scripts/cloudlab"
	cluster "github.com/vhive-serverless/vHive/scripts/cluster"
	configs "github.com/vhive-serverless/vHive/scripts/configs"
	gpu "github.com/vhive-serverless/vHive/scripts/gpu"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func main() {
	var err error
	// Detect and prepare for the environment
	if err = utils.PrepareEnvironment(); err != nil {
		os.Exit(1)
	}
	defer utils.CleanEnvironment()

	// Set up arguments
	var help bool
	setupFlagsName := os.Args[0]
	setupFlags := flag.NewFlagSet(setupFlagsName, flag.ExitOnError)
	setupFlags.StringVar(&configs.VHive.VHiveSetupConfigPath, "setup-configs-dir", configs.VHive.VHiveSetupConfigPath, "Config directory for setting up vHive (left blank to use default configs in vHive repo)")
	setupFlags.StringVar(&configs.VHive.VHiveRepoPath, "vhive-repo-dir", configs.VHive.VHiveRepoPath, "vHive repo path (left blank to use online repo automatically)")
	setupFlags.StringVar(&configs.VHive.VHiveRepoBranch, "vhive-repo-branch", configs.VHive.VHiveRepoBranch, "vHive repo branch (valid only when using online repo)")
	setupFlags.StringVar(&configs.VHive.VHiveRepoUrl, "vhive-repo-url", configs.VHive.VHiveRepoUrl, "vHive repo url (valid only when using online repo)")
	setupFlags.BoolVar(&help, "help", false, "Show help")
	setupFlags.BoolVar(&help, "h", false, "Show help")

	// Parse arguments
	setupFlags.Parse(os.Args[1:])
	// Show help
	if help {
		setupFlags.Usage()
		return
	}

	if setupFlags.NArg() < 1 {
		utils.FatalPrintf("Missing subcommand! Script terminated!\n")
		return
	}

	// Create logs
	if err = utils.CreateLogs(configs.System.CurrentDir); err != nil {
		utils.CleanEnvironment()
		os.Exit(1)
	}

	subCmd := setupFlags.Args()[0]
	availableCmds := []string{
		"create_multinode_cluster",
		"create_one_node_cluster",
		"setup_master_node",
		"setup_worker_kubelet",
		"setup_node",
		"start_onenode_vhive_cluster",
		"setup_nvidia_gpu",
	}

	// Check config directory
	if len(configs.VHive.VHiveSetupConfigPath) == 0 {
		utils.CheckVHiveRepo()
		configs.VHive.VHiveSetupConfigPath, err = utils.GetVHiveFilePath("configs/setup")
		if err != nil {
			utils.CleanEnvironment()
			os.Exit(1)
		}
	}
	// load config file
	utils.WaitPrintf("Loading config files from %s", configs.VHive.VHiveSetupConfigPath)
	if err = configs.VHive.LoadConfig(); !utils.CheckErrorWithMsg(err, "Failed to load config files!\n") {
		utils.CleanEnvironment()
		os.Exit(1)
	}
	if err = configs.System.LoadConfig(); !utils.CheckErrorWithMsg(err, "Failed to load config files!\n") {
		utils.CleanEnvironment()
		os.Exit(1)
	}
	if err = configs.Kube.LoadConfig(); !utils.CheckErrorWithMsg(err, "Failed to load config files!\n") {
		utils.CleanEnvironment()
		os.Exit(1)
	}
	if err = configs.Knative.LoadConfig(); !utils.CheckErrorWithMsg(err, "Failed to load config files!\n") {
		utils.CleanEnvironment()
		os.Exit(1)
	}
	utils.SuccessPrintf("\n")

	// Execute corresponding scripts
	switch subCmd {
	// Original scripts from `scripts/cluster` directory
	case "create_multinode_cluster":
		if setupFlags.NArg() < 2 {
			utils.FatalPrintf("Missing parameters: %s <stock-containerd>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Create multinode cluster\n")
		err = cluster.CreateMultinodeCluster(setupFlags.Args()[1])
	case "create_one_node_cluster":
		if setupFlags.NArg() < 2 {
			utils.FatalPrintf("Missing parameters: %s <stock-containerd>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Create one-node Cluster\n")
		err = cluster.CreateOneNodeCluster(setupFlags.Args()[1])
	case "setup_master_node":
		if setupFlags.NArg() < 2 {
			utils.FatalPrintf("Missing parameters: %s <stock-containerd>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Set up master node\n")
		err = cluster.SetupMasterNode(setupFlags.Args()[1])
	case "setup_worker_kubelet":
		if setupFlags.NArg() < 2 {
			utils.FatalPrintf("Missing parameters: %s <stock-containerd>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Set up worker kubelet\n")
		err = cluster.SetupWorkerKubelet(setupFlags.Args()[1])
		// Original scripts from `scripts/cloudlab` directory
	case "setup_node":
		if setupFlags.NArg() < 3 {
			utils.FatalPrintf("Missing parameters: %s <sandbox> <use-stargz>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Set up node\n")
		err = cloudlab.SetupNode(setupFlags.Args()[1], setupFlags.Args()[2])
	case "start_onenode_vhive_cluster":
		if setupFlags.NArg() < 2 {
			utils.FatalPrintf("Missing parameters: %s <sandbox>\n", subCmd)
			utils.CleanEnvironment()
			os.Exit(1)
		}
		utils.InfoPrintf("Start one-node vHive cluster\n")
		err = cloudlab.StartOnenodeVhiveCluster(setupFlags.Args()[1])
		// Original scripts from `scripts/cloudlab` directory
	case "setup_nvidia_gpu":
		utils.InfoPrintf("Set up Nvidia gpu\n")
		err = gpu.SetupNvidiaGpu()
	default:
		utils.FatalPrintf("Invalid subcommand --> %s! Available subcommands list: \n", subCmd)
		for _, subCmd := range availableCmds {
			fmt.Printf("%s\n", subCmd)
		}
		utils.CleanEnvironment()
		os.Exit(1)
	}

	if err != nil {
		utils.FatalPrintf("Faild subcommand: %s!\n", subCmd)
		utils.CleanEnvironment()
		os.Exit(1)
	}
}
