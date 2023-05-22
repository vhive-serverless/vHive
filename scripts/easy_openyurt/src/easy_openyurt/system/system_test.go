package system

import (
	"fmt"
	"testing"

	"github.com/flyinghorse0510/easy_openyurt/src/easy_openyurt/configs"
)

func TestExecShellCmd(t *testing.T) {
	ExecShellCmd("echo %s", `"Successfully execute shell command!"`)
}

func TestVariousDetection(t *testing.T) {
	DetectArch()
	fmt.Printf("Current Architecture: %s\n", configs.System.CurrentArch)
	DetectOS()
	fmt.Printf("Current OS: %s\n", configs.System.CurrentOS)
}

func TestGetDir(t *testing.T) {
	GetCurrentDir()
	fmt.Printf("Current directory: %s\n", configs.System.CurrentDir)
	GetUserHomeDir()
	fmt.Printf("Current home directory: %s\n", configs.System.UserHomeDir)
}

func TestCheckEnvironment(t *testing.T) {
	fmt.Printf("Begin to check system environment...\n")
	DetectOS()
	CheckSystemEnvironment()
}

func TestTmpDir(t *testing.T) {
	fmt.Printf("Create temporary directory")
	CreateTmpDir()
	fmt.Printf("Clean temporary directory")
	CleanUpTmpDir()
}

func TestDownload(t *testing.T) {
	CreateTmpDir()
	filePath, err := DownloadToTmpDir("https://www.google.com/index.html")
	if err != nil {
		t.Fatalf("DownloadToTmpDir(https://www.google.com/index.html): %v", err)
	} else {
		fmt.Printf("Successfullt download %s\n", filePath)
	}
	CleanUpTmpDir()
}
