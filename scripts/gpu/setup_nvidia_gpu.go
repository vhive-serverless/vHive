package gpu

import (
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Set up nvidia gpu support by executing vHive bash scripts (scripts/gpu/setup_nvidia_gpu.sh)
func SetupNvidiaGpu() error {
	utils.WaitPrintf("Setting up nvidia gpu support")
	scriptPath := "scripts/gpu/setup_nvidia_gpu.sh"
	_, err := utils.ExecVHiveBashScript(scriptPath)
	utils.CheckErrorWithTagAndMsg(err, "Failed to set up nvidia gpu support!\n")
	return err
}
