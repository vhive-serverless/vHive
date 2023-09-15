package configs

import "fmt"

type KnativeConfigStruct struct {
	KnativeVersion                       string
	IstioVersion                         string
	IstioDownloadUrlTemplate             string
	IstioOperatorConfigUrl               string
	MetalLBVersion                       string
	MetalLBConfigURLArray                []string
	LocalRegistryRepoVolumeSize          string
	LocalRegistryVolumeConfigUrl         string
	LocalRegistryDockerRegistryConfigUrl string
	LocalRegistryHostUpdateConfigUrl     string
	MagicDNSConfigUrl                    string
	VHiveMode                            bool
}

var Knative = KnativeConfigStruct{
	IstioOperatorConfigUrl:   "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/istio/istio-minimal-operator.yaml",
	IstioDownloadUrlTemplate: "https://github.com/istio/istio/releases/download/%s/istio-%s-linux-%s.tar.gz",
	MetalLBConfigURLArray: []string{
		"https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/metallb/metallb-ipaddresspool.yaml",
		"https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/metallb/metallb-l2advertisement.yaml"},
	LocalRegistryVolumeConfigUrl:         "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/repository-volume.yaml",
	LocalRegistryDockerRegistryConfigUrl: "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/docker-registry.yaml",
	LocalRegistryHostUpdateConfigUrl:     "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/repository-update-hosts.yaml",     //TODO: uses path
	MagicDNSConfigUrl:                    "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/knative_yamls/serving-default-domain.yaml", //TODO: uses path
	VHiveMode:                            true,
}

func (knative *KnativeConfigStruct) GetIstioDownloadUrl() string {
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, System.CurrentArch)
}
