package main

import (
	"fmt"
	"strings"

	"github.com/sfreiberg/simplessh"
	"github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupSSHConn(nodeName string) *simplessh.Client {
	utils.InfoPrintf("Connecting to %s\n", nodeName)
	splits := strings.Split(nodeName, "@")
	username := splits[0]
	host := splits[1]
	client, err := simplessh.ConnectWithAgent(host, username)
	if err != nil {
		utils.FatalPrintf("Failed to connect to: %s:%s\n", nodeName, err)
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
