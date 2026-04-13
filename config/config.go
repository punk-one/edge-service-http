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
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
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
	return Normalize(cfg), nil
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

func defaultConfig() Config {
	return Config{
		Service: ServiceConfig{Host: "0.0.0.0", Port: 59994},
		Logging: LoggingConfig{Level: "info", Format: "json"},
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
