package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	App     AppConfig     `yaml:"app"`
	Logging LoggingConfig `yaml:"logging"`
	Report  ReportConfig  `yaml:"report"`
	Queue   QueueConfig   `yaml:"queue"`
}

type AppConfig struct {
	Name   string `yaml:"name"`
	Listen string `yaml:"listen"`
}

type LoggingConfig struct {
	Level      string `yaml:"level"`
	Format     string `yaml:"format"`
	File       string `yaml:"file"`
	MaxSize    int    `yaml:"maxSize"`
	MaxFiles   int    `yaml:"maxFiles"`
	MaxBackups int    `yaml:"maxBackups"`
	Compress   bool   `yaml:"compress"`
}

type ReportConfig struct {
	URL                        string `yaml:"url"`
	Token                      string `yaml:"token"`
	Mac                        string `yaml:"mac"`
	TimeoutSec                 int    `yaml:"timeoutSec"`
	DeviceCodeField            string `yaml:"deviceCodeField"`
	AcceptedFalseIsSuccess     bool   `yaml:"acceptedFalseIsSuccess"`
	OverwritePayloadDeviceCode bool   `yaml:"overwritePayloadDeviceCode"`
	RetryableStatusCodes       []int  `yaml:"retryableStatusCodes"`
}

type QueueConfig struct {
	Enabled          bool   `yaml:"enabled"`
	SQLitePath       string `yaml:"sqlitePath"`
	BatchSize        int    `yaml:"batchSize"`
	FlushIntervalMs  int    `yaml:"flushIntervalMs"`
	ReplayIntervalMs int    `yaml:"replayIntervalMs"`
	ReplayRatePerSec int    `yaml:"replayRatePerSec"`
	RetentionDays    int    `yaml:"retentionDays"`
}

func Load(path string) (Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}

	hasMaxFiles, hasMaxBackups, err := loggingRetentionFieldPresence(data)
	if err != nil {
		return Config{}, err
	}

	cfg = Normalize(cfg)
	cfg.Logging = normalizeLoggingRetention(cfg.Logging, hasMaxFiles, hasMaxBackups)
	return cfg, nil
}

func Normalize(cfg Config) Config {
	def := defaultConfig()
	if cfg.App.Name == "" {
		cfg.App.Name = def.App.Name
	}
	if cfg.App.Listen == "" {
		cfg.App.Listen = def.App.Listen
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = def.Logging.Level
	}
	if cfg.Logging.Format == "" {
		cfg.Logging.Format = def.Logging.Format
	}
	if cfg.Logging.MaxSize == 0 {
		cfg.Logging.MaxSize = def.Logging.MaxSize
	}
	if cfg.Logging.MaxFiles == 0 && cfg.Logging.MaxBackups == 0 {
		cfg.Logging.MaxFiles = def.Logging.MaxFiles
	}
	if cfg.Report.TimeoutSec == 0 {
		cfg.Report.TimeoutSec = def.Report.TimeoutSec
	}
	if cfg.Report.DeviceCodeField == "" {
		cfg.Report.DeviceCodeField = def.Report.DeviceCodeField
	}
	if cfg.Queue.SQLitePath == "" {
		cfg.Queue.SQLitePath = def.Queue.SQLitePath
	}
	if cfg.Queue.BatchSize == 0 {
		cfg.Queue.BatchSize = def.Queue.BatchSize
	}
	if cfg.Queue.FlushIntervalMs == 0 {
		cfg.Queue.FlushIntervalMs = def.Queue.FlushIntervalMs
	}
	if cfg.Queue.ReplayIntervalMs == 0 {
		cfg.Queue.ReplayIntervalMs = def.Queue.ReplayIntervalMs
	}
	if cfg.Queue.ReplayRatePerSec == 0 {
		cfg.Queue.ReplayRatePerSec = def.Queue.ReplayRatePerSec
	}
	if cfg.Queue.RetentionDays == 0 {
		cfg.Queue.RetentionDays = def.Queue.RetentionDays
	}
	return cfg
}

func loggingRetentionFieldPresence(data []byte) (bool, bool, error) {
	var raw struct {
		Logging struct {
			MaxFiles   *int `yaml:"maxFiles"`
			MaxBackups *int `yaml:"maxBackups"`
		} `yaml:"logging"`
	}
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false, false, fmt.Errorf("parse config: %w", err)
	}
	return raw.Logging.MaxFiles != nil, raw.Logging.MaxBackups != nil, nil
}

func normalizeLoggingRetention(cfg LoggingConfig, hasMaxFiles, hasMaxBackups bool) LoggingConfig {
	def := defaultConfig().Logging

	switch {
	case !hasMaxFiles && !hasMaxBackups:
		cfg.MaxFiles = def.MaxFiles
		cfg.MaxBackups = 0
	case !hasMaxFiles:
		cfg.MaxFiles = 0
	case !hasMaxBackups:
		cfg.MaxBackups = 0
	}

	return cfg
}

func defaultConfig() Config {
	return Config{
		App: AppConfig{
			Name:   "edge-service-http",
			Listen: "0.0.0.0:59994",
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			MaxSize:    100,
			MaxFiles:   7,
			MaxBackups: 0,
			Compress:   false,
		},
		Report: ReportConfig{
			TimeoutSec:             int((15 * time.Second).Seconds()),
			DeviceCodeField:        "deviceCode",
			AcceptedFalseIsSuccess: true,
			RetryableStatusCodes:   []int{408, 429, 500, 502, 503, 504},
		},
		Queue: QueueConfig{
			Enabled:          true,
			SQLitePath:       "./data/runtime.db",
			BatchSize:        100,
			FlushIntervalMs:  1000,
			ReplayIntervalMs: 3000,
			ReplayRatePerSec: 20,
			RetentionDays:    7,
		},
	}
}
