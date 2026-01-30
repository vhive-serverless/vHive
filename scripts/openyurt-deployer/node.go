// MIT License
//
// Copyright (c) 2023 Jason Chua, Ruiqi Lai and vHive team
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

package main

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/sfreiberg/simplessh"
	"github.com/vhive-serverless/vHive/scripts/cluster"
	"github.com/vhive-serverless/vHive/scripts/configs"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

type NodeConfig struct {
	System  SystemEnvironmentStruct
	Kube    KubeConfigStruct
	Knative KnativeConfigStruct
	Yurt    YurtEnvironment
	Demo    DemoEnvironment
}

type Node struct {
	Name     string
	Client   *simplessh.Client
	NodeRole string
	Configs  *NodeConfig
}

func (node *Node) ExecShellCmd(cmd string, pars ...any) (string, error) {
	shellCmd := fmt.Sprintf(cmd, pars...)
	out, err := node.Client.Exec(shellCmd)
	if err != nil {
		utils.WarnPrintf("node: [%s] failed to exec: \n%s\nerror:%s\n", node.Name, shellCmd, out)
	}
	return strings.TrimSuffix(string(out), "\n"), err
}

func (node *Node) OnlyExecByMaster() {
	if node.NodeRole != "master" {
		utils.FatalPrintf("This function can only be executed by master node!\n")
	}
}

func (node *Node) OnlyExecByWorker() {
	if node.NodeRole == "master" {
		utils.FatalPrintf("This function can only be executed by worker node!\n")
	}
}

func (node *Node) SetMasterAsCloud(asCloud bool) {
	node.OnlyExecByMaster()
	node.Configs.Yurt.MasterAsCloud = asCloud
}

// System related functions

// Detect current architecture
func (node *Node) DetectArch() {
	utils.WaitPrintf("Detetcting current arch")
	out, err := node.ExecShellCmd("dpkg --print-architecture")
	utils.CheckErrorWithMsg(err, "Failed to get current arch!\n")
	node.Configs.System.CurrentArch = out
	switch node.Configs.System.CurrentArch {
	default:
		utils.InfoPrintf("Detected Arch: %s for node: %s\n", node.Configs.System.CurrentArch, node.Name)
	}
}

// Detect current operating system
func (node *Node) DetectOS() {
	switch node.Configs.System.CurrentOS {
	case "windows":
		utils.FatalPrintf("Unsupported OS: %s\n", node.Configs.System.CurrentOS)
	default:
		var err error
		node.Configs.System.CurrentOS, err = node.ExecShellCmd("sed -n 's/^NAME=\"\\(.*\\)\"/\\1/p' < /etc/os-release | head -1 | tr '[:upper:]' '[:lower:]'")
		utils.InfoPrintf("Detected OS: %s\n", node.Configs.System.CurrentOS)
		utils.CheckErrorWithMsg(err, "Failed to get Linux distribution info!\n")
		switch node.Configs.System.CurrentOS {
		case "ubuntu":
		default:
			utils.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
		}
		utils.InfoPrintf("Detected OS: %s for node: %s\n",
			strings.TrimSuffix(string(node.Configs.System.CurrentOS), "\n"),
			node.Name)
	}
}

// Get current directory
func (node *Node) GetCurrentDir() {
	var err error
	node.Configs.System.CurrentDir, err = node.ExecShellCmd("pwd")
	utils.CheckErrorWithMsg(err, "Failed to get get current directory!\n")
}

// Get current home directory
func (node *Node) GetUserHomeDir() {
	var err error
	node.Configs.System.UserHomeDir, err = node.ExecShellCmd("echo $HOME")
	utils.CheckErrorWithMsg(err, "Failed to get current home directory!\n")
}

// Get current node's hostname
func (node *Node) GetNodeHostName() {
	var err error
	node.Configs.System.NodeHostName, err = node.ExecShellCmd("echo $HOSTNAME")
	utils.CheckErrorWithMsg(err, "Failed to get current node hostname!\n")
}

// Create temporary directory
func (node *Node) CreateTmpDir() {
	var err error
	utils.InfoPrintf("Creating temporary directory")
	tmpDir := "~/yurt_tmp"
	_, err = node.ExecShellCmd("mkdir -p %s", tmpDir)
	node.Configs.System.TmpDir = tmpDir
	utils.CheckErrorWithMsg(err, "Failed to create temporary directory!\n")
}

// Clean up temporary directory
func (node *Node) CleanUpTmpDir() {
	utils.InfoPrintf("Cleaning up temporary directory")
	_, err := node.ExecShellCmd("rm -rf %s/*", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to create temporary directory!\n")
}

// Extract arhive file to specific directory(currently support .tar.gz file only)
func (node *Node) ExtractToDir(filePath string, dirPath string, privileged bool) error {
	var err error
	if privileged {
		_, err = node.ExecShellCmd("sudo tar -xzvf %s -C %s", filePath, dirPath)
	} else {
		_, err = node.ExecShellCmd("tar -xzvf %s -C %s", filePath, dirPath)
	}
	return err
}

// Append directory to PATH variable for bash & zsh
func (node *Node) AppendDirToPath(pathTemplate string, pars ...any) error {
	appendedPath := fmt.Sprintf(pathTemplate, pars...)

	// For bash
	_, err := node.ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.bashrc", appendedPath, node.Configs.System.UserHomeDir)
	if err != nil {
		return err
	}
	// For zsh
	_, err = node.LookPath("zsh")
	if err != nil {
		_, err = node.ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.zshrc", appendedPath, node.Configs.System.UserHomeDir)
	}
	return err
}

// Turn off unattended-upgrades
func (node *Node) TurnOffAutomaticUpgrade() (string, error) {
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		_, err := node.ExecShellCmd("stat /etc/apt/apt.conf.d/20auto-upgrades")
		if err == nil {
			return node.ExecShellCmd("sudo sed -i 's/\"1\"/\"0\"/g' /etc/apt/apt.conf.d/20auto-upgrades")
		}
		return "", nil
	default:
		return "", nil
	}
}

