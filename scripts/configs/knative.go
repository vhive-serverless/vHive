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
	LocalRegistryRepoVolumeSize             string
	LocalRegistryVolumeConfigUrl            string
	LocalRegistryDockerRegistryConfigUrl    string
	LocalRegistryHostUpdateConfigUrl        string
	MagicDNSConfigUrl                       string
}

var Knative = KnativeConfigStruct{}

func (knative *KnativeConfigStruct) GetIstioDownloadUrl() string {
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, System.CurrentArch)
}

func (knative *KnativeConfigStruct) GetIstioZipkinDownloadUrl() string {
	return fmt.Sprintf(knative.IstioZipkinDownloadUrlTemplate, knative.IstioZipkinVersion)
}
