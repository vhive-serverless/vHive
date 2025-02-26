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

package setup

import (
	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func InstallPmuTools() error {
	// Original Bash Scripts: scripts/install_pmutools.sh

	utils.WaitPrintf("Installing pmu-tools")
	// Get Linux kernel version
	kernelVersion, err := utils.GetKernelVersion()
	if !utils.CheckErrorWithMsg(err, "Failed to get kernel version!\n") {
		return err
	}

	// Install required packages (kernel version specific)
	err = utils.InstallPackages("numactl linux-tools-%s", kernelVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to install required packages!\n") {
		return err
	}

	// Clone the repo of pmu-tools to temporary directory
	repoPath, err := utils.CloneRepoToTmpDir("master", configs.System.PmuToolsRepoUrl)
	if !utils.CheckErrorWithMsg(err, "Failed to clone required repo!\n") {
		return err
	}

	// write system config
	err = utils.WriteToSysctl("kernel.perf_event_paranoid=-1")
	if !utils.CheckErrorWithMsg(err, "Failed to write sysctl.conf!\n") {
		return err
	}

	// copy and execute the tool
	err = utils.CopyToDir(repoPath, "/usr/local/", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy files to /usr/local!\n") {
		return err
	}

	_, err = utils.ExecShellCmd("/usr/local/pmu-tools/toplev --print > /dev/null")
	utils.CheckErrorWithTagAndMsg(err, "Failed to execute /usr/local/pmu-tools/toplev!\n")

	return err
}

// Install protoc
func InstallProtoc() error {
	// Download protoc
	protocFilePath, err := utils.DownloadToTmpDir(configs.System.GetProtocDownloadUrl())
	if !utils.CheckErrorWithMsg(err, "Failed to download protoc!\n") {
		return err
	}
	// Extract protoc
	err = utils.ExtractToDir(protocFilePath, "/usr/local", true)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract downloaded protoc!\n") {
		return err
	}
	// Success
	return nil
}

// Install containerd
func InstallContainerd() error {
	// Download containerd
	containerdFilePath, err := utils.DownloadToTmpDir(configs.System.GetContainerdDownloadUrl())
	if !utils.CheckErrorWithMsg(err, "Failed to Download containerd(ver %s)\n", configs.System.ContainerdVersion) {
		return err
	}
	// Extract containerd
	err = utils.ExtractToDir(containerdFilePath, "/usr/local", true)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract containerd!\n") {
		return err
	}
	// Create containerd config; enable systemd cgroup driver (recommended by k8s)
	_, err = utils.ExecShellCmd("sudo mkdir -p /etc/containerd")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create containerd config!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("containerd config default | sudo tee /etc/containerd/config.toml")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create containerd config!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to modify containerd config!\n") {
		return err
	}
	return nil
}

// Download and install runc
func InstallRunc() error {
	// Download runc
	runcFilePath, err := utils.DownloadToTmpDir(configs.System.GetRuncDownloadUrl())
	if !utils.CheckErrorWithMsg(err, "Failed to download runc(ver %s)!\n", configs.System.RuncVersion) {
		return err
	}
	// Install runc
	_, err = utils.ExecShellCmd("sudo install -m 755 %s /usr/local/sbin/runc", runcFilePath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to copy runc!\n") {
		return err
	}
	return nil
}

// Download and install runsc
func InstallRunsc() error {
	// Download runsc
	runscFilePath, err := utils.DownloadToTmpDir(configs.System.GetRunscDownloadUrl())
	if !utils.CheckErrorWithMsg(err, "Failed to download runsc!\n") {
		return err
	}
	// Grant permission and move the executable
	_, err = utils.ExecShellCmd("sudo chmod a+rx %s && sudo mv %s /usr/local/bin", runscFilePath, runscFilePath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install runsc!\n") {
		return err
	}
	return nil
}

// Install Kubernetes components
func InstallKubernetes() error {
	utils.WaitPrintf("Adding the Kubernetes apt repository")
	_, err := utils.ExecShellCmd("sudo mkdir -p -m 755 /etc/apt/keyrings && curl -fsSL %sRelease.key | sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg && echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] %s /' | sudo tee /etc/apt/sources.list.d/kubernetes.list", configs.System.KubeRepoUrl, configs.System.KubeRepoUrl)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to add the Kubernetes apt repository!\n") {
		return err
	}
	// Install kubeadm, kubelet, kubectl via apt
	utils.WaitPrintf("Installing kubeadm, kubelet, kubectl")
	err = utils.InstallPackages("cri-tools ebtables ethtool kubernetes-cni kubeadm=%s kubelet=%s kubectl=%s", configs.System.KubeVersion, configs.System.KubeVersion, configs.System.KubeVersion)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install kubeadm, kubelet, kubectl!\n") {
		return err
	}
	return nil
}

