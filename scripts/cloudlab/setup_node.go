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

package cloudlab

import (
	setup "github.com/vhive-serverless/vHive/scripts/setup"
	stargz "github.com/vhive-serverless/vHive/scripts/stargz"
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

func SetupNode(haMode string, sandbox string, useStargz string) error {
	if sandbox == "" {
		sandbox = "firecracker"
	}
	// Arguments Check
	switch sandbox {
	case "gvisor":
	case "firecracker":
	case "stock-only":
	default:
		utils.FatalPrintf("Specified sanboxing technique is not supported. Possible are \"stock-only\", \"firecracker\" and \"gvisor\"!\n")
		return &utils.ShellError{Msg: "Sandbox technique not supported!", ExitCode: 1}
	}

	if sandbox != "stock-only" && useStargz == "use-stargz" {
		utils.FatalPrintf("Invalid options! Stargz is only supported with stock-only mode!\n")
		return &utils.ShellError{Msg: "Invalid options: use-stargz", ExitCode: 1}
	}

	// Turn off automatic update
	utils.InfoPrintf("Turn off automatic update\n")
	if err := utils.TurnOffAutomaticUpgrade(); err != nil {
		return err
	}

	// Set up system
	utils.InfoPrintf("Set up system\n")
	if err := setup.SetupSystem(haMode); err != nil {
		return err
	}

	if _, err := utils.ExecShellCmd("sudo mkdir -p /etc/vhive-cri"); err != nil {
		return err
	}

	// Set up sandbox
	switch sandbox {
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

	switch sandbox {
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

	utils.InstallYQ()

	return nil
}
