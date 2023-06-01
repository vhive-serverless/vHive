// Author: Haoyuan Ma <flyinghorse0510@zju.edu.cn>
package utils

import (
	"fmt"
	"log"
	"os"
	"time"

	configs "github.com/vhive-serverless/vHive/scripts/configs"
)

var (
	_colorReset  = "\033[0m"
	_colorRed    = "\033[31m"
	_colorGreen  = "\033[32m"
	_colorYellow = "\033[33m"
	_colorBlue   = "\033[34m"
	// _colorPurple = "\033[35m"
	// _colorCyan   = "\033[36m"
	// _colorGray   = "\033[37m"
	// _colorWhite  = "\033[97m"
)

var (
	CommonLog *log.Logger = nil // Common logs
	ErrorLog  *log.Logger = nil // Error logs
)

// Print colored text in terminal
func coloredPrintf(color string, format string, pars ...any) {
	fmt.Print(color)
	fmt.Printf(format, pars...)
	fmt.Print(_colorReset)
}

// Print error message (red) in terminal and send it to the error logs(if exist)
func ErrorPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorRed, "[%02d:%02d:%02d] [Error] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorRed, format, pars...)
	// For logs
	if ErrorLog != nil {
		ErrorLog.Printf(format, pars...)
	}
}

// Print warning message (yellow) in terminal and send it to the common logs(if exist)
func WarnPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorYellow, "[%02d:%02d:%02d] [Warn] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorYellow, format, pars...)
	// For logs
	if CommonLog != nil {
		CommonLog.Printf(format, pars...)
	}
}

// Print success message (green) in terminal and send it to the common logs(if exist)
func SuccessPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorGreen, "[%02d:%02d:%02d] [Success] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorGreen, format, pars...)
	// For logs
	if CommonLog != nil {
		CommonLog.Printf(format, pars...)
	}
}

// Print information (blue) in terminal and send it to the common logs(if exist)
func InfoPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorBlue, "[%02d:%02d:%02d] [Info] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorBlue, format, pars...)
	// For logs
	if CommonLog != nil {
		CommonLog.Printf(format, pars...)
	}
}

// Print information (blue) with waiting symbol in terminal and send it to the common logs(if exist)
func WaitPrintf(format string, pars ...any) {
	InfoPrintf(format+" >>>>> ", pars...)
}

// Call `ErrorPrintf()`
func FatalPrintf(format string, pars ...any) {
	ErrorPrintf(format, pars...)
}

// If err is not nil, print the error message, send it to the error logs, and return false
// Otherwise, do nothing and return true
func CheckErrorWithMsg(err error, format string, pars ...any) bool {
	if err != nil {
		ErrorPrintf("%v\n", err)
		FatalPrintf(format, pars...)
		return false
	}
	return true
}

// If err is not nil, print the error message, send it to the error logs, and return false
// Otherwise, print a success tag, and return true
func CheckErrorWithTagAndMsg(err error, format string, pars ...any) bool {
	if CheckErrorWithMsg(err, format, pars...) {
		SuccessPrintf("\n")
		return true
	}
	return false
}

// Print general usage tips
func PrintGeneralUsage() {
	InfoPrintf("Usage: %s <object: system | kube | yurt> <nodeRole: master | worker> <operation: init | join | expand> [Parameters...]\n", os.Args[0])
}

// Print welcome information
func PrintWelcomeInfo() {
	coloredPrintf(_colorGreen, "<<<<<<<<< vHiveSetupScripts %s >>>>>>>>>\n", configs.Version)
}

// Print warning information
func PrintWarningInfo() {
	WarnPrintf("THIS IS AN EXPERIMENTAL PROGRAM DEVELOPED PERSONALLY\n")
	WarnPrintf("DO NOT ATTEMPT TO USE IN PRODUCTION ENVIRONMENT!\n")
	WarnPrintf("MAKE SURE TO BACK UP YOUR SYSTEM AND TAKE CARE!\n")
}

// Create Logs
func CreateLogs(logDir string) {
	// notify user
	WaitPrintf("Creating log files")

	// create log files
	commonLogFile, err := os.OpenFile(logDir+"/vHiveSetupScriptsCommon.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	CheckErrorWithMsg(err, "Failed to create log files!\n")

	errorLogFile, err := os.OpenFile(logDir+"/vHiveSetupScriptsError.log", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0666)
	CheckErrorWithMsg(err, "Failed to create log files!\n")

	// create Logger
	CommonLog = log.New(commonLogFile, "INFO: ", log.Ltime|log.Lshortfile)
	ErrorLog = log.New(errorLogFile, "ERROR: ", log.Ltime|log.Lshortfile)

	// Success
	SuccessPrintf("\n")
	SuccessPrintf("Stdout Log -> %s/vHiveSetupScriptsCommon.log\n", logDir)
	SuccessPrintf("Stderr Log -> %s/vHiveSetupScriptsError.log\n", logDir)
}
