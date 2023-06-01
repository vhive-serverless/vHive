package utils

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
)

// func CheckGoInstalled() (bool, error) {

// }

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
	_, err = ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.bashrc", appendedPath, configs.System.UserHomeDir)
	if err != nil {
		return err
	}
	// For zsh
	_, err = exec.LookPath("zsh")
	if err != nil {
		_, err = ExecShellCmd("echo 'export PATH=$PATH:%s' >> %s/.zshrc", appendedPath, configs.System.UserHomeDir)
	}

	return err
}

// Download file to temporary directory (absolute path of downloaded file will be the first return value if successful)
func DownloadToTmpDir(urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	fileName := path.Base(url)
	filePath := configs.System.TmpDir + "/" + fileName
	// Create temporary directory (if not exist)
	err := CreateTmpDir()
	if err != nil {
		return filePath, err
	}
	// Download file
	_, err = ExecShellCmd("curl -sSL --output %s %s", filePath, url)
	return filePath, err
}

func CloneRepoToTmpDir(branch string, urlTemplate string, pars ...any) (string, error) {
	url := fmt.Sprintf(urlTemplate, pars...)
	repoName := strings.TrimSuffix(path.Base(url), ".git")
	repoPath := configs.System.TmpDir + "/" + repoName
	// Create temporary directory (if not exist)
	err := CreateTmpDir()
	if err != nil {
		return repoPath, err
	}
	// Clone repo
	_, err = ExecShellCmd("git clone --quiet -c advice.detachedHead=false --branch %s %s %s", branch, url, repoPath)
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
		if strings.HasSuffix(fileExtName, ".tar.gz") {
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
func DownloadAndExecScript(scriptUrlTemplate string, pars ...any) error {
	scriptUrl := fmt.Sprintf(scriptUrlTemplate, pars...)
	// Create temporary directory (if not exist)
	err := CreateTmpDir()
	if err != nil {
		return err
	}
	// Download bash script
	scriptPath, err := DownloadToTmpDir(scriptUrl)
	if err == nil {
		// Execute the script
		_, err = ExecShellCmd("bash %s", scriptPath)
	}
	return err
}

// Turn off unattended-upgrades
func TurnOffAutomaticUpgrade() error {
	switch configs.System.CurrentOS {
	case "ubuntu":
		// Execute vHive bash script to disable auto update on Ubuntu
		err := DownloadAndExecScript(configs.System.DisableAutoUpgradeScriptsDownloadUrl)
		return err
	default:
		return nil
	}
}
