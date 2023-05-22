// Author: Haoyuan Ma <flyinghorse0510@zju.edu.cn>
package main

import (
	"os"

	knative "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/knative"
	kube "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/kube"
	logs "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/logs"
	system "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/system"
	vhive "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/vhive"
	yurt "github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/yurt"
)

// Init
func init() {
	logs.PrintWelcomeInfo()
	logs.PrintWarningInfo()
	system.DetectOS()
	system.DetectArch()
	system.GetCurrentDir()
	system.GetUserHomeDir()
	system.CreateLogs()
}

func main() {
	// Check Arguments number
	argc := len(os.Args)
	if argc < 4 {
		logs.PrintGeneralUsage()
		logs.FatalPrintf("Invalid arguments: too few arguments!\n")
	}
	// Parse subcommand
	operationObject := os.Args[1]
	switch operationObject {
	case "system":
		// `system` subcommand
		system.ParseSubcommandSystem(os.Args[2:])
	case "kube":
		// `kube` subcommand
		kube.ParseSubcommandKube(os.Args[2:])
	case "yurt":
		// `yurt` subcommand
		yurt.ParseSubcommandYurt(os.Args[2:])
	case "knative":
		// `knative` subcommand
		knative.ParseSubcommandKnative(os.Args[2:])
	case "vhive":
		// `vHive` subcommand
		vhive.ParseSubcommandVHive(os.Args[2:])
	default:
		logs.PrintGeneralUsage()
		logs.FatalPrintf("Invalid object: <object> -> %s\n", operationObject)
	}
}
