package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Service       ServiceConfig       `yaml:"service"`
	Logging       LoggingConfig       `yaml:"logging"`
	Storage       StorageConfig       `yaml:"storage"`
	HTTPReport    HTTPReportConfig    `yaml:"httpReport"`
	ReliableQueue ReliableQueueConfig `yaml:"reliableQueue"`
}

type ServiceConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
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

type StorageConfig struct {
	SQLitePath string `yaml:"sqlitePath"`
}

type HTTPReportConfig struct {
	BaseURL                    string `yaml:"baseURL"`
	Path                       string `yaml:"path"`
	TimeoutSec                 int    `yaml:"timeoutSec"`
	DeviceToken                string `yaml:"deviceToken"`
	DeviceMac                  string `yaml:"deviceMac"`
	DeviceCodeField            string `yaml:"deviceCodeField"`
	AcceptedFalseIsSuccess     bool   `yaml:"acceptedFalseIsSuccess"`
	OverwritePayloadDeviceCode bool   `yaml:"overwritePayloadDeviceCode"`
	RetryableStatusCodes       []int  `yaml:"retryableStatusCodes"`
}

type ReliableQueueConfig struct {
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
	if cfg.Service.Host == "" {
		cfg.Service.Host = def.Service.Host
	}
	if cfg.Service.Port == 0 {
		cfg.Service.Port = def.Service.Port
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
	if cfg.Storage.SQLitePath == "" {
		cfg.Storage.SQLitePath = def.Storage.SQLitePath
	}
	if cfg.HTTPReport.TimeoutSec == 0 {
		cfg.HTTPReport.TimeoutSec = def.HTTPReport.TimeoutSec
	}
	if cfg.HTTPReport.DeviceCodeField == "" {
		cfg.HTTPReport.DeviceCodeField = def.HTTPReport.DeviceCodeField
	}
	if cfg.ReliableQueue.SQLitePath == "" {
		cfg.ReliableQueue.SQLitePath = cfg.Storage.SQLitePath
	}
	if cfg.ReliableQueue.BatchSize == 0 {
		cfg.ReliableQueue.BatchSize = def.ReliableQueue.BatchSize
	}
	if cfg.ReliableQueue.FlushIntervalMs == 0 {
		cfg.ReliableQueue.FlushIntervalMs = def.ReliableQueue.FlushIntervalMs
	}
	if cfg.ReliableQueue.ReplayIntervalMs == 0 {
		cfg.ReliableQueue.ReplayIntervalMs = def.ReliableQueue.ReplayIntervalMs
	}
	if cfg.ReliableQueue.ReplayRatePerSec == 0 {
		cfg.ReliableQueue.ReplayRatePerSec = def.ReliableQueue.ReplayRatePerSec
	}
	if cfg.ReliableQueue.RetentionDays == 0 {
		cfg.ReliableQueue.RetentionDays = def.ReliableQueue.RetentionDays
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
		Service: ServiceConfig{Host: "0.0.0.0", Port: 59994},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "json",
			MaxSize:    100,
			MaxFiles:   7,
			MaxBackups: 0,
			Compress:   false,
		},
		Storage: StorageConfig{SQLitePath: "./data/runtime.db"},
		HTTPReport: HTTPReportConfig{
			TimeoutSec:             int((15 * time.Second).Seconds()),
			DeviceCodeField:        "deviceCode",
			AcceptedFalseIsSuccess: true,
			RetryableStatusCodes:   []int{408, 429, 500, 502, 503, 504},
		},
		ReliableQueue: ReliableQueueConfig{
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
