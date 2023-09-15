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
	KubeVersion                         string
	Dependencies                        string
	TmpDir                              string
	CurrentOS                           string
	CurrentArch                         string
	CurrentDir                          string
	UserHomeDir                         string
	NodeHostName                        string
}

// Current system environment
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
}
