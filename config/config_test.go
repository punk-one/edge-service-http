package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigAppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("service: {}\nhttpReport: {}\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPReport.DeviceCodeField != "deviceCode" {
		t.Fatalf("DeviceCodeField = %q, want %q", cfg.HTTPReport.DeviceCodeField, "deviceCode")
	}
	if !cfg.HTTPReport.AcceptedFalseIsSuccess {
		t.Fatalf("AcceptedFalseIsSuccess = false, want true")
	}
	if cfg.ReliableQueue.SQLitePath == "" {
		t.Fatalf("SQLitePath should be defaulted")
	}
}

func TestLoadConfigPreservesExplicitRetryableStatuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`httpReport:
  retryableStatusCodes: [408, 429, 503]
 `)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := len(cfg.HTTPReport.RetryableStatusCodes), 3; got != want {
		t.Fatalf("retryable status count = %d, want %d", got, want)
	}
}

func TestLoadConfigPreservesExplicitAcceptedFalseIsSuccessFalse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`httpReport:
  acceptedFalseIsSuccess: false
 `)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPReport.AcceptedFalseIsSuccess {
		t.Fatalf("AcceptedFalseIsSuccess = true, want false")
	}
}

func TestLoadConfigPreservesExplicitEmptyRetryableStatuses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := []byte(`httpReport:
  retryableStatusCodes: []
`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.HTTPReport.RetryableStatusCodes == nil {
		t.Fatalf("RetryableStatusCodes nil, want empty slice")
	}
	if len(cfg.HTTPReport.RetryableStatusCodes) != 0 {
		t.Fatalf("retryable status count = %d, want %d", len(cfg.HTTPReport.RetryableStatusCodes), 0)
	}
}
