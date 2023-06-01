package configs

import "fmt"

type KnativeConfigStruct struct {
	KnativeVersion                          string
	KnativeCLIRepoUrl                       string
	KnativeCLIBranch                        string
	NotStockOnlyKnativeServingYamlUrlPrefix string
	IstioVersion                            string
	IstioDownloadUrlTemplate                string
	IstioOperatorConfigUrl                  string
	IstioZipkinVersion                      string
	IstioZipkinDownloadUrlTemplate          string
	MetalLBVersion                          string
	MetalLBConfigURLArray                   []string
	LocalRegistryRepoVolumeSize             string
	LocalRegistryVolumeConfigUrl            string
	LocalRegistryDockerRegistryConfigUrl    string
	LocalRegistryHostUpdateConfigUrl        string
	MagicDNSConfigUrl                       string
	VHiveMode                               bool
}

var Knative = KnativeConfigStruct{
	KnativeVersion:                          "1.9.2",
	KnativeCLIBranch:                        "release-1.9",
	KnativeCLIRepoUrl:                       "https://github.com/knative/client.git",
	NotStockOnlyKnativeServingYamlUrlPrefix: "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/knative_yamls",
	IstioVersion:                            "1.16.3",
	IstioOperatorConfigUrl:                  "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/istio/istio-minimal-operator.yaml",
	IstioDownloadUrlTemplate:                "https://github.com/istio/istio/releases/download/%s/istio-%s-linux-%s.tar.gz",
	IstioZipkinVersion:                      "1.16",
	IstioZipkinDownloadUrlTemplate:          "https://raw.githubusercontent.com/istio/istio/release-%s/samples/addons/extras/zipkin.yaml",
	MetalLBVersion:                          "0.13.9",
	MetalLBConfigURLArray: []string{
		"https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/metallb/metallb-ipaddresspool.yaml",
		"https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/metallb/metallb-l2advertisement.yaml"},
	LocalRegistryRepoVolumeSize:          "5Gi",
	LocalRegistryVolumeConfigUrl:         "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/repository-volume.yaml",
	LocalRegistryDockerRegistryConfigUrl: "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/docker-registry.yaml",
	LocalRegistryHostUpdateConfigUrl:     "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/registry/repository-update-hosts.yaml",
	MagicDNSConfigUrl:                    "https://raw.githubusercontent.com/vhive-serverless/vHive/main/configs/knative_yamls/serving-default-domain.yaml",
	VHiveMode:                            true,
}

func (knative *KnativeConfigStruct) GetIstioDownloadUrl() string {
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, System.CurrentArch)
}

func (knative *KnativeConfigStruct) GetIstioZipkinDownloadUrl() string {
	return fmt.Sprintf(knative.IstioZipkinDownloadUrlTemplate, knative.IstioZipkinVersion)
}
