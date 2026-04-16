package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("app: {}\nreport: {}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.App.Listen != "0.0.0.0:59994" {
		t.Fatalf("listen = %q, want default listen address", cfg.App.Listen)
	}
	if cfg.Report.DeviceCodeField != "deviceCode" {
		t.Fatalf("DeviceCodeField = %q, want %q", cfg.Report.DeviceCodeField, "deviceCode")
	}
	if !cfg.Report.AcceptedFalseIsSuccess {
		t.Fatalf("AcceptedFalseIsSuccess = false, want true")
	}
	if cfg.Queue.SQLitePath == "" {
		t.Fatalf("SQLitePath should be defaulted")
	}
	if cfg.Logging.MaxFiles != 7 {
		t.Fatalf("MaxFiles = %d, want 7", cfg.Logging.MaxFiles)
	}
	if cfg.Logging.MaxBackups != 0 {
		t.Fatalf("MaxBackups = %d, want 0", cfg.Logging.MaxBackups)
	}
}

func TestLoadConfigPreservesExplicitRetryableStatuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`report:
  retryableStatusCodes: [408, 429, 503]
 `)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := len(cfg.Report.RetryableStatusCodes), 3; got != want {
		t.Fatalf("retryable status count = %d, want %d", got, want)
	}
}

func TestLoadConfigPreservesExplicitAcceptedFalseIsSuccessFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`report:
  acceptedFalseIsSuccess: false
 `)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Report.AcceptedFalseIsSuccess {
		t.Fatalf("AcceptedFalseIsSuccess = true, want false")
	}
}

func TestLoadConfigPreservesExplicitEmptyRetryableStatuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`report:
  retryableStatusCodes: []
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Report.RetryableStatusCodes == nil {
		t.Fatalf("RetryableStatusCodes nil, want empty slice")
	}
	if len(cfg.Report.RetryableStatusCodes) != 0 {
		t.Fatalf("retryable status count = %d, want %d", len(cfg.Report.RetryableStatusCodes), 0)
	}
}

func TestNormalizeSetsLoggingRotationDefaults(t *testing.T) {
	cfg := Normalize(Config{})

	if cfg.Logging.Level != "info" {
		t.Fatalf("level = %q, want info", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Fatalf("format = %q, want json", cfg.Logging.Format)
	}
	if cfg.Logging.MaxSize != 100 {
		t.Fatalf("maxSize = %d, want 100", cfg.Logging.MaxSize)
	}
	if cfg.Logging.MaxBackups != 0 {
		t.Fatalf("maxBackups = %d, want 0", cfg.Logging.MaxBackups)
	}
	if cfg.Logging.MaxFiles != 7 {
		t.Fatalf("maxFiles = %d, want 7", cfg.Logging.MaxFiles)
	}
}

func TestLoadConfigUsesMaxBackupsWhenMaxFilesOmitted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`logging:
  maxBackups: 4
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Logging.MaxFiles != 0 {
		t.Fatalf("maxFiles = %d, want 0", cfg.Logging.MaxFiles)
	}
	if cfg.Logging.MaxBackups != 4 {
		t.Fatalf("maxBackups = %d, want 4", cfg.Logging.MaxBackups)
	}
}

func TestLoadConfigDefaultsMaxFilesWhenRetentionUnset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte("logging:\n  level: debug\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Logging.MaxFiles != 7 {
		t.Fatalf("maxFiles = %d, want 7", cfg.Logging.MaxFiles)
	}
	if cfg.Logging.MaxBackups != 0 {
		t.Fatalf("maxBackups = %d, want 0", cfg.Logging.MaxBackups)
	}
}
