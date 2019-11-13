package utils

import (
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

// LoggingConfig specifies all the parameters needed for logging
type LoggingConfig struct {
	Level string
	File  string
}

// ConfigureLogging will take the logging configuration and also adds
// a few default parameters
func ConfigureLogging(appService string) (*logrus.Entry, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel != "" {
		level, err := logrus.ParseLevel(strings.ToUpper(logLevel))
		if err != nil {
			return nil, err
		}
		logrus.SetLevel(level)
	}

	// always use the fulltimestamp
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:    true,
		DisableTimestamp: false,
	})

	return logrus.StandardLogger().WithField("hostname",
		hostname).WithField("service", appService), nil
}
