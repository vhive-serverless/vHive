package configs

import (
	"fmt"
	"runtime"
)

// System environment struct
type SystemEnvironmentStruct struct {
	GoInstalled                                 bool
	ContainerdInstalled                         bool
	RuncInstalled                               bool
	RunscInstalled                              bool
	CniPluginsInstalled                         bool
	SystemdStartUp                              bool
	GoVersion                                   string
	GoDownloadUrlTemplate                       string
	ContainerdVersion                           string
	ContainerdDownloadUrlTemplate               string
	ContainerdSystemdProfileDownloadUrl         string
	RuncVersion                                 string
	RuncDownloadUrlTemplate                     string
	RunscVersion                                string
	RunscDownloadUrlTemplate                    string
	CniPluginsVersion                           string
	CniPluginsDownloadUrlTemplate               string
	KubectlVersion                              string
	KubeadmVersion                              string
	KubeletVersion                              string
	Dependencies                                string
	TmpDir                                      string
	CurrentOS                                   string
	CurrentArch                                 string
	CurrentDir                                  string
	UserHomeDir                                 string
	DisableAutoUpgradeScriptsDownloadUrl        string
	PmuToolsRepoUrl                             string
	ProtocVersion                               string
	ProtocDownloadUrlTemplate                   string
	SetupFirecrackerContainerdScriptDownloadUrl string
	SetupGvisorContainerdScriptDownloadUrl      string
	SetupSystemScriptDownloadUrl                string
	SetupZipkinScriptDownloadUrl                string
}

// Current system environment
var System = SystemEnvironmentStruct{
	GoInstalled:                          false,
	ContainerdInstalled:                  false,
	RuncInstalled:                        false,
	RunscInstalled:                       false,
	CniPluginsInstalled:                  false,
	SystemdStartUp:                       true,
	GoVersion:                            "1.18.10",
	GoDownloadUrlTemplate:                "https://go.dev/dl/go%s.linux-%s.tar.gz",
	ContainerdVersion:                    "1.6.18",
	ContainerdDownloadUrlTemplate:        "https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-%s.tar.gz",
	ContainerdSystemdProfileDownloadUrl:  "https://raw.githubusercontent.com/containerd/containerd/main/containerd.service",
	RuncVersion:                          "1.1.4",
	RuncDownloadUrlTemplate:              "https://github.com/opencontainers/runc/releases/download/v%s/runc.%s",
	RunscVersion:                         "20210622",
	RunscDownloadUrlTemplate:             "https://storage.googleapis.com/gvisor/releases/release/%s/%s/runsc",
	CniPluginsVersion:                    "1.2.0",
	CniPluginsDownloadUrlTemplate:        "https://github.com/containernetworking/plugins/releases/download/v%s/cni-plugins-linux-%s-v%s.tgz",
	KubectlVersion:                       "1.25.9-00",
	KubeadmVersion:                       "1.25.9-00",
	KubeletVersion:                       "1.25.9-00",
	CurrentOS:                            runtime.GOOS,
	CurrentArch:                          runtime.GOARCH,
	CurrentDir:                           "",
	UserHomeDir:                          "",
	DisableAutoUpgradeScriptsDownloadUrl: "https://raw.githubusercontent.com/vhive-serverless/vHive/main/scripts/utils/disable_auto_updates.sh",
	PmuToolsRepoUrl:                      "https://github.com/vhive-serverless/pmu-tools",
	ProtocVersion:                        "3.19.4",
	ProtocDownloadUrlTemplate:            "https://github.com/protocolbuffers/protobuf/releases/download/v%s/protoc-%s-linux-x86_64.zip",
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

	return fmt.Sprintf(system.RunscDownloadUrlTemplate, system.RuncVersion, unameArch)
}
