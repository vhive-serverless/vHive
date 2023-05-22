package configs

import (
	"runtime"
)

// System environment struct
type SystemEnvironmentStruct struct {
	GoInstalled                         bool
	ContainerdInstalled                 bool
	RuncInstalled                       bool
	CniPluginsInstalled                 bool
	SystemdStartUp                      bool
	GoVersion                           string
	GoDownloadUrlTemplate               string
	ContainerdVersion                   string
	ContainerdDownloadUrlTemplate       string
	ContainerdSystemdProfileDownloadUrl string
	RuncVersion                         string
	RuncDownloadUrlTemplate             string
	CniPluginsVersion                   string
	CniPluginsDownloadUrlTemplate       string
	KubectlVersion                      string
	KubeadmVersion                      string
	KubeletVersion                      string
	Dependencies                        string
	TmpDir                              string
	CurrentOS                           string
	CurrentArch                         string
	CurrentDir                          string
	UserHomeDir                         string
}

// Current system environment
var System = SystemEnvironmentStruct{
	GoInstalled:                         false,
	ContainerdInstalled:                 false,
	RuncInstalled:                       false,
	CniPluginsInstalled:                 false,
	SystemdStartUp:                      true,
	GoVersion:                           "1.18.10",
	GoDownloadUrlTemplate:               "https://go.dev/dl/go%s.linux-%s.tar.gz",
	ContainerdVersion:                   "1.6.18",
	ContainerdDownloadUrlTemplate:       "https://github.com/containerd/containerd/releases/download/v%s/containerd-%s-linux-%s.tar.gz",
	ContainerdSystemdProfileDownloadUrl: "https://raw.githubusercontent.com/containerd/containerd/main/containerd.service",
	RuncVersion:                         "1.1.4",
	RuncDownloadUrlTemplate:             "https://github.com/opencontainers/runc/releases/download/v%s/runc.%s",
	CniPluginsVersion:                   "1.2.0",
	CniPluginsDownloadUrlTemplate:       "https://github.com/containernetworking/plugins/releases/download/v%s/cni-plugins-linux-%s-v%s.tgz",
	KubectlVersion:                      "1.25.9-00",
	KubeadmVersion:                      "1.25.9-00",
	KubeletVersion:                      "1.25.9-00",
	CurrentOS:                           runtime.GOOS,
	CurrentArch:                         runtime.GOARCH,
	CurrentDir:                          "",
	UserHomeDir:                         "",
}
