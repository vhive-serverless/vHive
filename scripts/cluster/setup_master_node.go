package cluster

import (
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

	err = InstallKnativeServingComponent(stockContainerd)
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
	_, err := utils.ExecShellCmd("kubectl apply -f %s", configs.Kube.PodNetworkAddonConfigURL)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install pod network!\n") {
		return err
	}
	return nil
}

// Install and configure MetalLB
func InstallMetalLB() error {
	utils.WaitPrintf("Installing and configuring MetalLB")
	_, err := utils.ExecShellCmd(`kubectl get configmap kube-proxy -n kube-system -o yaml | sed -e "s/strictARP: false/strictARP: true/" | kubectl apply -f - -n kube-system`)
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl apply -f https://raw.githubusercontent.com/metallb/metallb/v%s/config/manifests/metallb-native.yaml", configs.Knative.MetalLBVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl -n metallb-system wait deploy controller --timeout=90s --for=condition=Available")
	if !utils.CheckErrorWithMsg(err, "Failed to install and configure MetalLB!\n") {
		return err
	}
	for _, value := range configs.Knative.MetalLBConfigURLArray {
		_, err = utils.ExecShellCmd("kubectl apply -f %s", value)
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
	// Update PATH
	err = utils.AppendDirToPath("/usr/local/istio-%s/bin", configs.Knative.IstioVersion)
	if !utils.CheckErrorWithMsg(err, "Failed to update PATH!\n") {
		return err
	}
	// Deploy istio operator
	utils.WaitPrintf("Deploying istio operator")
	operatorConfigPath, err := utils.DownloadToTmpDir(configs.Knative.IstioOperatorConfigUrl)
	if !utils.CheckErrorWithMsg(err, "Failed to deploy istio operator!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("/usr/local/istio-%s/bin/istioctl install -y -f %s", configs.Knative.IstioVersion, operatorConfigPath)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio operator!\n") {
		return err
	}

	return nil
}

// Install Knative Serving component
func InstallKnativeServingComponent(stockContainerd string) error {
	utils.WaitPrintf("Installing Knative Serving component (%s mode)", stockContainerd)
	if stockContainerd == "stock-only" {
		_, err := utils.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-crds.yaml", configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
		_, err = utils.ExecShellCmd("kubectl apply -f https://github.com/knative/serving/releases/download/knative-v%s/serving-core.yaml", configs.Knative.KnativeVersion)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
	} else {
		_, err := utils.ExecShellCmd("kubectl apply -f %s/serving-crds.yaml", configs.Knative.NotStockOnlyKnativeServingYamlUrlPrefix)
		if !utils.CheckErrorWithMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
		_, err = utils.ExecShellCmd("kubectl apply -f %s/serving-core.yaml", configs.Knative.NotStockOnlyKnativeServingYamlUrlPrefix)
		if !utils.CheckErrorWithTagAndMsg(err, "Failed to install Knative Serving component!\n") {
			return err
		}
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
	configFilePath, err := utils.DownloadToTmpDir("%s", configs.Knative.LocalRegistryVolumeConfigUrl)
	if !utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("REPO_VOL_SIZE=%s envsubst < %s | kubectl create --filename -", configs.Knative.LocalRegistryRepoVolumeSize, configFilePath)
	if !utils.CheckErrorWithMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	_, err = utils.ExecShellCmd("kubectl create -f %s && kubectl apply -f %s", configs.Knative.LocalRegistryDockerRegistryConfigUrl, configs.Knative.LocalRegistryHostUpdateConfigUrl)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to install local cluster registry!\n") {
		return err
	}
	return nil
}

// Configure Magic DNS
func ConfigureMagicDNS() error {
	utils.WaitPrintf("Configuring Magic DNS")
	_, err := utils.ExecShellCmd("kubectl apply -f %s", configs.Knative.MagicDNSConfigUrl)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to configure Magic DNS!\n") {
		return err
	}
	return nil
}

// Deploy Istio pods
func DeployIstioPods() error {
	utils.WaitPrintf("Deploying istio pods")
	_, err := utils.ExecShellCmd("kubectl apply -f https://github.com/knative/net-istio/releases/download/knative-v%s/net-istio.yaml", configs.Knative.KnativeVersion)
	if !utils.CheckErrorWithTagAndMsg(err, "Failed to deploy istio pods!\n") {
		return err
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
