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

package cloudlab

import (
	"time"

	cluster "github.com/vhive-serverless/vHive/scripts/cluster"
	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func StartOnenodeVhiveCluster(sandbox, schedulerName string) error {

	// Arguments check
	if sandbox == "" {
		sandbox = "firecracker"
	}
	switch sandbox {
	case "gvisor":
	case "firecracker":
	default:
		utils.FatalPrintf("Specified sanboxing technique is not supported. Possible are \"firecracker\" and \"gvisor\"!\n")
		return &utils.ShellError{Msg: "Sandbox technique not supported!", ExitCode: 1}
	}

	// Clean up host resources
	utils.WaitPrintf("Cleaning up host resources if left after previous runs")
	cleanCriRunnerScriptPath := "scripts/github_runner/clean_cri_runner.sh"
	_, err := utils.ExecVHiveBashScript(cleanCriRunnerScriptPath, sandbox)
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
	switch sandbox {
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
	_, err = utils.ExecShellCmd("cd %s && source /etc/profile && go build", configs.VHive.VHiveRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to build vHive!\n") {
		return err
	}

	// Run vHive
	githubVHiveArgs := utils.GetEnvironmentVariable("GITHUB_VHIVE_ARGS")
	utils.WaitPrintf("Running vHive with \"%s\" arguments", githubVHiveArgs)
	vhiveExecutablePath, err := utils.GetVHiveFilePath("vhive")
	if !utils.CheckErrorWithMsg(err, "Failed to find vHive executable!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("sudo %s -sandbox %s %s 1>%s/orch.out 2>%s/orch.err &",
		vhiveExecutablePath,
		sandbox,
		githubVHiveArgs,
		ctrdLogDir,
		ctrdLogDir)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to run vHive!\n") {
		return err
	}

	time.Sleep(1 * time.Second)

	utils.InfoPrintf("Create one node cluster\n")
	if err := cluster.CreateOneNodeCluster(sandbox); err != nil {
		return err
	}

	if err = cluster.SetupMasterNode(sandbox, schedulerName); err != nil {
		return err
	}

	utils.InfoPrintf("All logs are stored in %s", ctrdLogDir)
	return nil
}
