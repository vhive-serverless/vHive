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
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
)

// Implement error interface of ShellError
type ShellError struct {
	Msg      string
	ExitCode int
}

func (err *ShellError) Error() string {
	return fmt.Sprintf("[exit %d] -> %s", err.ExitCode, err.Msg)
}

// Execute Shell Command
func ExecShellCmd(cmd string, pars ...any) (string, error) {
	// Allocate bytes buffer
	bashCmd := new(bytes.Buffer)
	cmdStdout := new(bytes.Buffer)
	cmdStderr := new(bytes.Buffer)
	fmt.Fprintf(bashCmd, cmd, pars...)
	bashProcess := exec.Command("bash", "-c", bashCmd.String())
	// Redirect stdout & stderr
	bashProcess.Stdout = cmdStdout
	bashProcess.Stderr = cmdStderr

	// Execute command
	err := bashProcess.Run()

	// remove suffix "\n" in Stdout & Stderr
	var trimmedStdout string
	var trimmedStderr string
	if cmdStdout.Len() > 0 {
		trimmedStdout = strings.TrimSuffix(cmdStdout.String(), "\n")
	} else {
		trimmedStdout = ""
	}
	if cmdStderr.Len() > 0 {
		trimmedStderr = strings.TrimSuffix(cmdStderr.String(), "\n")
	} else {
		trimmedStderr = ""
	}

	// Rewrite error message
	if err != nil {
		err = &ShellError{Msg: trimmedStderr, ExitCode: bashProcess.ProcessState.ExitCode()}
	}

	// For logs
	if CommonLog != nil {
		CommonLog.Printf("Executing shell command: %s\n", bashCmd.String())
		CommonLog.Printf("Stdout from shell:\n%s\n", trimmedStdout)
	}
	if ErrorLog != nil {
		ErrorLog.Printf("Executing shell command: %s\n", bashCmd.String())
		ErrorLog.Printf("Stderr from shell:\n%s\n", trimmedStderr)
	}

	return trimmedStdout, err
}

// Detect current architecture
func DetectArch() error {
	switch configs.System.CurrentArch {
	case "amd64":
	default:
		// Only amd64(x86_64) are supported at present
		FatalPrintf("Unsupported architecture: %s\n", configs.System.CurrentArch)
		return &ShellError{"Unsupported architecture", 1}
	}
	InfoPrintf("Detected Arch: %s\n", configs.System.CurrentArch)
	return nil
}

// Detect current operating system
func DetectOS() error {
	switch configs.System.CurrentOS {
	case "windows":
		// Windows is not supported to use vHive
		FatalPrintf("Unsupported OS: %s\n", configs.System.CurrentOS)
		return &ShellError{"Unsupported OS", 1}
	default:
		var err error
		// Get Linux distribution info (some bash trick)
		configs.System.CurrentOS, err = ExecShellCmd("sed -n 's/^NAME=\"\\(.*\\)\"/\\1/p' < /etc/os-release | head -1 | tr '[:upper:]' '[:lower:]'")
		// Failed to get Linux distribution info
		if !CheckErrorWithMsg(err, "Failed to get Linux distribution info!\n") {
			return err
		}
		switch configs.System.CurrentOS {
		case "ubuntu":
		default:
			// Only Ubuntu is supported at present
			FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
			return &ShellError{"Unsupported Linux distribution", 1}
		}
		InfoPrintf("Detected OS: %s\n", configs.System.CurrentOS)
	}
	return nil
}

// Get current directory
func GetCurrentDir() error {
	var err error
	configs.System.CurrentDir, err = os.Getwd()
	CheckErrorWithMsg(err, "Failed to get get current directory!\n")
	return err
}

// Get current home directory
func GetUserHomeDir() error {
	var err error
	configs.System.UserHomeDir, err = os.UserHomeDir()
	CheckErrorWithMsg(err, "Failed to get current home directory!\n")
	return err
}

// Create temporary directory
func CreateTmpDir() error {
	var err error
	if configs.System.TmpDir != "" {
		return nil
	}
	WaitPrintf("Creating temporary directory")
	configs.System.TmpDir, err = os.MkdirTemp("", "vHive_tmp")
	CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
	return err
}

// Clean up temporary directory
func CleanUpTmpDir() error {
	if configs.System.TmpDir == "" {
		return nil
	}
	WaitPrintf("Cleaning up temporary directory")
	err := os.RemoveAll(configs.System.TmpDir)
	CheckErrorWithTagAndMsg(err, "Failed to create temporary directory!\n")
	configs.System.TmpDir = ""
	return err
}

func CopyToDir(source string, target string, privileged bool) error {
	var err error

	privilegedCmd := ""
	if privileged {
		privilegedCmd = "sudo"
	}

	_, err = ExecShellCmd("%s cp -R %s %s", privilegedCmd, source, target)

	return err
}

// Get kernel version info (equivalent to `uname -r`)
func GetKernelVersion() (string, error) {
	kernelVersion, err := ExecShellCmd("uname -r")
	return kernelVersion, err
}

// Get kernel arch info (equivalent to `uname -m`)
func GetKernelArch() (string, error) {
	kernelArch, err := ExecShellCmd("uname -m")
	return kernelArch, err
}

// Install packages on various OS
func InstallPackages(packagesTemplate string, pars ...any) error {
	packages := fmt.Sprintf(packagesTemplate, pars...)
	switch configs.System.CurrentOS {
	case "ubuntu":
		_, err := ExecShellCmd("sudo apt-get -qq update && sudo apt-get -qq install -y --allow-downgrades %s", packages)
		return err
	default:
		FatalPrintf("Unsupported Linux distribution: %s\n", configs.System.CurrentOS)
		return &ShellError{Msg: "Unsupported Linux distribution", ExitCode: 1}
	}
}

// Get the value of specific environment variable
func GetEnvironmentVariable(variableNameTemplate string, pars ...any) string {
	variableName := fmt.Sprintf(variableNameTemplate, pars...)
	return os.Getenv(variableName)
}

// Update the value of specific environment variable
func UpdateEnvironmentVariable(variableName string, newValueTemplate string, pars ...any) (string, error) {
	oldValue := GetEnvironmentVariable(variableName)
	newValue := fmt.Sprintf(newValueTemplate, pars...)
	err := os.Setenv(variableName, newValue)
	return oldValue, err
}

// Write to sysctl.conf
func WriteToSysctl(sysConfigTemplate string, pars ...any) error {
	sysConfig := fmt.Sprintf(sysConfigTemplate, pars...)
	_, err := ExecShellCmd("sudo sysctl --quiet -w %s", sysConfig)
	return err
}

// Detect and prepare for the environment
func PrepareEnvironment() error {
	// Define task List
	preTaskList := []func() error{
		DetectArch,
		DetectOS,
		GetCurrentDir,
		GetUserHomeDir,
		CreateTmpDir,
	}
	// Execute task
	for _, task := range preTaskList {
		if err := task(); err != nil {
			return err
		}
	}

	return nil
}

// Clean the Environment
func CleanEnvironment() error {
	// Define task List
	cleanTaskList := []func() error{
		CleanUpTmpDir,
	}
	// Execute task
	for _, task := range cleanTaskList {
		if err := task(); err != nil {
			return err
		}
	}

	return nil
}
