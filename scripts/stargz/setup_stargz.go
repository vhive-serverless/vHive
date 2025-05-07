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

package stargz

import (
	utils "github.com/vhive-serverless/vHive/scripts/utils"
)

// Set up stargz by executing vHive bash scripts (scripts/stargz/setup_stargz.sh)
func SetupStargz(sandbox string) error {
	utils.WaitPrintf("Setting up stargz")
	var err error
	if sandbox == "firecracker" {
		_, err = utils.ExecVHiveBashScript("scripts/stargz/setup_demux_snapshotter.sh")

		bashCmd := `sudo sysctl --quiet -w net.ipv4.conf.all.forwarding=1 && ` + 
			`sudo sysctl --quiet -w net.ipv4.ip_forward=1 && ` +
			`sudo sysctl --quiet --system`

		_, err := utils.ExecShellCmd(bashCmd)
		return err
	} else {
		_, err = utils.ExecVHiveBashScript("scripts/stargz/setup_stargz.sh")
	}
	utils.CheckErrorWithTagAndMsg(err, "Failed to set up stargz!\n")
	return err
}
