package main

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"

	"github.com/sfreiberg/simplessh"
	"github.com/vhive-serverless/vhive/scripts/openyurt_deployer/logs"
)
func SetupSSHConn(nodeName string) (*simplessh.Client){
	logs.InfoPrintf("Connecting to %s\n", nodeName)
	splits := strings.Split(nodeName, "@")
	username := splits[0]
	host := splits[1]
	client, err := simplessh.ConnectWithAgent(host, username)
	if err != nil {
		logs.FatalPrintf("Failed to connect to: %s:%s\n", nodeName, err)
	}
	return client
}

type ShellError struct {
	msg      string
	exitCode int
}

func (err *ShellError) Error() string {
	return fmt.Sprintf("[exit %d] -> %s", err.exitCode, err.msg)
}

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
		err = &ShellError{msg: trimmedStderr, exitCode: bashProcess.ProcessState.ExitCode()}
	}

	return trimmedStdout, err
}