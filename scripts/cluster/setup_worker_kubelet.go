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
	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupWorkerKubelet(stockContainerd string) error {
	var criSock string
	if stockContainerd == "stock-only" {
		criSock = "/run/containerd/containerd.sock"
	} else {
		criSock = "/etc/vhive-cri/vhive-cri.sock"
	}

	if err := CreateWorkerKubeletService(criSock); err != nil {
		return err
	}

	return nil
}

// Create kubelet service on worker node
func CreateWorkerKubeletService(criSock string) error {
	utils.WaitPrintf("Creating kubelet service")
	// Create service directory if not exist
	_, err := utils.ExecShellCmd("sudo mkdir -p /etc/systemd/system/kubelet.service.d")
	if !utils.CheckErrorWithMsg(err, "Failed to create kubelet service!\n") {
		return err
	}
	bashCmd := "sudo sh -c 'cat <<EOF > /etc/systemd/system/kubelet.service.d/0-containerd.conf\n" +
		"[Service]\n" +
		`Environment="KUBELET_EXTRA_ARGS=--container-runtime=remote --v=%d --runtime-request-timeout=15m --container-runtime-endpoint=unix://%s"` +
		"\nEOF'"

	_, err = utils.ExecShellCmd(bashCmd, configs.System.LogVerbosity, criSock)
	if !utils.CheckErrorWithMsg(err, "Failed to create kubelet service!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("sudo systemctl daemon-reload")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to create kubelet service!\n") {
		return err
	}

	return nil
}
