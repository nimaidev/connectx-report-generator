package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

var log *logrus.Logger

// CustomFormatter formats logs similar to Java's log format
type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("01-02-2006 15:04:05.000")
	level := strings.ToUpper(entry.Level.String())

	// Get caller information
	caller := ""
	if entry.HasCaller() {
		caller = fmt.Sprintf("%s:%d", filepath.Base(entry.Caller.File), entry.Caller.Line)
	}

	// Build the log message
	var msg string
	if len(entry.Data) > 0 {
		// Format fields
		fields := ""
		for k, v := range entry.Data {
			if k != "service" && k != "version" {
				fields += fmt.Sprintf("%s=%v ", k, v)
			}
		}
		if fields != "" {
			msg = fmt.Sprintf("%s  %-5s [%s]: %s %s\n", timestamp, level, caller, entry.Message, fields)
		} else {
			msg = fmt.Sprintf("%s  %-5s [%s]: %s\n", timestamp, level, caller, entry.Message)
		}
	} else {
		msg = fmt.Sprintf("%s  %-5s [%s]: %s\n", timestamp, level, caller, entry.Message)
	}

	return []byte(msg), nil
}

func initLogger() {
	log = logrus.New()

	// Set output to stdout
	log.SetOutput(os.Stdout)

	// Enable caller reporting
	log.SetReportCaller(true)

	// Set custom formatter
	log.SetFormatter(&CustomFormatter{})

	// Set log level (can be configured via environment variable)
	logLevel := os.Getenv("LOG_LEVEL")
	switch logLevel {
	case "debug":
		log.SetLevel(logrus.DebugLevel)
	case "info":
		log.SetLevel(logrus.InfoLevel)
	case "warn":
		log.SetLevel(logrus.WarnLevel)
	case "error":
		log.SetLevel(logrus.ErrorLevel)
	default:
		log.SetLevel(logrus.InfoLevel)
	}

	log.Info("Logger initialized")
}
