package cloudlab

import (
	setup "github.com/vhive-serverless/vHive/scripts/setup"
	stargz "github.com/vhive-serverless/vHive/scripts/stargz"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupNode(sandBox string, useStargz string) error {
	if sandBox == "" {
		sandBox = "firecracker"
	}
	// Arguments Check
	switch sandBox {
	case "gvisor":
	case "firecracker":
	case "stock-only":
	default:
		utils.FatalPrintf("Specified sanboxing technique is not supported. Possible are \"stock-only\", \"firecracker\" and \"gvisor\"!\n")
		return &utils.ShellError{Msg: "Sandbox technique not supported!", ExitCode: 1}
	}

	// Turn off automatic update
	utils.InfoPrintf("Turn off automatic update\n")
	if err := utils.TurnOffAutomaticUpgrade(); err != nil {
		return err
	}

	// Install Golang
	utils.InfoPrintf("Install Golang\n")
	if err := setup.InstallGo(); err != nil {
		return err
	}

	// Set up system
	utils.InfoPrintf("Set up system\n")
	if err := setup.SetupSystem(); err != nil {
		return err
	}

	if _, err := utils.ExecShellCmd("sudo mkdir -p /etc/vhive-cri"); err != nil {
		return err
	}

	// Set up sandbox
	switch sandBox {
	case "firecracker":
		// Set up firecracker
		utils.InfoPrintf("Set up firecracker\n")
		if err := setup.SetupFirecrackerContainerd(); err != nil {
			return err
		}
	case "gvisor":
		// Set up Gvisor
		utils.WaitPrintf("Set up Gvisor")
		if err := setup.SetupGvisorContainerd(); err != nil {
			return err
		}
	default:
	}

	// Install stock
	utils.InfoPrintf("Install stock\n")
	if err := setup.InstallStock(); err != nil {
		return err
	}

	switch sandBox {
	// Use firecracker
	case "firecracker":
		// create devmapper
		utils.InfoPrintf("Create devmapper\n")
		if err := setup.CreateDevmapper(); err != nil {
			return err
		}
	default:
	}

	// Use Stargz
	if useStargz == "use-stargz" {
		utils.InfoPrintf("Set up stargz\n")
		if err := stargz.SetupStargz(); err != nil {
			return err
		}
	}

	return nil
}
