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
	"path"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupMasterNode(stockContainerd string) error {
	// Original Bash Scripts: scripts/cluster/setup_master_node.sh

	err := InstallCalico()
	if err != nil {
		return err
	}

	err = InstallMetalLB()
	if err != nil {
		return err
	}

	err = InstallIstio()
	if err != nil {
		return err
	}

	if stockContainerd == "firecracker" {
		err = PatchKnativeForFirecracker()
		if err != nil {
			return err
		}
	}

	err = InstallKnativeServingComponent()
	if err != nil {
		return err
	}

	err = InstallLocalClusterRegistry()
	if err != nil {
		return err
	}

	err = ConfigureMagicDNS()
	if err != nil {
		return err
	}

	err = DeployIstioPods()
	if err != nil {
		return err
	}

	// Logs for verification
	_, err = utils.ExecShellCmd("kubectl get pods -n knative-serving")
	if !utils.CheckErrorWithMsg(err, "Verification Failed!\n") {
		return err
	}

	err = InstallKnativeEventingComponent()
	if err != nil {
		return err
	}

	// Logs for verification
	_, err = utils.ExecShellCmd("kubectl get pods -n knative-eventing")
	if !utils.CheckErrorWithMsg(err, "Verification Failed!") {
		return err
	}

	err = InstallChannelLayer()
	if err != nil {
		return err
	}

	err = InstallBrokerLayer()
	if err != nil {
		return err
	}

	// Logs for verification
	_, err = utils.ExecShellCmd("kubectl --namespace istio-system get service istio-ingressgateway")
	if !utils.CheckErrorWithMsg(err, "Verification Failed!") {
		return err
	}

	return nil
}

// Install Calico network add-on
func InstallCalico() error {
	utils.WaitPrintf("Installing pod network")

	_, err := utils.ExecShellCmd("wget -nc https://raw.githubusercontent.com/projectcalico/calico/v%s/manifests/calico.yaml -P %s",
		configs.Kube.CalicoVersion, path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/calico")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to download stock version of Calico!\n") {
		return err
	}

	_, err = utils.ExecShellCmd(`yq -i '(select (.kind == "DaemonSet" and .metadata.name == "calico-node" and
	.spec.template.spec.containers[].name == "calico-node") |
	.spec.template.spec.containers[].env) += {"name": "IP_AUTODETECTION_METHOD", "value": "kubernetes-internal-ip"}' %s`,
		path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/calico", "calico.yaml")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to patch Calico for IP detection!\n") {
		return err
	}

	utils.WaitPrintf("Configuring Calico to use nftables")
	_, err = utils.ExecShellCmd(`yq -i '(select (.kind == "DaemonSet" and .metadata.name == "calico-node" and
	.spec.template.spec.containers[].name == "calico-node") |
	.spec.template.spec.containers[].env) += {"name": "FELIX_IPTABLESBACKEND", "value": "NFT"}' %s`,
		path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/calico", "calico.yaml")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to patch Calico for nftables!\n") {
		return err
	}

	utils.SuccessPrintf("All nodes are ready!\n")
	_, err = utils.ExecShellCmd(`kubectl apply -f %s`, path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/calico", "calico.yaml")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to apply Calico!\n") {
		return err
	}

	utils.WaitPrintf("Waiting for all nodes to be ready")
	_, err = utils.ExecShellCmd("kubectl wait --for=condition=Ready nodes --all --timeout=600s")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to wait for all nodes to be ready!\n") {
		return err
	}

	return nil
}

// Install and configure MetalLB
func InstallMetalLB() error {
	utils.WaitPrintf("Installing and configuring MetalLB")
	_, err := utils.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml |
	sed -e "s/strictARP: false/strictARP: true/" | kubectl apply -f - -n kube-system`)
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml",
		configs.Knative.MetalLBVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=600s --for=condition=Available")
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}

	metalibConfigsDir := "configs/metallb"
	metalibConfigsList := []string{
		"metallb-ipaddresspool.yaml",
		"metallb-l2advertisement.yaml",
	}

	for _, configFile := range metalibConfigsList {
		metalibConfigPath, err := utils.GetVHiveFilePath(path.Join(metalibConfigsDir, configFile))
		if err != nil {
			return err
		}
		_, err = utils.ExecShellCmd("kubectl apply -f %s", metalibConfigPath)
		if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
			return err
		}
	}
	utils.SuccessPrintf("\n")
	return nil
}

// Install istio
func InstallIstio() error {
	// Install istio
	// Download istio
	utils.WaitPrintf("Downloading istio")
	istioFilePath, err := utils.DownloadToTmpDir(configs.Knative.GetIstioDownloadUrl())
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to download istio!\n") {
		return err
	}
	// Extract istio
	utils.WaitPrintf("Extracting istio")
	err = utils.ExtractToDir(istioFilePath, "/usr/local", true)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to extract istio!\n") {
		return err
	}

	// Grant permissions for other users to use
	_, err = utils.ExecShellCmd("sudo chmod -R o+x /usr/local/istio-%s", configs.Knative.IstioVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to grant permissions to istioctl!\n") {
		return err
	}

	// Update PATH
	err = utils.AppendDirToPath("/usr/local/istio-%s/bin", configs.Knative.IstioVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to update PATH!\n") {
		return err
	}
	// Deploy istio operator
	utils.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := utils.GetVHiveFilePath(configs.Knative.IstioOperatorConfigPath)
	if !utils.CheckErrorWithMsg(err, "Failed to find istio operator config!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("/usr/local/istio-%s/bin/istioctl install -y -f %s", configs.Knative.IstioVersion, operatorConfigPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio operator!\n") {
		return err
	}

	return nil
}

// Change queue-proxy image to accept Firecracker as a runtime
func PatchKnativeForFirecracker() error {
	utils.WaitPrintf("Patching Knative for Firecracker")
	_, err := utils.ExecShellCmd("wget -nc https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml -P %s",
		configs.Knative.KnativeVersion, path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to download stock version of Knative!\n") {
		return err
	}

	_, err = utils.ExecShellCmd(`yq -i '((select (.metadata.labels."app.kubernetes.io/component" == "queue-proxy") | .spec.image),
	(select (.metadata.name == "config-deployment") | .data.queue-sidecar-image)) = "%s"' %s`,
		configs.Knative.QueueProxyImage, path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-core.yaml")))
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to patch Knative for Firecracker!\n") {
		return err
	}

	return nil
}

