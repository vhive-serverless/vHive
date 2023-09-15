package node

import (
	"fmt"
	"path"
	"strings"

	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
)

type ShellError struct {
	msg      string
	exitCode int
}

func (err *ShellError) Error() string {
	return fmt.Sprintf("[exit %d] -> %s", err.exitCode, err.msg)
}

// Detect current architecture
func (node *Node) DetectArch() {
	logs.WaitPrintf("Detetcting current arch")
	out ,err := node.ExecShellCmd("dpkg --print-architecture")
	logs.CheckErrorWithTagAndMsg(err, "Failed to detec current arch!")
	node.Configs.System.CurrentArch = out
	switch node.Configs.System.CurrentArch {
	default:
		logs.InfoPrintf("Detected Arch: %s for node: %s\n", node.Configs.System.CurrentArch, node.Name)
	}
}

// Detect current operating system
func (node *Node) DetectOS() {
	switch node.Configs.System.CurrentOS {
	case "windows":
		logs.FatalPrintf("Unsupported OS: %s\n", node.Configs.System.CurrentOS)
	default:
		var err error
		node.Configs.System.CurrentOS, err = node.ExecShellCmd("sed -n 's/^NAME=\"\\(.*\\)\"/\\1/p' < /etc/os-release | head -1 | tr '[:upper:]' '[:lower:]'")
		logs.InfoPrintf("Detected OS: %s\n", node.Configs.System.CurrentOS)
		logs.CheckErrorWithMsg(err, "Failed to get Linux distribution info!\n")
		switch node.Configs.System.CurrentOS {
		case "ubuntu":
		default:
			logs.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
		}
		logs.InfoPrintf("Detected OS: %s for node: %s\n", strings.TrimSuffix(string(node.Configs.System.CurrentOS), "\n"), node.Name)
	}
}

// Get current directory
func (node *Node) GetCurrentDir() {
	var err error
	node.Configs.System.CurrentDir, err = node.ExecShellCmd("pwd")
	logs.CheckErrorWithMsg(err, "Failed to get get current directory!\n")
}

// Get current home directory
func (node *Node) GetUserHomeDir() {
	var err error
	node.Configs.System.UserHomeDir, err = node.ExecShellCmd("echo $HOME")
	logs.CheckErrorWithMsg(err, "Failed to get current home directory!\n")
}

// Get current node's hostname
func (node *Node) GetNodeHostName() {
	var err error
	node.Configs.System.NodeHostName, err = node.ExecShellCmd("echo $HOSTNAME")
	logs.CheckErrorWithMsg(err, "Failed to get current node hostname!\n")
}

// Create temporary directory
func (node *Node) CreateTmpDir() {
	var err error
	logs.InfoPrintf("Creating temporary directory")
	tmpDir := "~/yurt_tmp"
	_, err = node.ExecShellCmd("mkdir -p %s", tmpDir)
	node.Configs.System.TmpDir = tmpDir
	logs.CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
}

// Clean up temporary directory
func (node *Node) CleanUpTmpDir() {
	logs.InfoPrintf("Cleaning up temporary directory")
	_, err := node.ExecShellCmd("rm -rf %s/*", node.Configs.System.TmpDir)
	logs.CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
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
		_, err := node.ExecShellCmd("sudo apt-get -qq update && sudo apt-get -qq install -y --allow-downgrades %s", packages)
		return err
	case "centos":
		_, err := node.ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	case "rocky linux":
		_, err := node.ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	default:
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
		return &ShellError{msg: "Unsupported Linux distribution", exitCode: 1}
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
	logs.InfoPrintf("Checking system environment...\n")
	var err error

	// Check Golang
	_, err = node.LookPath("go")
	if err != nil {
		logs.InfoPrintf("Golang not found! Golang(version %s) will be automatically installed!\n", node.Configs.System.GoVersion)
	} else {
		logs.InfoPrintf("Golang found!\n")
		node.Configs.System.GoInstalled = true
	}

	// Check Containerd
	_, err = node.LookPath("containerd")
	if err != nil {
		logs.InfoPrintf("Containerd not found! containerd(version %s) will be automatically installed!\n", node.Configs.System.ContainerdVersion)
	} else {
		logs.InfoPrintf("Containerd found!\n")
		node.Configs.System.ContainerdInstalled = true
	}

	// Check runc
	_, err = node.LookPath("runc")
	if err != nil {
		logs.InfoPrintf("runc not found! runc(version %s) will be automatically installed!\n", node.Configs.System.RuncVersion)
	} else {
		logs.InfoPrintf("runc found!\n")
		node.Configs.System.RuncInstalled = true
	}

	// Check CNI plugins
	_, err = node.ExecShellCmd("stat /opt/cni/bin")
	if err != nil {
		logs.InfoPrintf("CNI plugins not found! CNI plugins(version %s) will be automatically installed!\n", node.Configs.System.CniPluginsVersion)
	} else {
		logs.InfoPrintf("CNI plugins found!\n")
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
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
	}

	logs.InfoPrintf("Finish checking system environment!\n")
}

