package cloudlab

import (
	"time"

	cluster "github.com/vhive-serverless/vHive/scripts/cluster"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func StartOnenodeVhiveCluster(sandBox string) error {

	// Arguments check
	if sandBox == "" {
		sandBox = "firecracker"
	}
	switch sandBox {
	case "gvisor":
	case "firecracker":
	default:
		utils.FatalPrintf("Specified sanboxing technique is not supported. Possible are \"firecracker\" and \"gvisor\"!\n")
		return &utils.ShellError{Msg: "Sandbox technique not supported!", ExitCode: 1}
	}

	// Clean up host resources
	utils.WaitPrintf("Cleaning up host resources if left after previous runs")
	cleanCriRunnerScriptPath := "scripts/github_runner/clean_cri_runner.sh"
	_, err := utils.ExecVHiveBashScript(cleanCriRunnerScriptPath, sandBox)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to clean up host resources!\n") {
		return err
	}

	// Create Log directory
	githubRunId := utils.GetEnvironmentVariable("GITHUB_RUN_ID")
	ctrdLogDir := "/tmp/ctrd-logs/" + githubRunId
	if _, err := utils.ExecShellCmd("sudo mkdir -p -m777 -p %s", ctrdLogDir); err != nil {
		return err
	}

	// Run the stock containerd daemon
	utils.WaitPrintf("Running the stock containerd daemon")
	_, err = utils.ExecShellCmd("sudo containerd 1>%s/ctrd.out 2>%s/ctrd.err &", ctrdLogDir, ctrdLogDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to run the stock containerd daemon!\n") {
		return err
	}

	// Sleep 1 second
	time.Sleep(1 * time.Second)

	// Run the containerd daemon
	switch sandBox {
	case "gvisor":
		utils.WaitPrintf("Running the gvisor-containerd daemon")
		_, err := utils.ExecShellCmd("sudo /usr/local/bin/gvisor-containerd --address /run/gvisor-containerd/gvisor-containerd.sock --config /etc/gvisor-containerd/config.toml 1>%s/gvisor.out 2>%s/gvisor.err &",
			ctrdLogDir, ctrdLogDir)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to run the gvisor-containerd daemon!\n") {
			return err
		}
	case "firecracker":
		utils.WaitPrintf("Running the firecracker-containerd daemon")
		_, err := utils.ExecShellCmd("sudo /usr/local/bin/firecracker-containerd --config /etc/firecracker-containerd/config.toml 1>%s/fccd.out 2>%s/fccd.err &",
			ctrdLogDir, ctrdLogDir)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to run the firecracker-containerd daemon!\n") {
			return err
		}
	default:
	}

	// Sleep 1 second
	time.Sleep(1 * time.Second)

	// Build vHive
	utils.WaitPrintf("Building vHive")
	_, err = utils.ExecShellCmd("cd %s && source /etc/profile && go build")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to build vHive!\n") {
		return err
	}

	// Run vHive
	githubVHiveArgs := utils.GetEnvironmentVariable("GITHUB_VHIVE_ARGS")
	utils.WaitPrintf("Running vHive with \"%s\" arguments", githubVHiveArgs)
	_, err = utils.ExecShellCmd("sudo ./vhive -sandbox %s %s 1>%s/orch.out 2>%s/orch.err &",
		sandBox,
		githubVHiveArgs,
		ctrdLogDir,
		ctrdLogDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to run vHive!\n") {
		return err
	}

	time.Sleep(time.Second)

	utils.InfoPrintf("Create one node cluster\n")
	if err := cluster.CreateOneNodeCluster(sandBox); err != nil {
		return err
	}

	utils.InfoPrintf("All logs are stored in %s", ctrdLogDir)
	return nil
}
