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

package utils

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
)

// Append directory to current PATH variable
func AppendDirToPathVariable(pathTemplate string, pars ...any) error {
	oldPath := GetEnvironmentVariable("PATH")
	appendedPath := fmt.Sprintf(pathTemplate, pars...)
	_, err := UpdateEnvironmentVariable("PATH", "%s:%s", oldPath, appendedPath)
	return err
}

// Append directory to PATH variable for bash & zsh
func AppendDirToPath(pathTemplate string, pars ...any) error {
	appendedPath := fmt.Sprintf(pathTemplate, pars...)

	// Update PATH
	err := AppendDirToPathVariable(appendedPath)
	if err != nil {
		return err
	}

	// For bash
	if _, err = ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.bashrc", appendedPath, configs.System.UserHomeDir); err != nil {
		return err
	}
	// For zsh
	_, err = exec.LookPath("zsh")
	if err != nil {
		_, err = ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.zshrc", appendedPath, configs.System.UserHomeDir)
		if err != nil {
			return err
		}
	}
	// For other users
	_, err = ExecShellCmd("echo 'export PATH=$PATH:%s' | sudo tee -a /etc/profile", appendedPath)

	return err
}

// Download file to temporary directory (absolute path of downloaded file will be the first return value if successful)
func DownloadToTmpDir(urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	fileName := path.Base(url)
	filePath := path.Join(configs.System.TmpDir, fileName)
	// Create temporary directory (if not exist)
	if err := CreateTmpDir(); err != nil {
		return filePath, err
	}
	// Download file
	_, err := ExecShellCmd("curl -sSL --output %s %s", filePath, url)
	return filePath, err
}

// Clone git repo to temporary directory (absolute path of cloned repo will be the first return value if successful)
func CloneRepoToTmpDir(branch string, urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	repoName := strings.TrimSuffix(path.Base(url), ".git")
	repoPath := path.Join(configs.System.TmpDir, repoName)
	// Create temporary directory (if not exist)
	if err := CreateTmpDir(); err != nil {
		return repoPath, err
	}
	// Clone repo
	_, err := ExecShellCmd("git clone --quiet --recurse-submodules -c advice.detachedHead=false --branch %s %s %s", branch, url, repoPath)
	return repoPath, err
}

// Extract archive file to specific directory(currently support .tar.gz, .gz, .tgz, .zip file only)
func ExtractToDir(archiveFilePath string, dirPath string, privileged bool) error {
	var err error

	// Privileged mode, use sudo
	privilegedCmd := ""
	if privileged {
		privilegedCmd = "sudo"
	}

	// Get file extension name
	fileExtName := filepath.Ext(archiveFilePath)
	switch fileExtName {
	case ".zip":
		// Extract `zip` file
		_, err = ExecShellCmd("%s unzip -o -q %s -d %s", privilegedCmd, archiveFilePath, dirPath)
	case ".gz":
		if strings.HasSuffix(archiveFilePath, ".tar.gz") {
			// Extract `tar.gz` file
			_, err = ExecShellCmd("%s tar -xzvf %s -C %s", privilegedCmd, archiveFilePath, dirPath)
		} else {
			// Extract `gz` file
			_, err = ExecShellCmd("%s gzip -d %s -C %s", privilegedCmd, archiveFilePath, dirPath)
		}
	case ".tgz":
		// Extract `tgz` file
		_, err = ExecShellCmd("%s tar -xzvf %s -C %s", privilegedCmd, archiveFilePath, dirPath)
	default:
		return &ShellError{Msg: "Unsupported format!", ExitCode: 1}
	}

	return err
}

// Download and execute remote bash script
func DownloadAndExecScript(scriptUrl string, scriptPars ...string) error {
	// Create temporary directory (if not exist)
	err := CreateTmpDir()
	if err != nil {
		return err
	}
	// Combine all script parameters
	combinedScriptPars := ""
	for _, scriptPar := range scriptPars {
		combinedScriptPars = combinedScriptPars + " " + scriptPar
	}
	// Download bash script
	scriptPath, err := DownloadToTmpDir(scriptUrl)
	if err == nil {
		// Execute the script
		_, err = ExecShellCmd("bash %s %s", scriptPath, combinedScriptPars)
	}
	return err
}

// Turn off unattended-upgrades
func TurnOffAutomaticUpgrade() error {
	switch configs.System.CurrentOS {
	case "ubuntu":
		// Execute vHive bash script to disable auto update on Ubuntu
		WaitPrintf("Turning off automatic update")
		_, err := ExecVHiveBashScript("scripts/utils/disable_auto_updates.sh")
		CheckErrorWithTagAndMsg(err, "Failed to turn off automatic update!\n")
		return err
	default:
		return nil
	}
}

func InstallYQ() {
	WaitPrintf("Downloading yq for yaml parsing of template")
	yqUrl := fmt.Sprintf(configs.System.YqDownloadUrlTemplate, configs.System.CurrentArch)
	_, err := ExecShellCmd(`sudo wget %s -O /usr/bin/yq && sudo chmod +x /usr/bin/yq`, yqUrl)
	CheckErrorWithTagAndMsg(err, "Failed to add yq!\n")
}
