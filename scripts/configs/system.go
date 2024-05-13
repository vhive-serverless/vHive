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

package configs

import (
	"fmt"
	"runtime"
)

// System environment struct
type SystemEnvironmentStruct struct {
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
	LogVerbosity                        int
	YqDownloadUrlTemplate               string
	HelmDownloadUrl                     string
	MinIOVersion                        string
	MinIOValuePath                      string
}

// Current system environment
var System = SystemEnvironmentStruct{
	CurrentOS:   runtime.GOOS,
	CurrentArch: runtime.GOARCH,
	CurrentDir:  "",
	UserHomeDir: "",
}

func (system *SystemEnvironmentStruct) GetProtocDownloadUrl() string {
	unameArch := system.CurrentArch
	switch unameArch {
	case "amd64":
		unameArch = "x86_64"
	case "arm64":
		unameArch = "aarch_64"
	default:
	}
	return fmt.Sprintf(system.ProtocDownloadUrlTemplate, system.ProtocVersion, system.ProtocVersion, unameArch)
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
	case "arm64":
		unameArch = "aarch_64"
	default:
	}

	return fmt.Sprintf(system.RunscDownloadUrlTemplate, system.RunscVersion, unameArch)
}
