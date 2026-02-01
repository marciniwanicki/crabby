package config

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogConfig holds logging configuration
type LogConfig struct {
	// MaxSize is the maximum size in megabytes before rotation
	MaxSize int
	// MaxBackups is the maximum number of old log files to retain
	MaxBackups int
	// MaxAge is the maximum number of days to retain old log files
	MaxAge int
	// Compress determines if rotated files should be compressed
	Compress bool
}

// DefaultLogConfig returns default logging configuration
func DefaultLogConfig() LogConfig {
	return LogConfig{
		MaxSize:    10, // 10 MB
		MaxBackups: 5,  // Keep 5 old files
		MaxAge:     14, // 14 days
		Compress:   true,
	}
}

// LogsDir returns the path to ~/.crabby/logs/
func LogsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}

// SetupLogger creates a zerolog logger that writes to both stdout and a rolling log file
func SetupLogger(cfg LogConfig) (zerolog.Logger, io.Closer, error) {
	logsDir, err := LogsDir()
	if err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to get logs directory: %w", err)
	}

	// Create logs directory if it doesn't exist
	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logPath := filepath.Join(logsDir, "crabby.log")

	// Set up lumberjack for rolling logs
	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}

	// Create a multi-writer for both stdout and file
	// Console output is human-readable, file output is JSON for parsing
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}
	multiWriter := io.MultiWriter(consoleWriter, fileWriter)

	// Create logger with timestamp
	logger := zerolog.New(multiWriter).With().Timestamp().Caller().Logger()

	// Set global log level to debug for detailed logging
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	return logger, fileWriter, nil
}

// SetupFileOnlyLogger creates a logger that only writes to file (no stdout)
func SetupFileOnlyLogger(cfg LogConfig) (zerolog.Logger, io.Closer, error) {
	logsDir, err := LogsDir()
	if err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to get logs directory: %w", err)
	}

	if err := os.MkdirAll(logsDir, 0750); err != nil {
		return zerolog.Logger{}, nil, fmt.Errorf("failed to create logs directory: %w", err)
	}

	logPath := filepath.Join(logsDir, "crabby.log")

	fileWriter := &lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}

	logger := zerolog.New(fileWriter).With().Timestamp().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	return logger, fileWriter, nil
}