// Install packages on various OS
func (node *Node) InstallPackages(packagesTemplate string, pars ...any) error {
	packages := fmt.Sprintf(packagesTemplate, pars...)
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		_, err := node.ExecShellCmd(`sudo apt-get -qq update && \
		 sudo apt-get -qq install -y --allow-downgrades --allow-change-held-packages %s`, packages)
		return err
	case "centos":
		_, err := node.ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	case "rocky linux":
		_, err := node.ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	default:
		utils.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
		return &utils.ShellError{Msg: "Unsupported Linux distribution", ExitCode: 1}
	}
}

// Download file to temporary directory (absolute path of downloaded file will be the first return value if successful)
func (node *Node) DownloadToTmpDir(urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	fileName := path.Base(url)
	filePath := node.Configs.System.TmpDir + "/" + fileName
	_, err := node.ExecShellCmd("curl -sSL --output %s %s", filePath, url)
	return filePath, err
}

func (node *Node) LookPath(path string) (string, error) {
	return node.ExecShellCmd("command -v %s", path)
}

// Check system environment
func (node *Node) CheckSystemEnvironment() {
	// Check system environment
	utils.InfoPrintf("Checking system environment...\n")
	var err error

	// Check Golang
	_, err = node.LookPath("go")
	if err != nil {
		utils.InfoPrintf("Golang not found! Golang(version %s) will be automatically installed!\n",
			node.Configs.System.GoVersion)
	} else {
		utils.InfoPrintf("Golang found!\n")
		node.Configs.System.GoInstalled = true
	}

	// Check Containerd
	_, err = node.LookPath("containerd")
	if err != nil {
		utils.InfoPrintf("Containerd not found! containerd(version %s) will be automatically installed!\n",
			node.Configs.System.ContainerdVersion)
	} else {
		utils.InfoPrintf("Containerd found!\n")
		node.Configs.System.ContainerdInstalled = true
	}

	// Check runc
	_, err = node.LookPath("runc")
	if err != nil {
		utils.InfoPrintf("runc not found! runc(version %s) will be automatically installed!\n",
			node.Configs.System.RuncVersion)
	} else {
		utils.InfoPrintf("runc found!\n")
		node.Configs.System.RuncInstalled = true
	}

	// Check CNI plugins
	_, err = node.ExecShellCmd("stat /opt/cni/bin")
	if err != nil {
		utils.InfoPrintf("CNI plugins not found! CNI plugins(version %s) will be automatically installed!\n",
			node.Configs.System.CniPluginsVersion)
	} else {
		utils.InfoPrintf("CNI plugins found!\n")
		node.Configs.System.CniPluginsInstalled = true
	}

	// Add OS-specific dependencies to installation lists
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		node.Configs.System.Dependencies = "git wget curl build-essential apt-transport-https ca-certificates"
	case "rocky linux":
		node.Configs.System.Dependencies = ""
	case "centos":
		node.Configs.System.Dependencies = ""
	default:
		utils.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
	}

	utils.InfoPrintf("Finish checking system environment!\n")
}

func (node *Node) ReadSystemInfo() {
	node.DetectOS()
	node.DetectArch()
	node.GetCurrentDir()
	node.GetUserHomeDir()
	node.GetNodeHostName()
	node.CheckSystemEnvironment()
}

