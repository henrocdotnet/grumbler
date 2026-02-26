package log

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

var logger *slog.Logger

// Init creates logs/ under dir and configures the package-level logger at DEBUG.
func Init(dir string) error {
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "grumbler.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	logger = slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return nil
}

// InitTee creates logs/ under dir and configures the logger to write to both
// the log file and w (typically os.Stderr).
func InitTee(dir string, w io.Writer) error {
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(logDir, "grumbler.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	logger = slog.New(slog.NewTextHandler(io.MultiWriter(f, w), &slog.HandlerOptions{Level: slog.LevelDebug}))
	return nil
}

// InitTest configures logger to write to the given file (typically os.Stderr).
func InitTest(w *os.File) {
	logger = slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

// L returns the package-level logger, falling back to slog.Default().
func L() *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}
