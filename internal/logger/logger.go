// Package logger provides structured logging for apicurio-client.
package logger

import (
	"fmt"
	"os"
	"time"
)

type LogLevel int

const (
	LogLevelQuiet   LogLevel = iota // Errors only
	LogLevelNormal                  // Default (no logs, just results)
	LogLevelVerbose                 // Info + warnings
	LogLevelDebug                   // Everything
)

type Logger struct {
	level LogLevel
}

var globalLogger *Logger

// InitLogger initializes the global logger
func InitLogger(verbose bool, debug bool) {
	level := LogLevelNormal
	if debug {
		level = LogLevelDebug
	} else if verbose {
		level = LogLevelVerbose
	}
	globalLogger = &Logger{level: level}
}

// GetLogger returns the global logger instance
func GetLogger() *Logger {
	if globalLogger == nil {
		globalLogger = &Logger{level: LogLevelNormal}
	}
	return globalLogger
}

func (l *Logger) Debug(format string, args ...interface{}) {
	if l.level >= LogLevelDebug {
		fmt.Fprintf(os.Stderr, "[DEBUG] %s %s\n",
			time.Now().Format("15:04:05.000"),
			fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Info(format string, args ...interface{}) {
	if l.level >= LogLevelVerbose {
		fmt.Fprintf(os.Stderr, "[INFO]  %s %s\n",
			time.Now().Format("15:04:05.000"),
			fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Warn(format string, args ...interface{}) {
	if l.level >= LogLevelVerbose {
		fmt.Fprintf(os.Stderr, "[WARN]  %s %s\n",
			time.Now().Format("15:04:05.000"),
			fmt.Sprintf(format, args...))
	}
}

func (l *Logger) Error(format string, args ...interface{}) {
	if l.level >= LogLevelQuiet {
		fmt.Fprintf(os.Stderr, "[ERROR] %s %s\n",
			time.Now().Format("15:04:05.000"),
			fmt.Sprintf(format, args...))
	}
}

// Timer helps measure operation duration
type Timer struct {
	start time.Time
	label string
}

func (l *Logger) StartTimer(label string) *Timer {
	l.Debug("Starting: %s", label)
	return &Timer{
		start: time.Now(),
		label: label,
	}
}

func (t *Timer) Stop() time.Duration {
	duration := time.Since(t.start)
	GetLogger().Debug("Completed: %s (duration: %dms)", t.label, duration.Milliseconds())
	return duration
}

func (t *Timer) StopWithInfo() time.Duration {
	duration := time.Since(t.start)
	GetLogger().Info("Completed: %s (duration: %dms)", t.label, duration.Milliseconds())
	return duration
}
