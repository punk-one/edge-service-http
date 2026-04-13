package logging

import (
	"log/slog"
	"os"
	"strings"
)

type Logger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
}

type SLogger struct {
	logger *slog.Logger
}

func New(level string, format string) Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "text":
		return &SLogger{logger: slog.New(slog.NewTextHandler(os.Stdout, opts))}
	default:
		return &SLogger{logger: slog.New(slog.NewJSONHandler(os.Stdout, opts))}
	}
}

func (l *SLogger) Debug(msg string, args ...any) { l.logger.Debug(msg, args...) }
func (l *SLogger) Info(msg string, args ...any)  { l.logger.Info(msg, args...) }
func (l *SLogger) Warn(msg string, args ...any)  { l.logger.Warn(msg, args...) }
func (l *SLogger) Error(msg string, args ...any) { l.logger.Error(msg, args...) }

func parseLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
