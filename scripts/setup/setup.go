package setup

import (
	"path"
	"time"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupFirecrackerContainerd() error {
	_, err := utils.ExecShellCmd("sudo mkdir -p /etc/firecracker-containerd && sudo mkdir -p /var/lib/firecracker-containerd/runtime && sudo mkdir -p /etc/containerd/")
	if err != nil {
		return err
	}

	utils.WaitPrintf("Pulling vHive repo and LFS")
	vHiveRepoPath, err := utils.CloneRepoToTmpDir(configs.VHive.FirecrackerVHiveBranch, configs.VHive.FirecrackerVHiveRepoUrl)
	if !utils.CheckErrorWithMsg(err, "Failed to pull vHive repo and LFS!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("cd %s && git lfs pull", vHiveRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to pull vHive repo and LFS!\n") {
		return err
	}

	utils.WaitPrintf("Copying required files")
	dstDir := "/usr/local/bin"
	binsDir := path.Join(vHiveRepoPath, "bin")
	// configsDir := path.Join(vHiveRepoPath, "configs/firecracker-containerd")

	binLists := []string{
		"firecracker",
		"jailer",
		"containerd-shim-aws-firecracker",
		"firecracker-containerd",
		"firecracker-ctr",
	}
	for _, bin := range binLists {
		src := path.Join(binsDir, bin)
		err = utils.CopyToDir(src, dstDir, true)
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
	}

	// rootfs image
	err = utils.CopyToDir(path.Join(binsDir, "default-rootfs.img"), "/var/lib/firecracker-containerd/runtime/", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}

	return nil
}

func SetupGvisorContainerd() error {
	// Create required directory
	_, err := utils.ExecShellCmd("sudo mkdir -p /etc/gvisor-containerd && sudo mkdir -p /etc/cni/net.d")
	if err != nil {
		return err
	}

	utils.WaitPrintf("Pulling vHive repo and LFS")
	vHiveRepoPath, err := utils.CloneRepoToTmpDir(configs.VHive.GVisorVHiveBranch, configs.VHive.GVisorVHiveRepoUrl)
	if !utils.CheckErrorWithMsg(err, "Failed to pull vHive repo and LFS!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("cd %s && git lfs pull", vHiveRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to pull vHive repo and LFS!\n") {
		return err
	}

	// Copy various required files
	utils.WaitPrintf("Copying required files")
	dstDir := "/usr/local/bin"
	binsDir := path.Join(vHiveRepoPath, "bin")
	ctrdConfigsDir := path.Join(vHiveRepoPath, "configs/gvisor-containerd")
	cniConfigsDir := path.Join(vHiveRepoPath, "configs/cni")

	binLists := []string{
		"containerd-shim-runsc-v1",
		"gvisor-containerd",
	}
	for _, bin := range binLists {
		src := path.Join(binsDir, bin)
		err = utils.CopyToDir(src, dstDir, true)
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
	}

	err = utils.CopyToDir(path.Join(ctrdConfigsDir, "config.toml"), "/etc/gvisor-containerd/", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}

	dstDir = "/etc/cni/net.d"
	configLists := []string{
		"10-bridge.conf",
		"99-loopback.conf",
	}
	for _, configFile := range configLists {
		src := path.Join(cniConfigsDir, configFile)
		err = utils.CopyToDir(src, dstDir, true)
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
	}
	utils.SuccessPrintf("\n")

	return nil
}

func SetupSystem() error {
	// Original Bash Scripts: scripts/setup_system.sh

	// Install required dependencies
	utils.WaitPrintf("Installing required dependencies")
	err := utils.InstallPackages("curl ca-certificates")
	if !utils.CheckErrorWithMsg(err, "Failed to install required dependencies!\n") {
		return err
	}
	// Add to apt repo
	_, err = utils.ExecShellCmd("sudo add-apt-repository -y universe")
	if !utils.CheckErrorWithMsg(err, "Failed to install required dependencies!\n") {
		return err
	}
	err = utils.InstallPackages("apt-transport-https gcc g++ make acl net-tools git-lfs bc gettext-base jq dmsetup gnupg-agent software-properties-common iproute2 nftables")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install required dependencies!\n") {
		return err
	}

	// ************************************************************************************
	// *********** A very direct copy from original script, need to be improved ***********
	// ************************************************************************************

	// # stack size, # of open files, # of pids
	// Configure resource limitation
	utils.WaitPrintf("Configuring resource limitation")
	bashCmd :=
		`sudo sh -c "echo \"* soft nofile 1000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"* hard nofile 1000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root soft nofile 1000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root hard nofile 1000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"* soft nproc 4000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"* hard nproc 4000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root soft nproc 4000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root hard nproc 4000000\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"* soft stack 65536\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"* hard stack 65536\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root soft stack 65536\" >> /etc/security/limits.conf" && ` +
			`sudo sh -c "echo \"root hard stack 65536\" >> /etc/security/limits.conf"`
	_, err = utils.ExecShellCmd(bashCmd)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to configure resource limitation!\n") {
		return err
	}

	// Avoid "neighbour: arp_cache: neighbor table overflow!"
	utils.WaitPrintf(`Avoiding "neighbour: arp_cache: neighbor table overflow!"`)
	bashCmd =
		`sudo sysctl --quiet -w net.ipv4.conf.all.forwarding=1 && ` +
			`sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh1=1024 && ` +
			`sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh2=2048 && ` +
			`sudo sysctl --quiet -w net.ipv4.neigh.default.gc_thresh3=4096 && ` +
			`sudo sysctl --quiet -w net.ipv4.ip_local_port_range="32769 65535" && ` +
			`sudo sysctl --quiet -w kernel.pid_max=4194303 && ` +
			`sudo sysctl --quiet -w kernel.threads-max=999999999 && ` +
			`sudo sysctl --quiet -w net.ipv4.ip_forward=1 && ` +
			`sudo sysctl --quiet --system`
	_, err = utils.ExecShellCmd(bashCmd)
	if !utils.CheckErrorWithTagAndMsg(err, `Failed to avoid "neighbour: arp_cache: neighbor table overflow!"`+"\n") {
		return err
	}

	// `sudo swapoff -a >> /dev/null`
	// ********************************************************************
	// Disable swap moved here, a more appropriate approach(boot resistant)
	// ********************************************************************
	// Disable swap
	utils.WaitPrintf("Disabling swap")
	_, err = utils.ExecShellCmd("sudo swapoff -a && sudo cp /etc/fstab /etc/fstab.old") // Turn off Swap && Backup fstab file
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to disable swap!\n") {
		return err
	}
	utils.WaitPrintf("Modifying fstab")
	// Modify fstab to disable swap permanently
	_, err = utils.ExecShellCmd("sudo sed -i 's/#\\s*\\(.*swap.*\\)/\\1/g' /etc/fstab && sudo sed -i 's/.*swap.*/# &/g' /etc/fstab")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to modify fstab!\n") {
		return err
	}

	// NAT setup
	utils.WaitPrintf("Setting up NAT")
	bashCmd =
		`hostiface=$(sudo route | grep default | tr -s ' ' | cut -d ' ' -f 8) && ` +
			`sudo nft "add table ip filter" && ` +
			`sudo nft "add chain ip filter FORWARD { type filter hook forward priority 0; policy accept; }" && ` +
			`sudo nft "add rule ip filter FORWARD ct state related,established counter accept" && ` +
			`sudo nft "add table ip nat" && ` +
			`sudo nft "add chain ip nat POSTROUTING { type nat hook postrouting priority 0; policy accept; }" && ` +
			`sudo nft "add rule ip nat POSTROUTING oifname ${hostiface} counter masquerade" && `
	_, err = utils.ExecShellCmd(bashCmd)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to set up NAT!\n") {
		return err
	}

	return nil
}

func SetupZipkin() error {
	// Original Bash Scripts: scripts/setup_zipkin.sh

	// Install zipkin pods
	utils.WaitPrintf("Installing zipkin pods")
	_, err := utils.ExecShellCmd("kubectl apply -f %s", configs.Knative.GetIstioZipkinDownloadUrl())
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install zipkin pods!\n") {
		return err
	}

	// Sleep for 10 seconds
	time.Sleep(10 * time.Second)

	// Enable tracing in Knative
	utils.WaitPrintf("Enabling tracing in Knative")
	_, err = utils.ExecShellCmd(`kubectl patch configmap/config-tracing \
-n knative-serving \
--type merge \
-p '{"data":{"backend":"zipkin","zipkin-endpoint":"http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans","debug":"true"}}'`)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to enable tracing in Knative!\n") {
		return err
	}

	return nil
}
