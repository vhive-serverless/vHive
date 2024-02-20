// MIT License
//
// # Copyright (c) 2023 Jason Chua, Ruiqi Lai and vHive team
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

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"runtime"

	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

type DemoEnvironment struct {
	CloudYamlFile      string
	EdgeYamlFile       string
	CloudBenchYamlFile string
	EdgeBenchYamlFile  string
	YurtAppSetYamlFile string
	CloudPoolName      string
	EdgePoolName       string
}

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

type KubeConfigStruct struct {
	K8sVersion                string
	AlternativeImageRepo      string
	ApiserverAdvertiseAddress string
	PodNetworkCidr            string
	ApiserverPort             string
	ApiserverToken            string
	ApiserverTokenHash        string
	CalicoVersion             string
}

type SystemEnvironmentStruct struct {
	GoInstalled                         bool
	ContainerdInstalled                 bool
	RuncInstalled                       bool
	CniPluginsInstalled                 bool
	SystemdStartUp                      bool
	NodeHostName                        string
	GoVersion                           string
	GoDownloadUrlTemplate               string
	ContainerdVersion                   string
	ContainerdDownloadUrlTemplate       string
	ContainerdSystemdProfileDownloadUrl string
	RuncVersion                         string
	RuncDownloadUrlTemplate             string
	RunscVersion                        string
	RunscDownloadUrlTemplate            string
	CniPluginsVersion                   string
	CniPluginsDownloadUrlTemplate       string
	KubeVersion                         string
	KubeRepoUrl                         string
	Dependencies                        string
	TmpDir                              string
	CurrentOS                           string
	CurrentArch                         string
	CurrentDir                          string
	UserHomeDir                         string
	PmuToolsRepoUrl                     string
	ProtocVersion                       string
	ProtocDownloadUrlTemplate           string
	YqDownloadUrl                       string
}

type YurtEnvironment struct {
	HelmInstalled                   bool
	HelmPublicSigningKeyDownloadUrl string
	KustomizeInstalled              bool
	KustomizeScriptDownloadUrl      string
	MasterAsCloud                   bool
	WorkerNodeName                  string
	WorkerAsEdge                    bool
	Dependencies                    string
	YurtVersion                     string
}

type VHiveConfigStruct struct {
	FirecrackerKernelImgDownloadUrl string
	StargzVersion                   string
	VHiveRepoPath                   string
	VHiveRepoBranch                 string
	VHiveRepoUrl                    string
	VHiveSetupConfigPath            string
	ForceRemote                     bool
}

// Variables for all configs structure
var Demo = DemoEnvironment{
	CloudYamlFile:      "cloud.yaml",
	EdgeYamlFile:       "edge.yaml",
	CloudBenchYamlFile: "cloudBench.yaml",
	EdgeBenchYamlFile:  "edgeBench.yaml",
	YurtAppSetYamlFile: "yurt.yaml",
	CloudPoolName:      "cloud",
	EdgePoolName:       "edge",
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

var Kube = KubeConfigStruct{}

var System = SystemEnvironmentStruct{
	GoInstalled:         false,
	ContainerdInstalled: false,
	RuncInstalled:       false,
	CniPluginsInstalled: false,
	SystemdStartUp:      true,
	CurrentOS:           runtime.GOOS,
	CurrentArch:         runtime.GOARCH,
	CurrentDir:          "",
	UserHomeDir:         "",
	NodeHostName:        "",
	YqDownloadUrl:       "https://github.com/mikefarah/yq/releases/latest/download/yq_linux_%s",
}

var VHive = VHiveConfigStruct{
	VHiveRepoPath:        ".",
	VHiveRepoBranch:      "main",
	VHiveRepoUrl:         "https://github.com/vhive-serverless/vHive.git",
	VHiveSetupConfigPath: "../../configs/setup",
	ForceRemote:          false,
}

var Yurt = YurtEnvironment{
	HelmInstalled:                   false,
	HelmPublicSigningKeyDownloadUrl: "https://baltocdn.com/helm/signing.asc",
	KustomizeInstalled:              false,
	KustomizeScriptDownloadUrl:      "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh",
	MasterAsCloud:                   true,
	WorkerNodeName:                  "",
	WorkerAsEdge:                    true,
	Dependencies:                    "",
	YurtVersion:                     "1.2.1",
}

// Helper functions to get URL
func (knative *KnativeConfigStruct) GetIstioDownloadUrl() string {
	return fmt.Sprintf(knative.IstioDownloadUrlTemplate, knative.IstioVersion, knative.IstioVersion, System.CurrentArch)
}

func (system *SystemEnvironmentStruct) GetProtocDownloadUrl() string {
	return fmt.Sprintf(system.ProtocDownloadUrlTemplate, system.ProtocVersion, system.ProtocVersion)
}

func (system *SystemEnvironmentStruct) GetContainerdDownloadUrl() string {
	return fmt.Sprintf(system.ContainerdDownloadUrlTemplate, system.ContainerdVersion, system.ContainerdVersion, system.CurrentArch)
}

func (system *SystemEnvironmentStruct) GetRuncDownloadUrl() string {
	return fmt.Sprintf(system.RuncDownloadUrlTemplate, system.RuncVersion, system.CurrentArch)
}

func (system *SystemEnvironmentStruct) GetRunscDownloadUrl() string {
	unameArch := system.CurrentArch
	switch unameArch {
	case "amd64":
		unameArch = "x86_64"
	default:
	}

	return fmt.Sprintf(system.RunscDownloadUrlTemplate, system.RunscVersion, unameArch)
}

// Decode specific config files (JSON format)
func DecodeConfig(configFilePath string, configStruct interface{}) error {
	// Open & read the config file
	configFile, err := os.Open(configFilePath)
	if err != nil {
		return err
	}
	defer configFile.Close()

	// Read file content
	configContent, err := io.ReadAll(configFile)
	if err != nil {
		return err
	}

	// Decode json into struct
	err = json.Unmarshal(configContent, configStruct)

	return err

}

// Load knative config files
func (knative *KnativeConfigStruct) LoadConfig() error {
	var err error
	// Check config directory
	if len(VHive.VHiveSetupConfigPath) == 0 {
		VHive.VHiveSetupConfigPath, err = utils.GetVHiveFilePath("configs/setup")
		if err != nil {
			utils.CleanEnvironment()
			os.Exit(1)
		}
	}
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "knative.json")

	// Decode json into struct
	err = DecodeConfig(configFilePath, knative)

	return err

}

// Load kubernetes config files
func (kube *KubeConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "kube.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, kube)

	return err
}

// Load system config files
func (system *SystemEnvironmentStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "system.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, system)

	return err
}

// Load vHive config files
func (vhive *VHiveConfigStruct) LoadConfig() error {
	// Get the (absolute) path of the config file
	configFilePath := path.Join(VHive.VHiveSetupConfigPath, "vhive.json")

	// Decode json into struct
	err := DecodeConfig(configFilePath, vhive)

	return err

}

const (
	Version = "0.2.4b" // Version Info
)
