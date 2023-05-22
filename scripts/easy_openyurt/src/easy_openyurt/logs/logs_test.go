package logs

import (
	"testing"
)

func TestColorfulPrint(t *testing.T) {
	ErrorPrintf("Here is an error message\n")
	WarnPrintf("Here is a warning message\n")
	SuccessPrintf("Here is a successful message\n")
	InfoPrintf("Here is a information message\n")
}
func TestCreateLogs(t *testing.T) {
	CreateLogs(".")
	CommonLog.Printf("This is a common log file\n")
	ErrorLog.Printf("This is an error log file\n")
}
