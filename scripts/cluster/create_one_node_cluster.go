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

package cluster

import (
	"os"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func CreateOneNodeCluster(stockContainerd string) error {
	// Original Bash Scripts: scripts/cluster/create_one_node_cluster.sh

	err := SetupWorkerKubelet(stockContainerd)
	if err != nil {
		return err
	}

	var criSock string

	if stockContainerd == "stock-only" {
		criSock = "/run/containerd/containerd.sock"
	} else {
		criSock = "/etc/vhive-cri/vhive-cri.sock"
	}

	err = CreateOneNodeKubernetes(criSock)
	if err != nil {
		return err
	}

	err = KubectlForNonRoot()
	if err != nil {
		return err
	}

	// if the user is root, export KUBECONFIG as $HOME is different for root user and /etc is readable
	eUserId := os.Geteuid()
	if eUserId == 0 {
		// root user
		_, err = utils.UpdateEnvironmentVariable("KUBECONFIG", "/etc/kubernetes/admin.conf")
		if err != nil {
			return err
		}
	}

	err = UntaintMaster()
	if err != nil {
		return err
	}

	err = SetupMasterNode(stockContainerd)
	if err != nil {
		return err
	}

	return nil
}

// Deploy one node kubernetes cluster
func CreateOneNodeKubernetes(criSock string) error {
	// When executed inside a docker container, this command returns the container ID of the container.
	// on a non container environment, this returns "/".
	containerId, err := utils.ExecShellCmd("basename $(cat /proc/1/cpuset)")
	if err != nil {
		return err
	}

	if len(containerId) == 64 {
		// Inside a docker, create cluster using the config file
		utils.WaitPrintf("Creating cluster using the config file")
		_, err = utils.ExecShellCmd(`CRI_SOCK=%s envsubst < "/scripts/kubeadm.conf" > "/scripts/kubeadm_patched.conf" && sudo kubeadm init --kubernetes-version %s --skip-phases="preflight" --config="/scripts/kubeadm_patched.conf"`, criSock, configs.Kube.K8sVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to create cluster using the config file!\n") {
			return err
		}
	} else {
		// On a non container environment
		utils.WaitPrintf("Creating cluster")
		_, err = utils.ExecShellCmd(`sudo kubeadm init --ignore-preflight-errors=all --cri-socket unix://%s --pod-network-cidr=%s --kubernetes-version %s`, criSock, configs.Kube.PodNetworkCidr, configs.Kube.K8sVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to create cluster!\n") {
			return err
		}
	}

	return nil
}

// Untaint master (allow pods to be scheduled on master)
func UntaintMaster() error {
	utils.WaitPrintf("Untainting master")
	_, err := utils.ExecShellCmd("kubectl taint nodes --all node-role.kubernetes.io/control-plane-")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to untaint master!\n") {
		return err
	}
	return nil
}