// Initialize system environment
func (node *Node) SystemInit() {
	utils.InfoPrintf("Start init system environment for node:%s\n", node.Name)
	// Initialize

	var err error

	// node.ReadSystemInfo() // technically, this is not necessary
	node.CreateTmpDir()
	// defer node.CleanUpTmpDir()

	// Turn off unattended-upgrades on ubuntu
	utils.InfoPrintf("Turning off automatic upgrade")
	_, err = node.TurnOffAutomaticUpgrade()
	utils.CheckErrorWithMsg(err, "Failed to turn off automatic upgrade!\n")

	// Disable swap
	utils.InfoPrintf("Disabling swap")
	_, err = node.ExecShellCmd("sudo swapoff -a && sudo cp /etc/fstab /etc/fstab.old") // Turn off Swap && Backup fstab file
	utils.CheckErrorWithMsg(err, "Failed to disable swap!\n")

	utils.InfoPrintf("Modifying fstab")
	// Modify fstab to disable swap permanently
	_, err = node.ExecShellCmd("sudo sed -i 's/#\\s*\\(.*swap.*\\)/\\1/g' /etc/fstab && sudo sed -i 's/.*swap.*/# &/g' /etc/fstab")
	utils.CheckErrorWithMsg(err, "Failed to dodify fstab!\n")

	// Install dependencies
	utils.InfoPrintf("Installing dependencies")
	err = node.InstallPackages("%s", node.Configs.System.Dependencies)
	utils.CheckErrorWithMsg(err, "Failed to install dependencies!\n")

	// Install Golang
	if !node.Configs.System.GoInstalled {
		// Download & Extract Golang
		utils.InfoPrintf("Downloading Golang(ver %s)", node.Configs.System.GoVersion)
		filePathName, err := node.DownloadToTmpDir(node.Configs.System.GoDownloadUrlTemplate,
			node.Configs.System.GoVersion,
			node.Configs.System.CurrentArch)
		utils.CheckErrorWithMsg(err, "Failed to download Golang(ver %s)!\n", node.Configs.System.GoVersion)
		utils.InfoPrintf("Extracting Golang")
		_, err = node.ExecShellCmd("sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf %s", filePathName)
		utils.CheckErrorWithMsg(err, "Failed to extract Golang!\n")

		// For bash
		_, err = node.ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.bashrc", node.Configs.System.UserHomeDir)
		utils.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		// For zsh
		_, err = node.LookPath("zsh")
		if err != nil {
			_, err = node.ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.zshrc", node.Configs.System.UserHomeDir)
			utils.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		}
	}

	// Install containerd
	if !node.Configs.System.ContainerdInstalled {
		// Download containerd
		utils.InfoPrintf("Downloading containerd(ver %s)", node.Configs.System.ContainerdVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.ContainerdDownloadUrlTemplate,
			node.Configs.System.ContainerdVersion,
			node.Configs.System.ContainerdVersion,
			node.Configs.System.CurrentArch)
		utils.CheckErrorWithMsg(err, "Failed to Download containerd(ver %s)\n", node.Configs.System.ContainerdVersion)
		// Extract containerd
		utils.InfoPrintf("Extracting containerd")
		_, err = node.ExecShellCmd("sudo tar Cxzvf /usr/local %s", filePathName)
		utils.CheckErrorWithMsg(err, "Failed to extract containerd!\n")
		// Start containerd via systemd
		utils.InfoPrintf("Downloading systemd profile for containerd")
		filePathName, err = node.DownloadToTmpDir("%s", node.Configs.System.ContainerdSystemdProfileDownloadUrl)
		utils.CheckErrorWithMsg(err, "Failed to download systemd profile for containerd!\n")
		utils.InfoPrintf("Starting containerd via systemd")
		_, err = node.ExecShellCmd("sudo cp %s /lib/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now containerd", filePathName)
		utils.CheckErrorWithMsg(err, "Failed to start containerd via systemd!\n")
	}

	// Install runc
	if !node.Configs.System.RuncInstalled {
		// Download runc
		utils.InfoPrintf("Downloading runc(ver %s)", node.Configs.System.RuncVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.RuncDownloadUrlTemplate,
			node.Configs.System.RuncVersion,
			node.Configs.System.CurrentArch)
		utils.CheckErrorWithMsg(err, "Failed to download runc(ver %s)!\n", node.Configs.System.RuncVersion)
		// Install runc
		utils.InfoPrintf("Installing runc")
		_, err = node.ExecShellCmd("sudo install -m 755 %s /usr/local/sbin/runc", filePathName)
		utils.CheckErrorWithMsg(err, "Failed to install runc!\n")
	}

	// Install CNI plugins
	if !node.Configs.System.CniPluginsInstalled {
		utils.InfoPrintf("Downloading CNI plugins(ver %s)", node.Configs.System.CniPluginsVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.CniPluginsDownloadUrlTemplate,
			node.Configs.System.CniPluginsVersion,
			node.Configs.System.CurrentArch,
			node.Configs.System.CniPluginsVersion)
		utils.CheckErrorWithMsg(err, "Failed to download CNI plugins(ver %s)!\n", node.Configs.System.CniPluginsVersion)
		utils.InfoPrintf("Extracting CNI plugins")
		_, err = node.ExecShellCmd("sudo mkdir -p /opt/cni/bin && sudo tar Cxzvf /opt/cni/bin %s", filePathName)
		utils.CheckErrorWithMsg(err, "Failed to extract CNI plugins!\n")
	}

	// Configure the systemd cgroup driver
	utils.InfoPrintf("Configuring the systemd cgroup driver")
	_, err = node.ExecShellCmd(`containerd config default > %s &&
			sudo mkdir -p /etc/containerd && 
			sudo cp %s /etc/containerd/config.toml && 
			sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml && 
			sudo systemctl restart containerd`,
		node.Configs.System.TmpDir+"/config.toml",
		node.Configs.System.TmpDir+"/config.toml")
	utils.CheckErrorWithMsg(err, "Failed to configure the systemd cgroup driver!\n")

	// Enable IP forwading & br_netfilter
	utils.InfoPrintf("Enabling IP forwading & br_netfilter")
	_, err = node.ExecShellCmd(`sudo modprobe br_netfilter && sudo modprobe overlay && 
		sudo sysctl -w net.ipv4.ip_forward=1 && 
		sudo sysctl -w net.ipv4.conf.all.forwarding=1 && 
		sudo sysctl -w net.bridge.bridge-nf-call-iptables=1 && 
		sudo sysctl -w net.bridge.bridge-nf-call-ip6tables=1`)
	utils.CheckErrorWithMsg(err, "Failed to enable IP forwading & br_netfilter!\n")
	// Ensure Boot-Resistant
	utils.InfoPrintf("Ensuring Boot-Resistant")
	_, err = node.ExecShellCmd(`echo 'br_netfilter' | 
		sudo tee /etc/modules-load.d/netfilter.conf && 
		echo 'overlay' | sudo tee -a /etc/modules-load.d/netfilter.conf && 
		sudo sed -i 's/# *net.ipv4.ip_forward=1/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && 
		sudo sed -i 's/net.ipv4.ip_forward=0/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && 
		echo 'net.bridge.bridge-nf-call-iptables=1\nnet.bridge.bridge-nf-call-ip6tables=1\nnet.ipv4.conf.all.forwarding=1' | 
		sudo tee /etc/sysctl.d/99-kubernetes-cri.conf`)
	utils.CheckErrorWithMsg(err, "Failed to ensure Boot-Resistant!\n")

	// Install kubeadm, kubelet, kubectl
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		// Download Google Cloud public signing key and Add the Kubernetes apt repository
		utils.InfoPrintf("Adding the Kubernetes apt repository")
		_, err = node.ExecShellCmd(`sudo mkdir -p -m 755 /etc/apt/keyrings && curl -fsSL %sRelease.key |
			sudo gpg --batch --yes --dearmor -o /etc/apt/keyrings/kubernetes-apt-keyring.gpg &&
			echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-apt-keyring.gpg] %s /' | 
			sudo tee /etc/apt/sources.list.d/kubernetes.list`, node.Configs.System.KubeRepoUrl, node.Configs.System.KubeRepoUrl)
		utils.CheckErrorWithMsg(err, "Failed to add the Kubernetes apt repository!\n")
		// Install kubeadm, kubelet, kubectl via apt
		utils.InfoPrintf("Installing kubeadm, kubelet, kubectl")
		err = node.InstallPackages("kubeadm=%s kubelet=%s kubectl=%s",
			node.Configs.System.KubeVersion,
			node.Configs.System.KubeVersion,
			node.Configs.System.KubeVersion)
		utils.CheckErrorWithMsg(err, "Failed to install kubeadm, kubelet, kubectl!\n")
		// Lock kubeadm, kubelet, kubectl version
		utils.InfoPrintf("Locking kubeadm, kubelet, kubectl version")
		_, err = node.ExecShellCmd("sudo apt-mark hold kubelet kubeadm kubectl")
		utils.CheckErrorWithMsg(err, "Failed to lock kubeadm, kubelet, kubectl version!\n")
	default:
		utils.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
	}

	// Install yq for yaml parsing of template
	utils.InfoPrintf("Downloading yq for yaml parsing of template")
	yqUrl := fmt.Sprintf(node.Configs.System.YqDownloadUrl, node.Configs.System.CurrentArch)
	_, err = node.ExecShellCmd(`sudo wget %s -O /usr/bin/yq && sudo chmod +x /usr/bin/yq`, yqUrl)
	utils.CheckErrorWithMsg(err, "Failed to add yq!\n")
}

