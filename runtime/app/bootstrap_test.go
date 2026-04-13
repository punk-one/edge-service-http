package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/punk-one/edge-service-http/reporting"
	workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
)

type testWorker struct {
	started chan struct{}
	err     error
}

func (w *testWorker) Name() string { return "test-worker" }

func (w *testWorker) Start(ctx context.Context, reporter workerpkg.Reporter) error {
	select {
	case <-w.started:
	default:
		close(w.started)
	}

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if w.err != nil {
				return w.err
			}
			_ = reporter.Report(ctx, reporting.Message{
				Source:     w.Name(),
				DeviceCode: "device-01",
				Payload:    map[string]any{"ok": true},
			})
		}
	}
}

func TestApplicationRunStartsWorkers(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "runtime.db")

	configYAML := "service:\n  host: 127.0.0.1\n  port: 0\nlogging:\n  level: info\n  format: json\nhttpReport:\n  baseURL: http://127.0.0.1:1\n  path: /report\n  timeoutSec: 1\nreliableQueue:\n  enabled: false\n  sqlitePath: " + sqlitePath + "\n  replayIntervalMs: 10\n  replayRatePerSec: 1\n  retentionDays: 1\n"
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	worker := &testWorker{started: make(chan struct{})}
	app, err := New(AppConfig{ConfigPath: cfgPath, Workers: []workerpkg.Worker{worker}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(ctx)
	}()

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}

	cancel()

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}

	if status := app.status(); status.Ready {
		t.Fatalf("ready after shutdown = %v", status.Ready)
	}
}

func TestApplicationRecordErrorMarksUnhealthyAndTracksRecentErrors(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "runtime.db")

	configYAML := "service:\n  host: 127.0.0.1\n  port: 0\nlogging:\n  level: info\n  format: json\nhttpReport:\n  baseURL: http://127.0.0.1:1\n  path: /report\n  timeoutSec: 1\nreliableQueue:\n  enabled: false\n  sqlitePath: " + sqlitePath + "\n  replayIntervalMs: 10\n  replayRatePerSec: 1\n  retentionDays: 1\n"
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	worker := &testWorker{started: make(chan struct{}), err: errors.New("worker failed")}
	app, err := New(AppConfig{ConfigPath: cfgPath, Workers: []workerpkg.Worker{worker}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(ctx)
	}()

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}

	deadline := time.After(2 * time.Second)
	for {
		status := app.status()
		if !status.Healthy {
			if len(status.RecentErrors) == 0 {
				t.Fatal("expected recent errors")
			}
			if status.RecentErrors[len(status.RecentErrors)-1] != "test-worker: worker failed" {
				t.Fatalf("recentErrors = %#v", status.RecentErrors)
			}
			break
		}

		select {
		case <-deadline:
			t.Fatal("application did not become unhealthy")
		case <-time.After(10 * time.Millisecond):
		}
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return")
	}
}