// Install Knative Serving component
func InstallKnativeServingComponent() error {
	utils.WaitPrintf("Installing Knative Serving component")

	if _, err := os.Stat(path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-crds.yaml"))); err == nil {
		utils.WaitPrintf("Found local serving-crds.yaml. Using it instead of stock version of knative.")
		servingCrdsPath := path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-crds.yaml"))
		_, err = utils.ExecShellCmd("kubectl apply -f %s", servingCrdsPath)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	} else {
		utils.WaitPrintf("Using stock version of knative's crds.")
		_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml",
			configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	}

	if _, err := os.Stat(path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-core.yaml"))); err == nil {
		utils.WaitPrintf("Found local serving-core.yaml. Using it instead of stock version of knative.")
		servingCorePath := path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-core.yaml"))
		_, err = utils.ExecShellCmd("kubectl apply -f %s", servingCorePath)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	} else {
		utils.WaitPrintf("Using stock version of knative's core.")
		_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml",
			configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	}

	_, err := utils.ExecShellCmd("kubectl -n knative-serving wait deploy webhook --timeout=180s --for=condition=Available")
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
		return err
	}

	return nil
}

// Install local cluster registry
func InstallLocalClusterRegistry() error {
	utils.WaitPrintf("Installing local cluster registry")
	_, err := utils.ExecShellCmd("kubectl create namespace registry")
	if !utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	configFilePath, err := utils.GetVHiveFilePath(configs.Knative.LocalRegistryVolumeConfigPath)
	if !utils.CheckErrorWithMsg(err, "Failed to find local cluster registry config!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -", configs.Knative.LocalRegistryRepoVolumeSize, configFilePath)
	if !utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	dockerRegistryConfigPath, err := utils.GetVHiveFilePath(configs.Knative.LocalRegistryDockerRegistryConfigPath)
	if !utils.CheckErrorWithMsg(err, "Failed to find local cluster registry config!\n") {
		return err
	}
	hostUpdateConfigPath, err := utils.GetVHiveFilePath(configs.Knative.LocalRegistryHostUpdateConfigPath)
	if !utils.CheckErrorWithMsg(err, "Failed to find local cluster registry config!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s", dockerRegistryConfigPath, hostUpdateConfigPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	return nil
}

// Configure Magic DNS
func ConfigureMagicDNS() error {
	if _, err := os.Stat(path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "serving-default-domain.yaml"))); err == nil {
		utils.WaitPrintf("Found local serving-default-domain.yaml. Using it instead of stock version of knative.")
		servingDNSPath, err := utils.GetVHiveFilePath(path.Join("configs/knative_yamls", "serving-default-domain.yaml"))
		if err != nil {
			return err
		}
		_, err = utils.ExecShellCmd("kubectl apply -f %s", servingDNSPath)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	} else {
		utils.WaitPrintf("Using stock version of knative's magic DNS.")
		_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-default-domain.yaml",
			configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	}

	return nil
}

// Deploy Istio pods
func DeployIstioPods() error {
	utils.WaitPrintf("Deploying istio pods")

	if _, err := os.Stat(path.Join(configs.VHive.VHiveRepoPath, path.Join("configs/knative_yamls", "net-istio.yaml"))); err != nil {
		_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative-extensions/net-istio/releases/download/knative-v%s/net-istio.yaml",
			configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio pods!\n") {
			return err
		}
	} else {
		loaderIstioControllerPath, err := utils.GetVHiveFilePath(path.Join("configs/knative_yamls", "net-istio.yaml"))
		if err != nil {
			return err
		}
		_, err = utils.ExecShellCmd("kubectl apply --filename %s", loaderIstioControllerPath)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio pods!\n") {
			return err
		}
	}

	return nil
}

// Install Knative Eventing component
func InstallKnativeEventingComponent() error {
	utils.WaitPrintf("Installing Knative Eventing component")
	_, err := utils.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-crds.yaml", configs.Knative.KnativeVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to install Knative Eventing component!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/eventing-core.yaml", configs.Knative.KnativeVersion)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Eventing component!\n") {
		return err
	}
	return nil
}

// Install a default Channel (messaging) layer
func InstallChannelLayer() error {
	utils.WaitPrintf("Installing a default Channel (messaging) layer")
	_, err := utils.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/in-memory-channel.yaml", configs.Knative.KnativeVersion)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install a default Channel (messaging) layer!\n") {
		return err
	}
	return nil
}

// Install a Broker layer
func InstallBrokerLayer() error {
	utils.WaitPrintf("Installing a Broker layer")
	_, err := utils.ExecShellCmd("kubectl apply -f https://github.com/knative/eventing/releases/download/knative-v%s/mt-channel-broker.yaml", configs.Knative.KnativeVersion)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install a Broker layer!\n") {
		return err
	}
	return nil
}
