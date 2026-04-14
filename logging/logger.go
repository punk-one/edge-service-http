package logging

import (
	"io"
	"log/slog"
	"os"
	"strings"

	"gopkg.in/natefinch/lumberjack.v2"
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

type Config struct {
	Level      string
	Format     string
	File       string
	MaxSize    int
	MaxFiles   int
	MaxBackups int
	Compress   bool
}

func New(input any, format ...string) Logger {
	cfg := normalizeConfig(input, format)
	opts := &slog.HandlerOptions{Level: parseLevel(cfg.Level)}
	writer := outputWriter(cfg)
	switch strings.ToLower(strings.TrimSpace(cfg.Format)) {
	case "text":
		return &SLogger{logger: slog.New(slog.NewTextHandler(writer, opts))}
	default:
		return &SLogger{logger: slog.New(slog.NewJSONHandler(writer, opts))}
	}
}

func normalizeConfig(input any, format []string) Config {
	switch v := input.(type) {
	case Config:
		return v
	case *Config:
		if v == nil {
			return Config{}
		}
		return *v
	case string:
		cfg := Config{Level: v}
		if len(format) > 0 {
			cfg.Format = format[0]
		}
		return cfg
	default:
		return Config{}
	}
}

func outputWriter(cfg Config) io.Writer {
	if strings.TrimSpace(cfg.File) == "" {
		return os.Stdout
	}

	rotating := newRotatingLogger(cfg)
	return io.MultiWriter(os.Stdout, rotating)
}

func newRotatingLogger(cfg Config) *lumberjack.Logger {
	return &lumberjack.Logger{
		Filename:   cfg.File,
		MaxSize:    positiveOr(cfg.MaxSize, 100),
		MaxBackups: effectiveRetentionCount(cfg),
		Compress:   cfg.Compress,
	}
}

func effectiveRetentionCount(cfg Config) int {
	if cfg.MaxFiles > 0 {
		return cfg.MaxFiles
	}
	return positiveOr(cfg.MaxBackups, 3)
}

func positiveOr(value, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
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
