package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogLevel represents the severity level of a log message
type LogLevel int

const (
	DEBUG LogLevel = iota
	INFO
	WARN
	ERROR
	FATAL
)

var levelNames = map[LogLevel]string{
	DEBUG: "DEBUG",
	INFO:  "INFO",
	WARN:  "WARN",
	ERROR: "ERROR",
	FATAL: "FATAL",
}

var levelColors = map[LogLevel]string{
	DEBUG: "\033[36m", // Cyan
	INFO:  "\033[32m", // Green
	WARN:  "\033[33m", // Yellow
	ERROR: "\033[31m", // Red
	FATAL: "\033[35m", // Magenta
}

const colorReset = "\033[0m"

// Logger provides structured logging capabilities
type Logger struct {
	mu          sync.Mutex
	level       LogLevel
	component   string
	logFile     *os.File
	fileLogger  *log.Logger
	stdLogger   *log.Logger
	enableColor bool
	logDir      string
}

// Config holds logger configuration
type Config struct {
	Level       LogLevel
	Component   string // e.g., "controller", "agent"
	LogDir      string // e.g., "/var/log/aria"
	EnableColor bool
	MaxFileSize int64 // Max log file size in bytes before rotation
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// NewLogger creates a new logger instance
func NewLogger(cfg *Config) (*Logger, error) {
	if cfg.LogDir == "" {
		cfg.LogDir = "/var/log/aria"
	}

	// Create log directory if it doesn't exist
	if err := os.MkdirAll(cfg.LogDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %v", err)
	}

	// Create log file (fixed name, no timestamp)
	logFileName := fmt.Sprintf("%s.log", cfg.Component)
	logFilePath := filepath.Join(cfg.LogDir, logFileName)

	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	// Create multi-writer for both file and stderr
	multiWriter := io.MultiWriter(os.Stderr, logFile)

	logger := &Logger{
		level:       cfg.Level,
		component:   cfg.Component,
		logFile:     logFile,
		fileLogger:  log.New(logFile, "", 0),
		stdLogger:   log.New(multiWriter, "", 0),
		enableColor: cfg.EnableColor,
		logDir:      cfg.LogDir,
	}

	// Create symlink to latest log file
	latestLink := filepath.Join(cfg.LogDir, fmt.Sprintf("%s-latest.log", cfg.Component))
	os.Remove(latestLink)
	os.Symlink(logFilePath, latestLink)

	return logger, nil
}

// InitDefaultLogger initializes the default global logger
func InitDefaultLogger(cfg *Config) error {
	var initErr error
	once.Do(func() {
		defaultLogger, initErr = NewLogger(cfg)
	})
	return initErr
}

// GetLogger returns the default logger
func GetLogger() *Logger {
	if defaultLogger == nil {
		// Create a simple console logger if not initialized
		defaultLogger = &Logger{
			level:       INFO,
			component:   "default",
			stdLogger:   log.New(os.Stdout, "", 0),
			enableColor: true,
		}
	}
	return defaultLogger
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.logFile != nil {
		return l.logFile.Close()
	}
	return nil
}

// formatMessage formats the log message with metadata
func (l *Logger) formatMessage(level LogLevel, format string, args ...interface{}) string {
	message := fmt.Sprintf(format, args...)

	// Get caller information
	_, file, line, ok := runtime.Caller(3)
	if ok {
		file = filepath.Base(file)
	} else {
		file = "???"
		line = 0
	}

	levelStr := levelNames[level]
	if l.enableColor {
		levelStr = fmt.Sprintf("%s%s%s", levelColors[level], levelStr, colorReset)
	}

	return fmt.Sprintf("[%s] [%s] %s:%d - %s",
		levelStr, l.component, file, line, message)
}

// log writes a log message at the specified level
func (l *Logger) log(level LogLevel, format string, args ...interface{}) {
	if level < l.level {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	msg := l.formatMessage(level, format, args...)
	l.stdLogger.Println(msg)

	// Also write to file without colors
	if l.fileLogger != nil {
		plainMsg := l.formatMessage(level, format, args...)
		// Remove color codes for file
		plainMsg = strings.ReplaceAll(plainMsg, levelColors[level], "")
		plainMsg = strings.ReplaceAll(plainMsg, colorReset, "")
		l.fileLogger.Println(plainMsg)
	}
}

// Debug logs a debug message
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs an info message
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs a warning message
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs an error message
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal logs a fatal message and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
	os.Exit(1)
}

// WithField returns a new logger with additional context field
func (l *Logger) WithField(key, value string) *Logger {
	newLogger := &Logger{
		level:       l.level,
		component:   fmt.Sprintf("%s][%s=%s", l.component, key, value),
		logFile:     l.logFile,
		fileLogger:  l.fileLogger,
		stdLogger:   l.stdLogger,
		enableColor: l.enableColor,
		logDir:      l.logDir,
	}
	return newLogger
}

// Package-level convenience functions
func Debug(format string, args ...interface{}) {
	GetLogger().Debug(format, args...)
}

func Info(format string, args ...interface{}) {
	GetLogger().Info(format, args...)
}

func Warn(format string, args ...interface{}) {
	GetLogger().Warn(format, args...)
}

func Error(format string, args ...interface{}) {
	GetLogger().Error(format, args...)
}

func Fatal(format string, args ...interface{}) {
	GetLogger().Fatal(format, args...)
}

// RotateLogs rotates log files if they exceed the max size
func (l *Logger) RotateLogs(maxSize int64) error {
	if l.logFile == nil {
		return nil
	}

	info, err := l.logFile.Stat()
	if err != nil {
		return err
	}

	if info.Size() < maxSize {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Close current file
	l.logFile.Close()

	// Rename current file with timestamp
	currentPath := filepath.Join(l.logDir, info.Name())
	rotatedPath := currentPath + "." + time.Now().Format("150405")
	os.Rename(currentPath, rotatedPath)

	// Open new file
	newFile, err := os.OpenFile(currentPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	l.logFile = newFile
	l.fileLogger = log.New(newFile, "", 0)
	multiWriter := io.MultiWriter(os.Stdout, newFile)
	l.stdLogger = log.New(multiWriter, "", 0)

	return nil
}

// ParseLevel parses a log level string
func ParseLevel(levelStr string) LogLevel {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}
