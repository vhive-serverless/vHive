package utils

import (
	"github.com/sirupsen/logrus"
	easy "github.com/t-tomalak/logrus-easy-formatter"
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
		Formatter: &easy.Formatter{
			TimestampFormat: "15:04:05",
			LogFormat:       "[%lvl%]: %time% - %msg%",
		},
		ReportCaller: true,
	}
}
