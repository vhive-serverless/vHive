package cluster

import (
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupWorkerKubelet(stockContainerd string) error {
	var criSock string
	if stockContainerd == "stock-only" {
		criSock = "/run/containerd/containerd.sock"
	} else {
		criSock = "/etc/vhive-cri/vhive-cri.sock"
	}

	err := CreateWorkerKubeletService(criSock)
	if err != nil {
		return err
	}

	return nil
}

// Create kubelet service on worker node
func CreateWorkerKubeletService(criSock string) error {
	utils.WaitPrintf("Creating kubelet service")
	bashCmd := `sudo sh -c 'cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf
[Service]                                                 
Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --runtime-request-timeout=15m --container-runtime-endpoint=unix://'%s'"
EOF'`
	_, err := utils.ExecShellCmd(bashCmd, criSock)
	if !utils.CheckErrorWithMsg(err, "Failed to create kubelet service!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("sudo systemctl daemon-reload")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create kubelet service!\n") {
		return err
	}

	return nil
}
