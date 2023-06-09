package utils

import (
	"os"
	"path"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
)

// Check whether vHive repo exists, if not, clone it to the temporary directory
func CheckVHiveRepo() error {
	if _, err := os.Stat(configs.VHive.VHiveRepoPath); err != nil {
		// vHive Repo not specified or not exist
		WaitPrintf("vHive repo not detected! Automatically cloning the vHive repo")
		// Clone vHive repo automatically
		vHiveRepoPath, err := CloneRepoToTmpDir(configs.VHive.VHiveRepoBranch, configs.VHive.VHiveRepoUrl)
		if CheckErrorWithTagAndMsg(err, "Failed to clone the vHive repo!\n") {
			configs.VHive.VHiveRepoPath = vHiveRepoPath
		}
		return err
	}
	return nil
}

// Get absolute path of the file in vHive repo(given the relative path)
func GetVHiveFilePath(fileRelativePath string) (string, error) {
	// Check vHive repo path
	if err := CheckVHiveRepo(); err != nil {
		return "", err
	}
	// Check demanded file
	fileAbsolutePath := path.Join(configs.VHive.VHiveRepoPath, fileRelativePath)
	if _, err := os.Stat(fileAbsolutePath); err != nil {
		FatalPrintf("File: (%s) NOT found in the vHive repo!\n", fileRelativePath)
		return "", err
	}
	return fileAbsolutePath, nil
}

// Execute bash scripts from vHive repo
func ExecVHiveBashScript(scriptRelativePath string, scriptPars ...string) (string, error) {

	// Check bash script
	if _, err := GetVHiveFilePath(scriptRelativePath); err != nil {
		return "", err
	}

	// Combine all script parameters
	combinedScriptPars := ""
	for _, scriptPar := range scriptPars {
		combinedScriptPars = combinedScriptPars + " " + scriptPar
	}

	// Switch directory and then execute the bash script
	WaitPrintf("Executing vHive bash script --> %s", scriptRelativePath)
	scriptStdOut, err := ExecShellCmd("cd %s && bash %s %s", configs.VHive.VHiveRepoPath, scriptRelativePath, combinedScriptPars)
	// ****** ATTENTION ******
	// When executing a bash script, the return value of the script ONLY implies the success/failure of the last command.
	// So, it doesn't mean that the execution of the whole script is successful! TAKE CARE!
	// (As far as I'm concerned, the ultimate solution is to rewrite those bash scripts with Golang)
	CheckErrorWithTagAndMsg(err, "Failed to execute the bash script!\n")
	return scriptStdOut, err
}
