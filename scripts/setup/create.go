package setup

import (
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Original bash script `scripts/create_devmapper.sh`
func CreateDevmapper() error {
	utils.WaitPrintf("Creating devmapper")
	scriptPath := "scripts/create_devmapper.sh"
	_, err := utils.ExecVHiveBashScript(scriptPath)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create devmapper!\n")
	return err
}

// Original bash script `scripts/create_devmapper.sh`
func CreateDockerImage() error {
	utils.WaitPrintf("Creating docker image")
	scriptPath := "scripts/create_docker_image.sh"
	_, err := utils.ExecVHiveBashScript(scriptPath)
	utils.CheckErrorWithTagAndMsg(err, "Failed to create docker image!\n")
	return err
}

// Original bash script `scripts/clean_fcctr.sh`
func CleanFcctr() error {
	utils.WaitPrintf("Cleaning fcctr")
	scriptPath := "scripts/clean_fcctr.sh"
	_, err := utils.ExecVHiveBashScript(scriptPath)
	utils.CheckErrorWithTagAndMsg(err, "Failed to clean fcctr!\n")
	return err
}