// Initialize system environment
func (node *Node) SystemInit() {
	logs.InfoPrintf("Start init system environment for node:%s\n", node.Name)
	// Initialize

	var err error
	node.DetectOS()
	node.DetectArch()
	node.GetCurrentDir()
	node.GetUserHomeDir()
	node.GetNodeHostName()
	node.CheckSystemEnvironment()
	node.CreateTmpDir()
	defer node.CleanUpTmpDir()

	// Turn off unattended-upgrades on ubuntu
	logs.InfoPrintf("Turning off automatic upgrade")
	_, err = node.TurnOffAutomaticUpgrade()
	logs.CheckErrorWithTagAndMsg(err, "Failed to turn off automatic upgrade!\n")

	// Disable swap
	logs.InfoPrintf("Disabling swap")
	_, err = node.ExecShellCmd("sudo swapoff -a && sudo cp /etc/fstab /etc/fstab.old") // Turn off Swap && Backup fstab file
	logs.CheckErrorWithTagAndMsg(err, "Failed to disable swap!\n")

	logs.InfoPrintf("Modifying fstab")
	// Modify fstab to disable swap permanently
	_, err = node.ExecShellCmd("sudo sed -i 's/#\\s*\\(.*swap.*\\)/\\1/g' /etc/fstab && sudo sed -i 's/.*swap.*/# &/g' /etc/fstab")
	logs.CheckErrorWithTagAndMsg(err, "Failed to dodify fstab!\n")

	// Install dependencies
	logs.InfoPrintf("Installing dependencies")
	err = node.InstallPackages(node.Configs.System.Dependencies)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install dependencies!\n")

	// Install Golang
	if !node.Configs.System.GoInstalled {
		// Download & Extract Golang
		logs.InfoPrintf("Downloading Golang(ver %s)", node.Configs.System.GoVersion)
		filePathName, err := node.DownloadToTmpDir(node.Configs.System.GoDownloadUrlTemplate, node.Configs.System.GoVersion, node.Configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download Golang(ver %s)!\n", node.Configs.System.GoVersion)
		logs.InfoPrintf("Extracting Golang")
		_, err = node.ExecShellCmd("sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract Golang!\n")

		// For bash
		_, err = node.ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.bashrc", node.Configs.System.UserHomeDir)
		logs.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		// For zsh
		_, err = node.LookPath("zsh")
		if err != nil {
			_, err = node.ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.zshrc", node.Configs.System.UserHomeDir)
			logs.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		}
	}

	// Install containerd
	if !node.Configs.System.ContainerdInstalled {
		// Download containerd
		logs.InfoPrintf("Downloading containerd(ver %s)", node.Configs.System.ContainerdVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.ContainerdDownloadUrlTemplate,
			node.Configs.System.ContainerdVersion,
			node.Configs.System.ContainerdVersion,
			node.Configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to Download containerd(ver %s)\n", node.Configs.System.ContainerdVersion)
		// Extract containerd
		logs.InfoPrintf("Extracting containerd")
		_, err = node.ExecShellCmd("sudo tar Cxzvf /usr/local %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract containerd!\n")
		// Start containerd via systemd
		logs.InfoPrintf("Downloading systemd profile for containerd")
		filePathName, err = node.DownloadToTmpDir(node.Configs.System.ContainerdSystemdProfileDownloadUrl)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download systemd profile for containerd!\n")
		logs.InfoPrintf("Starting containerd via systemd")
		_, err = node.ExecShellCmd("sudo cp %s /lib/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now containerd", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to start containerd via systemd!\n")
	}

	// Install runc
	if !node.Configs.System.RuncInstalled {
		// Download runc
		logs.InfoPrintf("Downloading runc(ver %s)", node.Configs.System.RuncVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.RuncDownloadUrlTemplate,
			node.Configs.System.RuncVersion,
			node.Configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download runc(ver %s)!\n", node.Configs.System.RuncVersion)
		// Install runc
		logs.InfoPrintf("Installing runc")
		_, err = node.ExecShellCmd("sudo install -m 755 %s /usr/local/sbin/runc", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to install runc!\n")
	}

	// Install CNI plugins
	if !node.Configs.System.CniPluginsInstalled {
		logs.InfoPrintf("Downloading CNI plugins(ver %s)", node.Configs.System.CniPluginsVersion)
		filePathName, err := node.DownloadToTmpDir(
			node.Configs.System.CniPluginsDownloadUrlTemplate,
			node.Configs.System.CniPluginsVersion,
			node.Configs.System.CurrentArch,
			node.Configs.System.CniPluginsVersion)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download CNI plugins(ver %s)!\n", node.Configs.System.CniPluginsVersion)
		logs.InfoPrintf("Extracting CNI plugins")
		_, err = node.ExecShellCmd("sudo mkdir -p /opt/cni/bin && sudo tar Cxzvf /opt/cni/bin %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract CNI plugins!\n")
	}

	// Configure the systemd cgroup driver
	logs.InfoPrintf("Configuring the systemd cgroup driver")
	_, err = node.ExecShellCmd(
		"containerd config default > %s && sudo mkdir -p /etc/containerd && sudo cp %s /etc/containerd/config.toml && sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml && sudo systemctl restart containerd",
		node.Configs.System.TmpDir+"/config.toml",
		node.Configs.System.TmpDir+"/config.toml")
	logs.CheckErrorWithTagAndMsg(err, "Failed to configure the systemd cgroup driver!\n")

	// Enable IP forwading & br_netfilter
	logs.InfoPrintf("Enabling IP forwading & br_netfilter")
	_, err = node.ExecShellCmd("sudo modprobe br_netfilter && sudo modprobe overlay && sudo sysctl -w net.ipv4.ip_forward=1 && sudo sysctl -w net.ipv4.conf.all.forwarding=1 && sudo sysctl -w net.bridge.bridge-nf-call-iptables=1 && sudo sysctl -w net.bridge.bridge-nf-call-ip6tables=1")
	logs.CheckErrorWithTagAndMsg(err, "Failed to enable IP forwading & br_netfilter!\n")
	// Ensure Boot-Resistant
	logs.InfoPrintf("Ensuring Boot-Resistant")
	_, err = node.ExecShellCmd("echo 'br_netfilter' | sudo tee /etc/modules-load.d/netfilter.conf && echo 'overlay' | sudo tee -a /etc/modules-load.d/netfilter.conf && sudo sed -i 's/# *net.ipv4.ip_forward=1/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && sudo sed -i 's/net.ipv4.ip_forward=0/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && echo 'net.bridge.bridge-nf-call-iptables=1\nnet.bridge.bridge-nf-call-ip6tables=1\nnet.ipv4.conf.all.forwarding=1' | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf")
	logs.CheckErrorWithTagAndMsg(err, "Failed to ensure Boot-Resistant!\n")

	// Install kubeadm, kubelet, kubectl
	switch node.Configs.System.CurrentOS {
	case "ubuntu":
		// Download Google Cloud public signing key and Add the Kubernetes apt repository
		logs.InfoPrintf("Adding the Kubernetes apt repository")
		_, err = node.ExecShellCmd("sudo mkdir -p /etc/apt/keyrings && sudo curl -fsSLo /etc/apt/keyrings/kubernetes-archive-keyring.gpg https://dl.k8s.io/apt/doc/apt-key.gpg && echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main' | sudo tee /etc/apt/sources.list.d/kubernetes.list")
		logs.CheckErrorWithTagAndMsg(err, "Failed to add the Kubernetes apt repository!\n")
		// Install kubeadm, kubelet, kubectl via apt
		logs.InfoPrintf("Installing kubeadm, kubelet, kubectl")
		err = node.InstallPackages("kubeadm=%s kubelet=%s kubectl=%s", node.Configs.System.KubeadmVersion, node.Configs.System.KubeletVersion, node.Configs.System.KubectlVersion)
		logs.CheckErrorWithTagAndMsg(err, "Failed to install kubeadm, kubelet, kubectl!\n")
		// Lock kubeadm, kubelet, kubectl version
		logs.InfoPrintf("Locking kubeadm, kubelet, kubectl version")
		_, err = node.ExecShellCmd("sudo apt-mark hold kubelet kubeadm kubectl")
		logs.CheckErrorWithTagAndMsg(err, "Failed to lock kubeadm, kubelet, kubectl version!\n")
	default:
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", node.Configs.System.CurrentOS)
	}
}
