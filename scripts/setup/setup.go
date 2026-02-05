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
	"path"
	"time"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Set up firecracker containerd
func SetupFirecrackerContainerd() error {
	_, err := utils.ExecShellCmd(
		"sudo mkdir -p /etc/firecracker-containerd" +
			" && sudo mkdir -p /var/lib/firecracker-containerd/runtime" +
			" && sudo mkdir -p /etc/containerd/")
	if err != nil {
		return err
	}

	// Create snapshot base directory
	_, err = utils.ExecShellCmd("sudo mkdir -p /fccd && sudo chmod 777 /fccd")
	if err != nil {
		return err
	}

	// Pull LFS in vHive
	utils.WaitPrintf("Pulling LFS in vHive")
	if err := utils.CheckVHiveRepo(); !utils.CheckErrorWithMsg(err, "Failed to pull LFS in vHive!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("cd %s && git lfs pull", configs.VHive.VHiveRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to pull LFS in vHive!\n") {
		return err
	}

	utils.WaitPrintf("Copying required files")
	dstDir := "/usr/local/bin"
	binsDir := "bin"
	configsDir := "configs/firecracker-containerd"

	binLists := []string{
		"firecracker",
		"jailer",
		"containerd-shim-aws-firecracker",
		"firecracker-containerd",
		"firecracker-ctr",
	}
	for _, bin := range binLists {
		src, err := utils.GetVHiveFilePath(path.Join(binsDir, bin))
		if !utils.CheckErrorWithMsg(err, "Failed to find required file: <%s>!\n", bin) {
			return err
		}
		err = utils.CopyToDir(src, dstDir, true)
		if !utils.CheckErrorWithMsg(err, "Failed to copy required file: <%s>!\n", bin) {
			return err
		}
	}

	// rootfs image
	// https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/remote-snapshotter-getting-started.md#build-a-firecracker-rootfs-with-a-remote-snapshotter
	rootfsImgPath, err := utils.GetVHiveFilePath(path.Join(binsDir, "default-rootfs.img"))
	if !utils.CheckErrorWithMsg(err, "Failed to find rootfs image!\n") {
		return err
	}
	err = utils.CopyToDir(rootfsImgPath, "/var/lib/firecracker-containerd/runtime/default-rootfs.img", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy rootfs image!\n") {
		return err
	}

	// kernel image
	// https://github.com/firecracker-microvm/firecracker-containerd/blob/main/docs/remote-snapshotter-getting-started.md#build-a-linux-kernel-with-fuse-support
	kernelImgPath, err := utils.GetVHiveFilePath(path.Join(binsDir, "vmlinux-6.1.141"))
	if !utils.CheckErrorWithMsg(err, "Failed to find kernel image!\n") {
		return err
	}
	err = utils.CopyToDir(kernelImgPath, "/var/lib/firecracker-containerd/runtime/hello-vmlinux.bin", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy kernel image!\n") {
		return err
	}

	// Copy config.toml
	configFilePath, err := utils.GetVHiveFilePath(path.Join(configsDir, "config.toml"))
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}
	err = utils.CopyToDir(configFilePath, "/etc/firecracker-containerd/", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}

	// When executed inside a docker container, this command returns the container ID of the container.
	// on a non container environment, this returns "/".
	containerId, err := utils.ExecShellCmd("basename $(cat /proc/1/cpuset)")
	if err != nil {
		return err
	}
	if len(containerId) == 64 {
		// Inside a container
		_, err = utils.ExecShellCmd(`sudo sed -i "s/fc-dev-thinpool/%s_thinpool/" /etc/firecracker-containerd/config.toml`, containerId)
		if err != nil {
			return err
		}
	}

	// Copy `firecracker-runtime.json`
	configFilePath, err = utils.GetVHiveFilePath(path.Join(configsDir, "firecracker-runtime.json"))
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}
	err = utils.CopyToDir(configFilePath, "/etc/containerd/", true)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to copy required files!\n") {
		return err
	}

	return nil
}

// Set up gvisor containerd
func SetupGvisorContainerd() error {
	// Create required directory
	_, err := utils.ExecShellCmd("sudo mkdir -p /etc/gvisor-containerd && sudo mkdir -p /etc/cni/net.d")
	if err != nil {
		return err
	}

	// Pull LFS in vHive
	utils.WaitPrintf("Pulling LFS in vHive")
	if err := utils.CheckVHiveRepo(); !utils.CheckErrorWithMsg(err, "Failed to pull LFS in vHive!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("cd %s && git lfs pull", configs.VHive.VHiveRepoPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to pull LFS in vHive!\n") {
		return err
	}

	// Copy various required files
	utils.WaitPrintf("Copying required files")
	dstDir := "/usr/local/bin"
	binsDir := "bin"
	ctrdConfigsDir := "configs/gvisor-containerd"
	cniConfigsDir := "configs/cni"

	binLists := []string{
		"containerd-shim-runsc-v1",
		"gvisor-containerd",
	}
	for _, bin := range binLists {
		src, err := utils.GetVHiveFilePath(path.Join(binsDir, bin))
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
		err = utils.CopyToDir(src, dstDir, true)
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
	}

	configFilePath, err := utils.GetVHiveFilePath(path.Join(ctrdConfigsDir, "config.toml"))
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}
	err = utils.CopyToDir(configFilePath, "/etc/gvisor-containerd/", true)
	if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
		return err
	}

	dstDir = "/etc/cni/net.d"
	configLists := []string{
		"10-bridge.conf",
		"99-loopback.conf",
	}
	for _, configFile := range configLists {
		src, err := utils.GetVHiveFilePath(path.Join(cniConfigsDir, configFile))
		if !utils.CheckErrorWithMsg(err, "Failed to copy required files!\n") {
			return err
		}
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
	err := utils.InstallPackages("curl ca-certificates screen")
	if !utils.CheckErrorWithMsg(err, "Failed to install required dependencies!\n") {
		return err
	}
	// Add to apt repo
	_, err = utils.ExecShellCmd("sudo add-apt-repository -y universe")
	if !utils.CheckErrorWithMsg(err, "Failed to install required dependencies!\n") {
		return err
	}
	err = utils.InstallPackages("apt-transport-https gcc g++ make acl net-tools git-lfs bc gettext-base jq dmsetup gnupg-agent software-properties-common iproute2 nftables git-lfs")
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

	return nil
}

// Original Bash Scripts: scripts/setup_zipkin.sh
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
	_, err = utils.ExecShellCmd(
		`kubectl patch configmap/config-tracing` +
			` -n knative-serving` +
			` --type merge` +
			` -p '{"data":{"backend":"zipkin","zipkin-endpoint":"http://zipkin.istio-system.svc.cluster.local:9411/api/v2/spans","debug":"true"}}'`)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to enable tracing in Knative!\n") {
		return err
	}

	return nil
}
