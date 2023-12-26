// MIT License
//
// Copyright (c) 2023 Jason Chua, Ruiqi Lai and vHive team
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

package main

import (
	"testing"
)

func TestDetectArchofVM(t *testing.T) {
	// Call the method to be tested
	mockNode.DetectArch()

	if mockNode.Configs.System.CurrentArch == "" {
		t.Errorf("Expected CurrentArch not supposed to be empty.\n")
	}
	t.Logf("CurrentArch: %v", mockNode.Configs.System.CurrentArch)
}

func TestDetectOSofVM(t *testing.T) {
	// Call the method to be tested
	mockNode.DetectOS()

	if mockNode.Configs.System.CurrentOS == "" {
		t.Errorf("Expected CurrentOS not supposed to be empty.\n")
	}
	t.Logf("CurrentOS: %v", mockNode.Configs.System.CurrentOS)
}

func TestGetCurrentDir(t *testing.T) {
	// Call the method to be tested
	mockNode.GetCurrentDir()

	if mockNode.Configs.System.CurrentDir == "" {
		t.Errorf("Expected CurrentDir not supposed to be empty.\n")
	}
	t.Logf("CurrentDir: %v", mockNode.Configs.System.CurrentDir)
}

func TestGetUserHomeDir(t *testing.T) {
	// Call the method to be tested
	mockNode.GetUserHomeDir()

	if mockNode.Configs.System.UserHomeDir == "" {
		t.Errorf("Expected UserHomeDir not supposed to be empty.\n")
	}
	t.Logf("UserHomeDir: %v", mockNode.Configs.System.UserHomeDir)
}

func TestGetNodeHostName(t *testing.T) {
	// Call the method to be tested
	mockNode.GetNodeHostName()

	if mockNode.Configs.System.NodeHostName == "" {
		t.Errorf("Expected NodeHostName not supposed to be empty.\n")
	}
	t.Logf("NodeHostName: %v", mockNode.Configs.System.NodeHostName)
}

func TestCreateTmpDir(t *testing.T) {
	// Call the method to be tested
	mockNode.CreateTmpDir()

	result, _ := mockNode.ExecShellCmd("ls | grep yurt_tmp")

	if result != "yurt_tmp" {
		t.Errorf("Temp file creation test fail.\n")
	}
	t.Logf("Result: %v", result)
}

func TestExtractingToTargetDir(t *testing.T) {
	// Create the mock tar.gz file
	mockNode.ExecShellCmd("mkdir $HOME/projects/ $HOME/temp/ && cd projects && touch mockFile-1 mockFile-2 && cd ..")
	mockNode.ExecShellCmd("tar -czvf projects.tar.gz -C projects .")

	// Create tmp dir to extract
	mockNode.ExtractToDir("projects.tar.gz", "$HOME/temp", false)

	result, _ := mockNode.ExecShellCmd("ls temp | wc -l")
	t.Logf("Result for file count: %v", result)

	// // Remove mock file nand tmp dir
	mockNode.ExecShellCmd("rm -rf $HOME/projects/ $HOME/temp/ projects.tar.gz")

	if result != "2" {
		t.Errorf("Expected file is 2, returned value is %s.\n", result)
	}

}

func TestDownloadingToTmpDir(t *testing.T) {

	mockNode.Configs.System.TmpDir = "~/mockDir"

	mockNode.ExecShellCmd("mkdir %s", mockNode.Configs.System.TmpDir)

	filePath, _ := mockNode.DownloadToTmpDir("https://go.dev/dl/go1.21.5.linux-arm64.tar.gz")

	mockNode.ExecShellCmd("rm -rf %s", mockNode.Configs.System.TmpDir)

	t.Logf("FilePath returned: %s", filePath)
}
