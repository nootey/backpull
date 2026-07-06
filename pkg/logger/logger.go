package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func InitLogger(level string) (*zap.Logger, error) {
	cfg := zap.NewDevelopmentConfig()

	logFile := getLogFilePath()

	cfg.OutputPaths = []string{
		"stdout", // keep console output
		logFile,
	}
	cfg.ErrorOutputPaths = []string{
		"stderr",
		logFile,
	}

	// Set log level, falling back to Info on empty/invalid input
	cfg.Level = zap.NewAtomicLevelAt(parseLevel(level))

	logger, err := cfg.Build(zap.AddStacktrace(zapcore.ErrorLevel))
	if err != nil {
		return nil, err
	}

	return logger, nil
}

func parseLevel(level string) zapcore.Level {
	l, err := zapcore.ParseLevel(strings.ToLower(strings.TrimSpace(level)))
	if err != nil {
		return zapcore.InfoLevel
	}
	return l
}

func getLogFilePath() string {
	const logDir = "logs"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		panic(fmt.Sprintf("failed to create log directory: %v", err))
	}

	return filepath.Join(logDir, "app.log")
}
