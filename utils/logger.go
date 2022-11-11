package utils

import (
	"github.com/sirupsen/logrus"
	"os"
	"sync"
)

var logger *logrus.Logger
var once sync.Once

func GetLogger() *logrus.Logger {
	once.Do(func() {
		logger = createLogger()
	})
	return logger
}

func createLogger() *logrus.Logger {
	return &logrus.Logger{
		Out:   os.Stderr,
		Level: logrus.TraceLevel,
		Formatter: &logrus.TextFormatter{
			TimestampFormat: "2006-01-02 15:04:05",
		},
		ReportCaller: true,
	}
}