// Kubernetes related functions
func (node *Node) KubeMasterInit() (string, string, string, string) {

	// Initialize
	var err error
	node.CreateTmpDir()

	// Pre-pull Image
	utils.WaitPrintf("Pre-Pulling required images")
	shellCmd := fmt.Sprintf("sudo kubeadm config images pull --kubernetes-version %s ", node.Configs.Kube.K8sVersion)
	if len(node.Configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", node.Configs.Kube.AlternativeImageRepo)
	}
	_, err = node.ExecShellCmd("%s", shellCmd)
	utils.CheckErrorWithMsg(err, "Failed to pre-pull required images!\n")

	// Deploy Kubernetes
	utils.WaitPrintf("Deploying Kubernetes(version %s)", node.Configs.Kube.K8sVersion)
	shellCmd = fmt.Sprintf("sudo kubeadm init --kubernetes-version %s --pod-network-cidr=\"%s\" ",
		node.Configs.Kube.K8sVersion,
		node.Configs.Kube.PodNetworkCidr)
	if len(node.Configs.Kube.AlternativeImageRepo) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--image-repository %s ", node.Configs.Kube.AlternativeImageRepo)
	}
	if len(node.Configs.Kube.ApiserverAdvertiseAddress) > 0 {
		shellCmd = fmt.Sprintf(shellCmd+"--apiserver-advertise-address=%s ", node.Configs.Kube.ApiserverAdvertiseAddress)
	}
	shellCmd = fmt.Sprintf(shellCmd+"| tee %s/masterNodeInfo", node.Configs.System.TmpDir)
	_, err = node.ExecShellCmd("%s", shellCmd)
	utils.CheckErrorWithMsg(err, "Failed to deploy Kubernetes(version %s)!\n", node.Configs.Kube.K8sVersion)

	// Make kubectl work for non-root user
	utils.WaitPrintf("Making kubectl work for non-root user")
	_, err = node.ExecShellCmd(`mkdir -p %s/.kube && 
		sudo cp -i /etc/kubernetes/admin.conf %s/.kube/config && sudo chown $(id -u):$(id -g) %s/.kube/config`,
		node.Configs.System.UserHomeDir,
		node.Configs.System.UserHomeDir,
		node.Configs.System.UserHomeDir)
	utils.CheckErrorWithMsg(err, "Failed to make kubectl work for non-root user!\n")

	// Install Calico network add-on
	configs.Kube.CalicoVersion = node.Configs.Kube.CalicoVersion // TODO: @jchua99, pls fix
	cluster.InstallCalico(false)

	// Extract master node information from logs
	utils.WaitPrintf("Extracting master node information from logs")
	shellOut, err := node.ExecShellCmd("sed -n '/.*kubeadm join.*/p' < %s/masterNodeInfo |"+
		"sed -n 's/.*join \\(.*\\):\\(\\S*\\) --token \\(\\S*\\).*/\\1 \\2 \\3/p'", node.Configs.System.TmpDir)
	utils.InfoPrintf("shellOut 2: %s\n", shellOut) //DEBUG
	utils.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n")
	splittedOut := strings.Split(shellOut, " ")
	utils.InfoPrintf("spiltOut 3: %s\n", splittedOut) //DEBUG
	node.Configs.Kube.ApiserverAdvertiseAddress = splittedOut[0]
	node.Configs.Kube.ApiserverPort = splittedOut[1]
	node.Configs.Kube.ApiserverToken = splittedOut[2]
	shellOut, err = node.ExecShellCmd("sed -n '/.*sha256:.*/p' < %s/masterNodeInfo | sed -n 's/.*\\(sha256:\\S*\\).*/\\1/p'", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to extract master node information from logs!\n")
	node.Configs.Kube.ApiserverTokenHash = shellOut

	shellData := fmt.Sprintf("echo '%s\n%s\n%s\n%s' > %s/masterNodeValues",
		node.Configs.Kube.ApiserverAdvertiseAddress,
		node.Configs.Kube.ApiserverPort,
		node.Configs.Kube.ApiserverToken,
		node.Configs.Kube.ApiserverTokenHash,
		node.Configs.System.TmpDir)
	_, err = node.ExecShellCmd("%s", shellData)
	utils.CheckErrorWithMsg(err, "Failed to write master node information to file!\n")

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
	utils.CheckErrorWithMsg(err, "Failed to clean kube cluster!\n")

}

