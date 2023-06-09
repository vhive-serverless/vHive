package stargz

import (
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Set up stargz by executing vHive bash scripts (scripts/stargz/setup_stargz.sh)
func SetupStargz() error {
	utils.WaitPrintf("Setting up stargz")
	_, err := utils.ExecVHiveBashScript("scripts/stargz/setup_stargz.sh")
	utils.CheckErrorWithTagAndMsg(err, "Failed to set up stargz!\n")
	return err
}