func InstallKnativeCLI() error {
	// Clone Repo
	utils.WaitPrintf("Cloning knative CLI repo")
	knativeRepoPath, err := utils.CloneRepoToTmpDir(configs.Knative.KnativeCLIBranch, configs.Knative.KnativeCLIRepoUrl)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to clone knative CLI repo!\n") {
		return err
	}
	// Compile & Install
	utils.WaitPrintf("Compiling & Installing Knative CLI")
	_, err = utils.ExecShellCmd("cd %s && ./hack/build.sh -f && sudo mv kn /usr/local/bin", knativeRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install knative CLI!\n") {
		return err
	}
	return nil
}

// Enable IP forwading & br_netfilter
func ConfigSystemNetwork() error {
	// Necessary for containerd as container runtime but not docker
	// Set up required sysctl params, these persist across reboots.
	// ******** Enable IP forwading & br_netfilter ********
	utils.WaitPrintf("Enabling IP forwading & br_netfilter")
	_, err := utils.ExecShellCmd(
		"sudo modprobe br_netfilter" +
			" && sudo modprobe overlay" +
			" && sudo sysctl -w net.ipv4.ip_forward=1" +
			" && sudo sysctl -w net.ipv4.conf.all.forwarding=1" +
			" && sudo sysctl -w net.bridge.bridge-nf-call-iptables=1" +
			" && sudo sysctl -w net.bridge.bridge-nf-call-ip6tables=1")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to enable IP forwading & br_netfilter!\n") {
		return err
	}
	// Ensure Boot-Resistant
	utils.WaitPrintf("Ensuring Boot-Resistant")
	_, err = utils.ExecShellCmd(
		"echo 'br_netfilter' | sudo tee /etc/modules-load.d/netfilter.conf && echo 'overlay'" +
			" | sudo tee -a /etc/modules-load.d/netfilter.conf" +
			" && sudo sed -i 's/# *net.ipv4.ip_forward=1/net.ipv4.ip_forward=1/g' /etc/sysctl.conf" +
			" && sudo sed -i 's/net.ipv4.ip_forward=0/net.ipv4.ip_forward=1/g' /etc/sysctl.conf" +
			" && echo 'net.bridge.bridge-nf-call-iptables=1\nnet.bridge.bridge-nf-call-ip6tables=1\nnet.ipv4.conf.all.forwarding=1'" +
			" | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to ensure Boot-Resistant!\n") {
		return err
	}
	// `sudo sysctl --quiet --system`
	_, err = utils.ExecShellCmd("sudo sysctl --quiet --system")
	if !utils.CheckErrorWithMsg(err, "Failed to execute `sudo sysctl --quiet --system`!\n") {
		return err
	}
	return nil
}

func InstallStock() error {
	// Original Bash Scripts: scripts/install_stock.sh

	// Install required packages
	utils.WaitPrintf("Installing required packages for installing stock")
	err := utils.InstallPackages("btrfs-progs pkg-config libseccomp-dev unzip tar libseccomp2 socat util-linux apt-transport-https curl ipvsadm apparmor apparmor-utils")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install required packages for installing stock") {
		return err
	}

	// Install protoc
	utils.WaitPrintf("Installing protoc")
	if err := InstallProtoc(); err != nil {
		utils.ErrorPrintf("Failed to install protoc!\n")
		return err
	}

	// Install containerd
	utils.WaitPrintf("Installing containerd(ver %s)", configs.System.ContainerdVersion)
	if err := InstallContainerd(); err != nil {
		utils.ErrorPrintf("Failed to install containerd!\n")
		return err
	}

	// Install runc
	utils.WaitPrintf("Installing runc(ver %s)", configs.System.RuncVersion)
	if err := InstallRunc(); err != nil {
		utils.ErrorPrintf("Failed to install runc!\n")
		return err
	}

	// Install runsc
	utils.WaitPrintf("Installing runsc")
	if err := InstallRunsc(); err != nil {
		utils.ErrorPrintf("Failed to install runsc!\n")
		return err
	}

	// Verify containerd installation
	_, err = utils.ExecShellCmd("containerd --version")
	if !utils.CheckErrorWithMsg(err, "Failed to build containerd!\n") {
		return err
	}

	// Install Kubernetes
	utils.InfoPrintf("Installing Kubernetes\n")
	if err := InstallKubernetes(); err != nil {
		utils.ErrorPrintf("Failed to install Kubernetes!\n")
		return err
	}

	// Install knative CLI
	utils.InfoPrintf("Installing Knative CLI\n")
	if err := InstallKnativeCLI(); err != nil {
		utils.ErrorPrintf("Failed to install Knative CLI!\n")
		return err
	}

	// Configure system network
	utils.InfoPrintf("Configuring system network\n")
	if err := ConfigSystemNetwork(); err != nil {
		utils.ErrorPrintf("Failed to configure system network!\n")
		return err
	}

	// Success
	return nil
}