// Join worker node to Kubernetes cluster
func (node *Node) KubeWorkerJoin(apiServerAddr string, apiServerPort string, apiServerToken string, apiServerTokenHash string) {

	// Initialize
	var err error

	// Join Kubernetes cluster
	utils.WaitPrintf("Joining Kubernetes cluster")
	_, err = node.ExecShellCmd("sudo kubeadm join %s:%s --token %s --discovery-token-ca-cert-hash %s",
		apiServerAddr,
		apiServerPort,
		apiServerToken,
		apiServerTokenHash)
	utils.CheckErrorWithMsg(err, "Failed to join Kubernetes cluster!\n")
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

// Knative related functions
// Install Knative Serving
func (node *Node) InstallKnativeServing() {
	node.OnlyExecByMaster()
	var err error

	node.CreateTmpDir()

	// Install and configure MetalLB
	utils.WaitPrintf("Installing and configuring MetalLB")
	_, err = node.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml | 
		sed -e "s/strictARP: false/strictARP: true/" | 
		kubectl apply -f - -n kube-system`)
	utils.CheckErrorWithMsg(err, "Failed to apply config map MetalLB!")
	_, err = node.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", node.Configs.Knative.MetalLBVersion)
	utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!")
	_, err = node.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=600s --for=condition=Available")
	utils.CheckErrorWithMsg(err, "Failed to wait for deployment MetalLB!")
	for _, value := range node.Configs.Knative.MetalLBConfigURLArray {
		_, err = node.ExecShellCmd("kubectl apply -f %s", value)
		utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB array list!")
	}
	utils.SuccessPrintf("\n")

	// Install istio
	// Download istio
	utils.WaitPrintf("Downloading istio")
	istioFilePath, err := node.DownloadToTmpDir("%s", node.GetIstioDownloadUrl())
	utils.CheckErrorWithMsg(err, "Failed to download istio!")
	// Extract istio
	utils.WaitPrintf("Extracting istio")
	err = node.ExtractToDir(istioFilePath, "/usr/local", true)
	utils.CheckErrorWithMsg(err, "Failed to extract istio!")
	// Update PATH
	err = node.AppendDirToPath("/usr/local/istio-%s/bin", node.Configs.Knative.IstioVersion)
	utils.CheckErrorWithMsg(err, "Failed to update PATH!")
	// Deploy istio operator
	utils.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := node.DownloadToTmpDir("%s", node.Configs.Knative.IstioOperatorConfigUrl)
	utils.CheckErrorWithMsg(err, "Failed to download istio operator config!")
	_, err = node.ExecShellCmd("sudo /usr/local/istio-%s/bin/istioctl install -y -f %s",
		node.Configs.Knative.IstioVersion,
		operatorConfigPath)
	utils.CheckErrorWithMsg(err, "Failed to deploy istio operator!")

	// Install Knative Serving component
	utils.WaitPrintf("Installing Knative Serving component")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml",
		node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Serving component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml",
		node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Serving component!")

	// Install local cluster registry
	// utils.WaitPrintf("Installing local cluster registry")
	// _, err = node.ExecShellCmd("kubectl create namespace registry")
	// utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	// configFilePath, err := node.DownloadToTmpDir("%s", node.Configs.Knative.LocalRegistryVolumeConfigUrl)
	// utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	// _, err = node.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -",
	// 	node.Configs.Knative.LocalRegistryRepoVolumeSize,
	// 	configFilePath)
	// utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")
	// _, err = node.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s",
	// 	node.Configs.Knative.LocalRegistryDockerRegistryConfigUrl,
	// 	node.Configs.Knative.LocalRegistryHostUpdateConfigUrl)
	// utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!")

	// Configure Magic DNS
	utils.WaitPrintf("Configuring Magic DNS")
	_, err = node.ExecShellCmd("kubectl apply -f %s", node.Configs.Knative.MagicDNSConfigUrl)
	utils.CheckErrorWithMsg(err, "Failed to configure Magic DNS!")

	// Install networking layer
	utils.WaitPrintf("Installing networking layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v%s/net-istio.yaml",
		node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install networking layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-serving")
	utils.CheckErrorWithMsg(err, "Verification Failed!")

	// enable node selector
	utils.WaitPrintf("Enable node selector in knative serving")
	_, err = node.ExecShellCmd(`kubectl patch cm config-features -n knative-serving \
  --type merge \
  -p '{"data":{"kubernetes.podspec-nodeselector":"enabled"}}'
`)
	utils.CheckErrorWithMsg(err, "Failed to enable node selector in knative serving")
	// node.enableNodeSelect()
}

// Install Knative Eventing
func (node *Node) InstallKnativeEventing() {
	// Install Knative Eventing component
	utils.WaitPrintf("Installing Knative Eventing component")
	_, err := node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-crds.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-core.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl get pods -n knative-eventing")
	utils.CheckErrorWithMsg(err, "Verification Failed!")

	// Install a default Channel (messaging) layer
	utils.WaitPrintf("Installing a default Channel (messaging) layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/in-memory-channel.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install a default Channel (messaging) layer!")

	// Install a Broker layer
	utils.WaitPrintf("Installing a Broker layer")
	_, err = node.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/mt-channel-broker.yaml", node.Configs.Knative.KnativeVersion)
	utils.CheckErrorWithMsg(err, "Failed to install a Broker layer!")

	// Logs for verification
	_, err = node.ExecShellCmd("kubectl --namespace istio-system get service istio-ingressgateway")
	utils.CheckErrorWithMsg(err, "Verification Failed!")
}

// get istio download URL
func (node *Node) GetIstioDownloadUrl() string {
	knative := node.Configs.Knative
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, node.Configs.System.CurrentArch)
}

// Open yurt Related functions
func (node *Node) CheckYurtMasterEnvironment() {
	node.OnlyExecByMaster()
	// Check environment
	var err error
	utils.InfoPrintf("Checking system environment...\n")

	// Check Helm
	_, err = node.LookPath("helm")
	if err != nil {
		utils.WarnPrintf("Helm not found! Helm will be automatically installed!\n")
	} else {
		utils.SuccessPrintf("Helm found!\n")
		node.Configs.Yurt.HelmInstalled = true
	}

	// Check Kustomize
	_, err = node.LookPath("kustomize")
	if err != nil {
		utils.WarnPrintf("Kustomize not found! Kustomize will be automatically installed!\n")
	} else {
		utils.SuccessPrintf("Kustomize found!\n")
		node.Configs.Yurt.KustomizeInstalled = true
	}

	// Add OS-specific dependencies to installation lists
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		node.Configs.Yurt.Dependencies = "curl apt-transport-https ca-certificates build-essential git"
	case "rocky linux":
		node.Configs.Yurt.Dependencies = ""
	case "centos":
		node.Configs.Yurt.Dependencies = ""
	default:
		utils.FatalPrintf("Unsupported OS: %s\n", node.Configs.System.CurrentOS)
	}

	utils.SuccessPrintf("Finished checking system environment!\n")
}

// Initialize Openyurt on master node
func (node *Node) YurtMasterInit() {
	node.OnlyExecByMaster()
	// Initialize
	var err error
	node.CheckYurtMasterEnvironment()
	node.CreateTmpDir()
	// defer node.CleanUpTmpDir()

	// Install dependencies
	utils.WaitPrintf("Installing dependencies")
	err = node.InstallPackages("%s", node.Configs.Yurt.Dependencies)
	utils.CheckErrorWithMsg(err, "Failed to install dependencies!\n")

	// Treat master as cloud node
	if node.Configs.Yurt.MasterAsCloud {
		utils.WarnPrintf("Master node WILL also be treated as a cloud node!\n")
		node.ExecShellCmd("kubectl taint nodes --all node-role.kubernetes.io/master:NoSchedule-")
		node.ExecShellCmd("kubectl taint nodes --all node-role.kubernetes.io/control-plane-")
	}

	// Install helm
	if !node.Configs.Yurt.HelmInstalled {
		switch node.Configs.System.CurrentOS {
		case "ubuntu":
			// Download public signing key && Add the Helm apt repository
			utils.WaitPrintf("Downloading public signing key && Add the Helm apt repository")
			// Download public signing key
			filePathName, err := node.DownloadToTmpDir("%s", node.Configs.Yurt.HelmPublicSigningKeyDownloadUrl)
			utils.CheckErrorWithMsg(err, "Failed to download public signing key && add the Helm apt repository!\n")
			_, err = node.ExecShellCmd("sudo mkdir -p /usr/share/keyrings && cat %s | gpg --dearmor | sudo tee /usr/share/keyrings/helm.gpg > /dev/null", filePathName)
			utils.CheckErrorWithMsg(err, "Failed to download public signing key && add the Helm apt repository!\n")
			// Add the Helm apt repository
			_, err = node.ExecShellCmd(`echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/helm.gpg] https://baltocdn.com/helm/stable/debian/ all main" | 
				sudo tee /etc/apt/sources.list.d/helm-stable-debian.list`)
			utils.CheckErrorWithMsg(err, "Failed to download public signing key && add the Helm apt repository!\n")
			// Install helm
			utils.WaitPrintf("Installing Helm")
			err = node.InstallPackages("%s", "helm")
			utils.CheckErrorWithMsg(err, "Failed to install helm!\n")
		default:
			utils.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
		}
	}

	// Install kustomize
	if !node.Configs.Yurt.KustomizeInstalled {
		// Download kustomize helper script
		utils.WaitPrintf("Downloading kustomize")
		filePathName, err := node.DownloadToTmpDir("%s", node.Configs.Yurt.KustomizeScriptDownloadUrl)
		utils.CheckErrorWithMsg(err, "Failed to download kustomize!\n")
		// Download kustomize
		_, err = node.ExecShellCmd("chmod u+x %s && %s %s", filePathName, filePathName, node.Configs.System.TmpDir)
		utils.CheckErrorWithMsg(err, "Failed to download kustomize!\n")
		// Install kustomize
		utils.WaitPrintf("Installing kustomize")
		_, err = node.ExecShellCmd("sudo cp %s /usr/local/bin", node.Configs.System.TmpDir+"/kustomize")
		utils.CheckErrorWithMsg(err, "Failed to Install kustomize!\n")
	}

	// Add OpenYurt repo with helm
	utils.WaitPrintf("Adding OpenYurt repo(version %s) with helm", node.Configs.Yurt.YurtVersion)
	_, err = node.ExecShellCmd("git clone --quiet https://github.com/openyurtio/openyurt-helm.git %s/openyurt-helm && pushd %s/openyurt-helm && git checkout openyurt-%s && popd",
		node.Configs.System.TmpDir,
		node.Configs.System.TmpDir,
		node.Configs.Yurt.YurtVersion)
	utils.CheckErrorWithMsg(err, "Failed to add OpenYurt repo with helm!\n")

	// Deploy yurt-app-manager
	utils.WaitPrintf("Deploying yurt-app-manager")
	_, err = node.ExecShellCmd("helm install yurt-app-manager -n kube-system %s/openyurt-helm/charts/yurt-app-manager", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to deploy yurt-app-manager!\n")

	// Wait for yurt-app-manager to be ready
	utils.WaitPrintf("Waiting for yurt-app-manager to be ready")
	waitCount := 1
	for {
		yurtAppManagerStatus, err := node.ExecShellCmd(`kubectl get pod -n kube-system | grep yurt-app-manager | sed -n "s/\s*\(\S*\)\s*\(\S*\)\s*\(\S*\).*/\2 \3/p"`)
		utils.CheckErrorWithMsg(err, "Failed to wait for yurt-app-manager to be ready!\n")
		if yurtAppManagerStatus == "1/1 Running" {
			utils.SuccessPrintf("\n")
			break
		} else {
			utils.WarnPrintf("Waiting for yurt-app-manager to be ready [%ds]\n", waitCount)
			waitCount += 1
			time.Sleep(time.Second)
		}
	}

	// Deploy yurt-controller-manager
	utils.WaitPrintf("Deploying yurt-controller-manager")
	_, err = node.ExecShellCmd("helm install openyurt %s/openyurt-helm/charts/openyurt -n kube-system", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to deploy yurt-controller-manager!\n")

	// Setup raven-controller-manager Component
	// Clone repository
	utils.WaitPrintf("Cloning repo: raven-controller-manager")
	_, err = node.ExecShellCmd("git clone --quiet https://github.com/openyurtio/raven-controller-manager.git %s/raven-controller-manager", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to clone repo: raven-controller-manager!\n")
	// Deploy raven-controller-manager
	utils.WaitPrintf("Deploying raven-controller-manager")
	_, err = node.ExecShellCmd(`pushd %s/raven-controller-manager && 
		git checkout v0.3.0 && make generate-deploy-yaml && 
		kubectl apply -f _output/yamls/raven-controller-manager.yaml && 
		popd`,
		node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to deploy raven-controller-manager!\n")

	// Setup raven-agent Component
	// Clone repository
	utils.WaitPrintf("Cloning repo: raven-agent")
	_, err = node.ExecShellCmd("git clone --quiet https://github.com/openyurtio/raven.git %s/raven-agent", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to clone repo: raven-agent!\n")
	// Deploy raven-agent
	utils.WaitPrintf("Deploying raven-agent")
	_, err = node.ExecShellCmd("pushd %s/raven-agent && git checkout v0.3.0 && FORWARD_NODE_IP=true make deploy && popd", node.Configs.System.TmpDir)
	utils.CheckErrorWithMsg(err, "Failed to deploy raven-agent!\n")
}

// Expand Openyurt to worker node
func (node *Node) YurtMasterExpand(worker *Node) {
	node.OnlyExecByMaster()
	// Initialize
	var err error
	var workerAsEdge string

	// Label worker node as cloud/edge
	utils.WaitPrintf("Labeling worker node: %s", worker.Configs.System.NodeHostName)
	if worker.NodeRole == "edge" {
		workerAsEdge = "true"
	} else if worker.NodeRole == "cloud" {
		workerAsEdge = "false"
	} else {
		utils.FatalPrintf("worker's role must be edge or cloud, but this node's role is %s", worker.NodeRole)
	}
	_, err = node.ExecShellCmd("kubectl label node %s openyurt.io/is-edge-worker=%s", worker.Configs.System.NodeHostName, workerAsEdge)
	utils.CheckErrorWithMsg(err, "Failed to label worker node!\n")

	// Activate the node autonomous mode
	utils.WaitPrintf("Activating the node autonomous mode")
	_, err = node.ExecShellCmd("kubectl annotate node %s node.beta.openyurt.io/autonomy=true", worker.Configs.System.NodeHostName)
	utils.CheckErrorWithMsg(err, "Failed to activate the node autonomous mode!\n")

	// Wait for worker node to be Ready
	utils.WaitPrintf("Waiting for worker node to be ready")
	waitCount := 1
	for {
		workerNodeStatus, err := node.ExecShellCmd(`kubectl get nodes | sed -n "/.*%s.*/p" | 
			sed -n "s/\s*\(\S*\)\s*\(\S*\).*/\2/p"`,
			worker.Configs.System.NodeHostName)
		utils.CheckErrorWithMsg(err, "Failed to wait for worker node to be ready!\n")
		if workerNodeStatus == "Ready" {
			utils.SuccessPrintf("\n")
			break
		} else {
			utils.WarnPrintf("Waiting for worker node to be ready [%ds]\n", waitCount)
			waitCount += 1
			time.Sleep(time.Second)
		}
	}
}

// Join existing Kubernetes worker node to Openyurt cluster
func (node *Node) YurtWorkerJoin(addr string, port string, token string) {

	// Initialize
	var err error

	// Get yurt template from github
	yurtTempFilePath, _ := node.DownloadToTmpDir("https://raw.githubusercontent.com/vhive-serverless/vHive/openyurt/scripts/openyurt-deployer/configs/yurtTemplate.yaml")
	// Set up Yurthub
	utils.WaitPrintf("Setting up Yurthub")
	_, err = node.ExecShellCmd(
		"cat '%s' | sed -e 's|__kubernetes_master_address__|%s:%s|' -e 's|__bootstrap_token__|%s|' | sudo tee /etc/kubernetes/manifests/yurthub-ack.yaml",
		yurtTempFilePath, addr, port, token)
	utils.CheckErrorWithMsg(err, "Failed to set up Yurthub!\n")

	// Get kubelet template from github
	kubletTempFilePath, _ := node.DownloadToTmpDir("https://raw.githubusercontent.com/vhive-serverless/vHive/openyurt/scripts/openyurt-deployer/configs/kubeTemplate.yaml")
	// Configure Kubelet
	utils.WaitPrintf("Configuring kubelet")
	node.ExecShellCmd("sudo mkdir -p /var/lib/openyurt && cat '%s' | sudo tee /var/lib/openyurt/kubelet.conf", kubletTempFilePath)
	utils.CheckErrorWithMsg(err, "Failed to configure kubelet!\n")
	node.ExecShellCmd(`sudo sed -i "s|KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=\/etc\/kubernetes\/bootstrap-kubelet.conf\ --kubeconfig=\/etc\/kubernetes\/kubelet.conf|KUBELET_KUBECONFIG_ARGS=--kubeconfig=\/var\/lib\/openyurt\/kubelet.conf|g" \
    /usr/lib/systemd/system/kubelet.service.d/10-kubeadm.conf`)
	utils.CheckErrorWithMsg(err, "Failed to configure kubelet!\n")
	node.ExecShellCmd("sudo systemctl daemon-reload && sudo systemctl restart kubelet")
	utils.CheckErrorWithMsg(err, "Failed to configure kubelet!\n")
}

func (node *Node) YurtRestart(worker *Node) {
	// Restart pods in the worker node
	utils.WaitPrintf("Restarting pods in the worker node")
	shellOutput, err := node.ExecShellCmd(GetRestartPodsShell(), worker.Configs.System.NodeHostName)
	utils.CheckErrorWithMsg(err, "Failed to restart pods in the worker node!\n")
	podsToBeRestarted := strings.Split(shellOutput, "\n")
	for _, pods := range podsToBeRestarted {
		podsInfo := strings.Split(pods, " ")
		utils.WaitPrintf("Restarting pod: %s => %s", podsInfo[0], podsInfo[1])
		_, err = node.ExecShellCmd("kubectl -n %s delete pod %s", podsInfo[0], podsInfo[1])
		utils.CheckErrorWithMsg(err, "Failed to restart pods in the worker node!\n")
	}
}

func (node *Node) YurtWorkerClean() {
	node.OnlyExecByWorker()
	var err error
	utils.WaitPrintf("Cleaning openyurt kubelet on node:%s", node.Name)
	_, err = node.ExecShellCmd("sudo rm -rf /var/lib/openyurt")
	_, err = node.ExecShellCmd("sudo rm /etc/kubernetes/pki/ca.crt")
	_, err = node.ExecShellCmd(`sudo sed -i "s|KUBELET_KUBECONFIG_ARGS=--kubeconfig=\/var\/lib\/openyurt\/kubelet.conf|KUBELET_KUBECONFIG_ARGS=--bootstrap-kubeconfig=\/etc\/kubernetes\/bootstrap-kubelet.conf\ --kubeconfig=\/etc\/kubernetes\/kubelet.conf|g" /etc/systemd/system/kubelet.service.d/10-kubeadm.conf`)
	utils.CheckErrorWithMsg(err, "Failed to clean kubelet on node: %s", node.Name)
}

// Builds cloud and edge nodepools for node pool management
func (masterNode *Node) BuildDemo(workerNodes []Node) {

	masterNode.GetUserHomeDir()
	masterNode.GetNodeHostName()

	var err error
	cloudPoolName := masterNode.Configs.Demo.CloudPoolName
	edgePoolName := masterNode.Configs.Demo.EdgePoolName

	// copy cloudNP file over
	cloudNPTmpFilePath, _ := masterNode.DownloadToTmpDir("https://raw.githubusercontent.com/vhive-serverless/vHive/openyurt/scripts/openyurt-deployer/configs/cloudNodePoolTemplate.yaml")
	cloudFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.CloudYamlFile)
	// cloud.yaml
	utils.WaitPrintf("Creating yaml files for cloud nodepool")
	cloudNpcommand := fmt.Sprintf("yq  '.metadata.name = \"%s\"' %s > %s ", cloudPoolName, cloudNPTmpFilePath, cloudFile)
	_, err = masterNode.ExecShellCmd("%s", cloudNpcommand)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create yaml for cloud\n")

	// Copy edgeNP file over
	edgeNPTmpFilePath, _ := masterNode.DownloadToTmpDir("https://raw.githubusercontent.com/vhive-serverless/vHive/openyurt/scripts/openyurt-deployer/configs/edgeNodePoolTemplate.yaml")
	edgeFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.EdgeYamlFile)
	// edge.yaml
	utils.WaitPrintf("Creating yaml files for edge nodepool")
	edgeNpcommand := fmt.Sprintf("yq  '.metadata.name = \"%s\"' %s > %s ", edgePoolName, edgeNPTmpFilePath, edgeFile)
	_, err = masterNode.ExecShellCmd("%s", edgeNpcommand)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create yaml for edge\n")

	utils.WaitPrintf("Apply cloud.yaml")
	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", cloudFile)
	utils.CheckErrorWithTagAndMsg(err, "Failed to apply cloud.yaml\n")

	utils.WaitPrintf("Apply edge.yaml")
	_, err = masterNode.ExecShellCmd("kubectl apply -f %s", edgeFile)
	utils.CheckErrorWithTagAndMsg(err, "Failed to apply edge.yaml\n")
}

// Add benchmarking image
func (masterNode *Node) Demo(isCloud bool) {

	masterNode.GetUserHomeDir()
	masterNode.GetNodeHostName()

	var err error
	cloudPoolName := masterNode.Configs.Demo.CloudPoolName
	edgePoolName := masterNode.Configs.Demo.EdgePoolName

	utils.WaitPrintf("Creating benchmark's yaml file and apply it")
	benchmarkFilePath, _ := masterNode.DownloadToTmpDir("https://raw.githubusercontent.com/vhive-serverless/vHive/openyurt/scripts/openyurt-deployer/configs/benchmarkTemplate.yaml")

	if isCloud {
		cloudOutputFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.CloudBenchYamlFile)

		command := fmt.Sprintf(`yq '.metadata.name = "helloworld-python-cloud" | 
			.spec.template.spec.nodeSelector."apps.openyurt.io/nodepool" = "%s" | 
			.spec.template.spec.containers[0].image = "docker.io/vhiveease/hello-cloud:latest"' %s > %s`,
			cloudPoolName, benchmarkFilePath, cloudOutputFile)
		_, err = masterNode.ExecShellCmd("%s", command)
		utils.CheckErrorWithMsg(err, "cloud benchmark command fail.")

		_, err = masterNode.ExecShellCmd("kubectl apply -f %s", cloudOutputFile)
	} else {
		edgeOutputFile := fmt.Sprintf("%s/%s", masterNode.Configs.System.UserHomeDir, masterNode.Configs.Demo.EdgeBenchYamlFile)

		command := fmt.Sprintf(`yq '.metadata.name = "helloworld-python-edge" | 
			.spec.template.spec.nodeSelector."apps.openyurt.io/nodepool" = "%s" | 
			.spec.template.spec.containers[0].image = "docker.io/vhiveease/hello-edge:latest"' %s > %s`,
			edgePoolName, benchmarkFilePath, edgeOutputFile)
		_, err = masterNode.ExecShellCmd("%s", command)
		utils.CheckErrorWithMsg(err, "edge benchmark command fail.")
		_, err = masterNode.ExecShellCmd("kubectl apply -f %s", edgeOutputFile)
	}
	utils.CheckErrorWithTagAndMsg(err, "Failed to create benchmark's yaml file and apply it")

}

