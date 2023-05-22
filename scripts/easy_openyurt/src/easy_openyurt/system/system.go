// Author: Haoyuan Ma <flyinghorse0510@zju.edu.cn>
package system

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	configs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/configs"
	logs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/logs"
)

// Implement error interface of ShellError
type ShellError struct {
	msg      string
	exitCode int
}

func (err *ShellError) Error() string {
	return fmt.Sprintf("[exit %d] -> %s", err.exitCode, err.msg)
}

// Parse parameters for subcommand `system`
func ParseSubcommandSystem(args []string) {
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

	// Parse parameters for `system master/worker init`
	var help bool
	systemFlagsName := fmt.Sprintf("%s system %s init", os.Args[0], nodeRole)
	systemFlags := flag.NewFlagSet(systemFlagsName, flag.ExitOnError)
	systemFlags.StringVar(&configs.System.GoVersion, "go-version", configs.System.GoVersion, "Golang version")
	systemFlags.StringVar(&configs.System.ContainerdVersion, "containerd-version", configs.System.ContainerdVersion, "Containerd version")
	systemFlags.StringVar(&configs.System.RuncVersion, "runc-version", configs.System.RuncVersion, "Runc version")
	systemFlags.StringVar(&configs.System.CniPluginsVersion, "cni-plugins-version", configs.System.CniPluginsVersion, "CNI plugins version")
	systemFlags.StringVar(&configs.System.KubectlVersion, "kubectl-version", configs.System.KubectlVersion, "Kubectl version")
	systemFlags.StringVar(&configs.System.KubeadmVersion, "kubeadm-version", configs.System.KubeadmVersion, "Kubeadm version")
	systemFlags.StringVar(&configs.System.KubeletVersion, "kubelet-version", configs.System.KubeletVersion, "Kubelet version")
	systemFlags.BoolVar(&help, "help", false, "Show help")
	systemFlags.BoolVar(&help, "h", false, "Show help")
	systemFlags.Parse(args[2:])
	// Show help
	if help {
		systemFlags.Usage()
		os.Exit(0)
	}
	SystemInit()
	logs.SuccessPrintf("Init System Successfully!\n")
}

// Execute Shell Command
func ExecShellCmd(cmd string, pars ...any) (string, error) {
	// Allocate bytes buffer
	bashCmd := new(bytes.Buffer)
	cmdStdout := new(bytes.Buffer)
	cmdStderr := new(bytes.Buffer)
	fmt.Fprintf(bashCmd, cmd, pars...)
	bashProcess := exec.Command("bash", "-c", bashCmd.String())
	// Redirect stdout & stderr
	bashProcess.Stdout = cmdStdout
	bashProcess.Stderr = cmdStderr

	// Execute command
	err := bashProcess.Run()

	// remove suffix "\n" in Stdout & Stderr
	var trimmedStdout string
	var trimmedStderr string
	if cmdStdout.Len() > 0 {
		trimmedStdout = strings.TrimSuffix(cmdStdout.String(), "\n")
	} else {
		trimmedStdout = ""
	}
	if cmdStderr.Len() > 0 {
		trimmedStderr = strings.TrimSuffix(cmdStderr.String(), "\n")
	} else {
		trimmedStderr = ""
	}

	// Rewrite error message
	if err != nil {
		err = &ShellError{msg: trimmedStderr, exitCode: bashProcess.ProcessState.ExitCode()}
	}

	// For logs
	if logs.CommonLog != nil {
		logs.CommonLog.Printf("Executing shell command: %s\n", bashCmd.String())
		logs.CommonLog.Printf("Stdout from shell:\n%s\n", trimmedStdout)
	}
	if logs.ErrorLog != nil {
		logs.ErrorLog.Printf("Executing shell command: %s\n", bashCmd.String())
		logs.ErrorLog.Printf("Stderr from shell:\n%s\n", trimmedStderr)
	}

	return trimmedStdout, err
}

// Detect current architecture
func DetectArch() {
	switch configs.System.CurrentArch {
	default:
		logs.InfoPrintf("Detected Arch: %s\n", configs.System.CurrentArch)
	}
}

// Detect current operating system
func DetectOS() {
	switch configs.System.CurrentOS {
	case "windows":
		logs.FatalPrintf("Unsupported OS: %s\n", configs.System.CurrentOS)
	default:
		var err error
		configs.System.CurrentOS, err = ExecShellCmd("sed -n 's/^NAME=\"\\(.*\\)\"/\\1/p' < /etc/os-release | head -1 | tr '[:upper:]' '[:lower:]'")
		logs.CheckErrorWithMsg(err, "Failed to get Linux distribution info!\n")
		switch configs.System.CurrentOS {
		case "ubuntu":
		default:
			logs.FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
		}
		logs.InfoPrintf("Detected OS: %s\n", strings.TrimSuffix(string(configs.System.CurrentOS), "\n"))
	}
}

