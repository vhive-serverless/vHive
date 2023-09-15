package logs

import (
	"fmt"
	"os"
	"time"

	log "github.com/sirupsen/logrus"
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

func coloredPrintf(color string, format string, pars ...any) {
	fmt.Print(color)
	fmt.Printf(format, pars...)
	fmt.Print(_colorReset)
}

func ErrorPrintf(format string, pars ...any){
	currentTime := time.Now().Local()
		// For output
	coloredPrintf(_colorRed, "[%02d:%02d:%02d] [Error] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorRed, format, pars...)
}

func WarnPrintf(format string, pars ...any){
	currentTime := time.Now().Local()
		// For output
	coloredPrintf(_colorYellow, "[%02d:%02d:%02d] [Warn] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorYellow, format, pars...)
}

// Print success message (green) in terminal and send it to the common logs(if exist)
func SuccessPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorGreen, "[%02d:%02d:%02d] [Success] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorGreen, format, pars...)
	
}

// Print information (blue) in terminal and send it to the common logs(if exist)
func InfoPrintf(format string, pars ...any) {
	currentTime := time.Now().Local()
	// For output
	coloredPrintf(_colorBlue, "[%02d:%02d:%02d] [Info] ", currentTime.Hour(), currentTime.Minute(), currentTime.Second())
	coloredPrintf(_colorBlue, format, pars...)
}

// Print information (blue) with waiting symbol in terminal and send it to the common logs(if exist)
func WaitPrintf(format string, pars ...any) {
	InfoPrintf(format+" >>>>> ", pars...)
}

// Call `ErrorPrintf()` and then exit with code 1
func FatalPrintf(format string, pars ...any) {
	ErrorPrintf(format, pars...)
	os.Exit(1)
}
func CheckErrorWithMsg(err error, format string, pars ...any) {
	if err != nil {
		ErrorPrintf("%v\n",err)
		log.Fatalf(format, pars...)
	}
}

// If err is not nil, print the error message, send it to the error logs, and then exit
// Otherwise, print a success tag
func CheckErrorWithTagAndMsg(err error, format string, pars ...any) {
	CheckErrorWithMsg(err, format, pars...)
	// For output
	SuccessPrintf("\n")
}

// Print general usage tips
func PrintGeneralUsage() {
	InfoPrintf("Usage: %s  <operation: deploy | clean | demo-c | demo-e | demo-clear | demo-print> [Parameters...]\n", os.Args[0])
}