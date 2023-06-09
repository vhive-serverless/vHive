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