// Get current directory
func GetCurrentDir() {
	var err error
	configs.System.CurrentDir, err = os.Getwd()
	logs.CheckErrorWithMsg(err, "Failed to get get current directory!\n")
}

// Get current home directory
func GetUserHomeDir() {
	var err error
	configs.System.UserHomeDir, err = os.UserHomeDir()
	logs.CheckErrorWithMsg(err, "Failed to get current home directory!\n")
}

// Create temporary directory
func CreateTmpDir() {
	var err error
	logs.WaitPrintf("Creating temporary directory")
	configs.System.TmpDir, err = os.MkdirTemp("", "yurt_tmp")
	logs.CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
}

// Clean up temporary directory
func CleanUpTmpDir() {
	logs.WaitPrintf("Cleaning up temporary directory")
	err := os.RemoveAll(configs.System.TmpDir)
	logs.CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
}

// Download file to temporary directory (absolute path of downloaded file will be the first return value if successful)
func DownloadToTmpDir(urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	fileName := path.Base(url)
	filePath := configs.System.TmpDir + "/" + fileName
	_, err := ExecShellCmd("curl -sSL --output %s %s", filePath, url)
	return filePath, err
}

// Extract arhive file to specific directory(currently support .tar.gz file only)
func ExtractToDir(filePath string, dirPath string, privileged bool) error {
	var err error
	if privileged {
		_, err = ExecShellCmd("sudo tar -xzvf %s -C %s", filePath, dirPath)
	} else {
		_, err = ExecShellCmd("tar -xzvf %s -C %s", filePath, dirPath)
	}
	return err
}

// Install packages on various OS
func InstallPackages(packagesTemplate string, pars ...any) error {
	packages := fmt.Sprintf(packagesTemplate, pars...)
	switch configs.System.CurrentOS {
	case "ubuntu":
		_, err := ExecShellCmd("sudo apt-get -qq update && sudo apt-get -qq install -y --allow-downgrades %s", packages)
		return err
	case "centos":
		_, err := ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	case "rocky linux":
		_, err := ExecShellCmd("sudo dnf -y -q install %s", packages)
		return err
	default:
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
		return &ShellError{msg: "Unsupported Linux distribution", exitCode: 1}
	}
}

// Turn off unattended-upgrades
func TurnOffAutomaticUpgrade() (string, error) {
	switch configs.System.CurrentOS {
	case "ubuntu":
		_, err := os.Stat("/etc/apt/apt.conf.d/20auto-upgrades")
		if err == nil {
			return ExecShellCmd("sudo sed -i 's/\"1\"/\"0\"/g' /etc/apt/apt.conf.d/20auto-upgrades")
		}
		return "", nil
	default:
		return "", nil
	}
}

// Check system environment
func CheckSystemEnvironment() {
	// Check system environment
	logs.InfoPrintf("Checking system environment...\n")
	var err error

	// Check Golang
	_, err = exec.LookPath("go")
	if err != nil {
		logs.WarnPrintf("Golang not found! Golang(version %s) will be automatically installed!\n", configs.System.GoVersion)
	} else {
		logs.SuccessPrintf("Golang found!\n")
		configs.System.GoInstalled = true
	}

	// Check Containerd
	_, err = exec.LookPath("containerd")
	if err != nil {
		logs.WarnPrintf("Containerd not found! containerd(version %s) will be automatically installed!\n", configs.System.ContainerdVersion)
	} else {
		logs.SuccessPrintf("Containerd found!\n")
		configs.System.ContainerdInstalled = true
	}

	// Check runc
	_, err = exec.LookPath("runc")
	if err != nil {
		logs.WarnPrintf("runc not found! runc(version %s) will be automatically installed!\n", configs.System.RuncVersion)
	} else {
		logs.SuccessPrintf("runc found!\n")
		configs.System.RuncInstalled = true
	}

	// Check CNI plugins
	_, err = os.Stat("/opt/cni/bin")
	if err != nil {
		logs.WarnPrintf("CNI plugins not found! CNI plugins(version %s) will be automatically installed!\n", configs.System.CniPluginsVersion)
	} else {
		logs.SuccessPrintf("CNI plugins found!\n")
		configs.System.CniPluginsInstalled = true
	}

	// Add OS-specific dependencies to installation lists
	switch configs.System.CurrentOS {
	case "ubuntu":
		configs.System.Dependencies = "git wget curl build-essential apt-transport-https ca-certificates"
	case "rocky linux":
		configs.System.Dependencies = ""
	case "centos":
		configs.System.Dependencies = ""
	default:
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
	}

	logs.SuccessPrintf("Finish checking system environment!\n")
}