func (masterNode *Node) PrintDemoInfo(workerNodes []Node, isCloud bool) {
	utils.InfoPrintf("NodePool Information:\n")
	utils.InfoPrintf("+--------------------------------------------------------------------+\n")
	npType := "cloud"
	if !isCloud {
		npType = "edge"
	}

	poolName := masterNode.Configs.Demo.CloudPoolName
	if !isCloud {
		poolName = masterNode.Configs.Demo.EdgePoolName
	}

	utils.InfoPrintf("+%s Nodepool %s:\n", npType, poolName)
	utils.InfoPrintf("+Nodes:\n")
	if isCloud {
		utils.InfoPrintf("+\tnode: %s <- Master\n", masterNode.Configs.System.NodeHostName)
	}
	for _, worker := range workerNodes {
		worker.GetNodeHostName()
		if worker.NodeRole == npType {
			utils.InfoPrintf("+\tnode: %s\n", worker.Configs.System.NodeHostName)
		}
	}

	shellOut, _ := masterNode.ExecShellCmd("kubectl get ksvc | grep '\\-%s' | awk '{print $1, substr($2, 8)}'", npType)
	var serviceName string
	var serviceURL string
	splittedOut := strings.Split(shellOut, " ")
	if len(splittedOut) != 2 {
		serviceName = "Null"
		serviceURL = "Null"
	} else {
		serviceName = splittedOut[0]
		serviceURL = splittedOut[1]
	}
	utils.SuccessPrintf("+Service: Name: [%s] with URL [%s]\n", serviceName, serviceURL)
	utils.InfoPrintf("+--------------------------------------------------------------------+\n")

}
