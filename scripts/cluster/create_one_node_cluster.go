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
		_, err = utils.ExecShellCmd(`CRI_SOCK=%s envsubst < "/scripts/kubeadm.conf" > "/scripts/kubeadm_patched.conf" && sudo kubeadm init --skip-phases="preflight" --config="/scripts/kubeadm_patched.conf"`, criSock)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to create cluster using the config file!\n") {
			return err
		}
	} else {
		// On a non container environment
		utils.WaitPrintf("Creating cluster")
		_, err = utils.ExecShellCmd(`sudo kubeadm init --ignore-preflight-errors=all --cri-socket %s --pod-network-cidr=%s`, criSock, configs.Kube.PodNetworkCidr)
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