// Create logs
func CreateLogs() {
	logs.CreateLogs(configs.System.CurrentDir)
}

// Append directory to PATH variable for bash & zsh
func AppendDirToPath(pathTemplate string, pars ...any) error {
	appendedPath := fmt.Sprintf(pathTemplate, pars...)

	// For bash
	_, err := ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.bashrc", appendedPath, configs.System.UserHomeDir)
	if err != nil {
		return err
	}
	// For zsh
	_, err = exec.LookPath("zsh")
	if err != nil {
		_, err = ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.zshrc", appendedPath, configs.System.UserHomeDir)
	}
	return err
}

// Initialize system environment
func SystemInit() {

	// Initialize
	var err error
	CheckSystemEnvironment()
	CreateTmpDir()
	defer CleanUpTmpDir()

	// Turn off unattended-upgrades on ubuntu
	logs.WaitPrintf("Turning off automatic upgrade")
	_, err = TurnOffAutomaticUpgrade()
	logs.CheckErrorWithTagAndMsg(err, "Failed to turn off automatic upgrade!\n")

	// Disable swap
	logs.WaitPrintf("Disabling swap")
	_, err = ExecShellCmd("sudo swapoff -a && sudo cp /etc/fstab /etc/fstab.old") // Turn off Swap && Backup fstab file
	logs.CheckErrorWithTagAndMsg(err, "Failed to disable swap!\n")

	logs.WaitPrintf("Modifying fstab")
	// Modify fstab to disable swap permanently
	_, err = ExecShellCmd("sudo sed -i 's/#\\s*\\(.*swap.*\\)/\\1/g' /etc/fstab && sudo sed -i 's/.*swap.*/# &/g' /etc/fstab")
	logs.CheckErrorWithTagAndMsg(err, "Failed to dodify fstab!\n")

	// Install dependencies
	logs.WaitPrintf("Installing dependencies")
	err = InstallPackages(configs.System.Dependencies)
	logs.CheckErrorWithTagAndMsg(err, "Failed to install dependencies!\n")

	// Install Golang
	if !configs.System.GoInstalled {
		// Download & Extract Golang
		logs.WaitPrintf("Downloading Golang(ver %s)", configs.System.GoVersion)
		filePathName, err := DownloadToTmpDir(configs.System.GoDownloadUrlTemplate, configs.System.GoVersion, configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download Golang(ver %s)!\n", configs.System.GoVersion)
		logs.WaitPrintf("Extracting Golang")
		_, err = ExecShellCmd("sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract Golang!\n")

		// For bash
		_, err = ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.bashrc", configs.System.UserHomeDir)
		logs.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		// For zsh
		_, err = exec.LookPath("zsh")
		if err != nil {
			_, err = ExecShellCmd("echo 'export PATH=$PATH:/usr/local/go/bin' >> %s/.zshrc", configs.System.UserHomeDir)
			logs.CheckErrorWithMsg(err, "Failed to update PATH!\n")
		}
	}

	// Install containerd
	if !configs.System.ContainerdInstalled {
		// Download containerd
		logs.WaitPrintf("Downloading containerd(ver %s)", configs.System.ContainerdVersion)
		filePathName, err := DownloadToTmpDir(
			configs.System.ContainerdDownloadUrlTemplate,
			configs.System.ContainerdVersion,
			configs.System.ContainerdVersion,
			configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to Download containerd(ver %s)\n", configs.System.ContainerdVersion)
		// Extract containerd
		logs.WaitPrintf("Extracting containerd")
		_, err = ExecShellCmd("sudo tar Cxzvf /usr/local %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract containerd!\n")
		// Start containerd via systemd
		logs.WaitPrintf("Downloading systemd profile for containerd")
		filePathName, err = DownloadToTmpDir(configs.System.ContainerdSystemdProfileDownloadUrl)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download systemd profile for containerd!\n")
		logs.WaitPrintf("Starting containerd via systemd")
		_, err = ExecShellCmd("sudo cp %s /lib/systemd/system/ && sudo systemctl daemon-reload && sudo systemctl enable --now containerd", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to start containerd via systemd!\n")
	}

	// Install runc
	if !configs.System.RuncInstalled {
		// Download runc
		logs.WaitPrintf("Downloading runc(ver %s)", configs.System.RuncVersion)
		filePathName, err := DownloadToTmpDir(
			configs.System.RuncDownloadUrlTemplate,
			configs.System.RuncVersion,
			configs.System.CurrentArch)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download runc(ver %s)!\n", configs.System.RuncVersion)
		// Install runc
		logs.WaitPrintf("Installing runc")
		_, err = ExecShellCmd("sudo install -m 755 %s /usr/local/sbin/runc", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to install runc!\n")
	}

	// Install CNI plugins
	if !configs.System.CniPluginsInstalled {
		logs.WaitPrintf("Downloading CNI plugins(ver %s)", configs.System.CniPluginsVersion)
		filePathName, err := DownloadToTmpDir(
			configs.System.CniPluginsDownloadUrlTemplate,
			configs.System.CniPluginsVersion,
			configs.System.CurrentArch,
			configs.System.CniPluginsVersion)
		logs.CheckErrorWithTagAndMsg(err, "Failed to download CNI plugins(ver %s)!\n", configs.System.CniPluginsVersion)
		logs.WaitPrintf("Extracting CNI plugins")
		_, err = ExecShellCmd("sudo mkdir -p /opt/cni/bin && sudo tar Cxzvf /opt/cni/bin %s", filePathName)
		logs.CheckErrorWithTagAndMsg(err, "Failed to extract CNI plugins!\n")
	}

	// Configure the systemd cgroup driver
	logs.WaitPrintf("Configuring the systemd cgroup driver")
	_, err = ExecShellCmd(
		"containerd config default > %s && sudo mkdir -p /etc/containerd && sudo cp %s /etc/containerd/config.toml && sudo sed -i 's/SystemdCgroup = false/SystemdCgroup = true/g' /etc/containerd/config.toml && sudo systemctl restart containerd",
		configs.System.TmpDir+"/config.toml",
		configs.System.TmpDir+"/config.toml")
	logs.CheckErrorWithTagAndMsg(err, "Failed to configure the systemd cgroup driver!\n")

	// Enable IP forwading & br_netfilter
	logs.WaitPrintf("Enabling IP forwading & br_netfilter")
	_, err = ExecShellCmd("sudo modprobe br_netfilter && sudo modprobe overlay && sudo sysctl -w net.ipv4.ip_forward=1 && sudo sysctl -w net.ipv4.conf.all.forwarding=1 && sudo sysctl -w net.bridge.bridge-nf-call-iptables=1 && sudo sysctl -w net.bridge.bridge-nf-call-ip6tables=1")
	logs.CheckErrorWithTagAndMsg(err, "Failed to enable IP forwading & br_netfilter!\n")
	// Ensure Boot-Resistant
	logs.WaitPrintf("Ensuring Boot-Resistant")
	_, err = ExecShellCmd("echo 'br_netfilter' | sudo tee /etc/modules-load.d/netfilter.conf && echo 'overlay' | sudo tee -a /etc/modules-load.d/netfilter.conf && sudo sed -i 's/# *net.ipv4.ip_forward=1/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && sudo sed -i 's/net.ipv4.ip_forward=0/net.ipv4.ip_forward=1/g' /etc/sysctl.conf && echo 'net.bridge.bridge-nf-call-iptables=1\nnet.bridge.bridge-nf-call-ip6tables=1\nnet.ipv4.conf.all.forwarding=1' | sudo tee /etc/sysctl.d/99-kubernetes-cri.conf")
	logs.CheckErrorWithTagAndMsg(err, "Failed to ensure Boot-Resistant!\n")

	// Install kubeadm, kubelet, kubectl
	switch configs.System.CurrentOS {
	case "ubuntu":
		// Download Google Cloud public signing key and Add the Kubernetes apt repository
		logs.WaitPrintf("Adding the Kubernetes apt repository")
		_, err = ExecShellCmd("sudo mkdir -p /etc/apt/keyrings && sudo curl -fsSLo /etc/apt/keyrings/kubernetes-archive-keyring.gpg https://dl.k8s.io/apt/doc/apt-key.gpg && echo 'deb [signed-by=/etc/apt/keyrings/kubernetes-archive-keyring.gpg] https://apt.kubernetes.io/ kubernetes-xenial main' | sudo tee /etc/apt/sources.list.d/kubernetes.list")
		logs.CheckErrorWithTagAndMsg(err, "Failed to add the Kubernetes apt repository!\n")
		// Install kubeadm, kubelet, kubectl via apt
		logs.WaitPrintf("Installing kubeadm, kubelet, kubectl")
		err = InstallPackages("kubeadm=%s kubelet=%s kubectl=%s", configs.System.KubeadmVersion, configs.System.KubeletVersion, configs.System.KubectlVersion)
		logs.CheckErrorWithTagAndMsg(err, "Failed to install kubeadm, kubelet, kubectl!\n")
		// Lock kubeadm, kubelet, kubectl version
		logs.WaitPrintf("Locking kubeadm, kubelet, kubectl version")
		_, err = ExecShellCmd("sudo apt-mark hold kubelet kubeadm kubectl")
		logs.CheckErrorWithTagAndMsg(err, "Failed to lock kubeadm, kubelet, kubectl version!\n")
	default:
		logs.FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
	}
}
