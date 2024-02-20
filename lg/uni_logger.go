package lg

import (
	"log"
	"os"
	// log "github.com/sirupsen/logrus"
)

var UniLogger *log.Logger

func init() {
	file, err := os.OpenFile("output.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalln("Failed to open log file:", err)
	}
	UniLogger = log.New(file, "DEBUG: ", log.Ldate|log.Ltime|log.Lshortfile)
}
