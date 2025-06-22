package main

import (
	"os"

	"k8s.io/component-base/cli"
	"k8s.io/component-base/logs"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"

	"github.com/vhive-serverless/vhive/k8s/scheduler/plugins/snapshotlocality"
)

// Inspired in https://github.com/kubernetes-sigs/scheduler-plugins/blob/9f441058651164d406d6ac7ec751f0dadecd162a/cmd/scheduler/main.go
// TODO add tests

func main() {
	// Register custom plugins to the scheduler framework.
	// Later they can consist of scheduler profile(s) and hence
	// used by various kinds of workloads.
	command := app.NewSchedulerCommand(
		app.WithPlugin(snapshotlocality.PluginName, snapshotlocality.New),
	)
	logs.InitLogs()
	defer logs.FlushLogs()

	code := cli.Run(command)
	os.Exit(code)
}
