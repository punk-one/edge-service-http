package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/punk-one/edge-service-http/logging"
	"github.com/punk-one/edge-service-http/reliable"
	"github.com/punk-one/edge-service-http/reporting"
	workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
)

type testWorker struct {
	started chan struct{}
	stopped chan struct{}
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
			if w.stopped != nil {
				select {
				case <-w.stopped:
				default:
					close(w.stopped)
				}
			}
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

type statsStore struct {
	stats reliable.QueueStats
	err   error
}

func (s *statsStore) Append(reliable.StoredJob) error { return nil }

func (s *statsStore) FetchPending(int) ([]reliable.StoredJob, error) { return nil, nil }

func (s *statsStore) Ack([]int64) error { return nil }

func (s *statsStore) UpdateFailure(int64, int, int64, string, int) error { return nil }

func (s *statsStore) PurgeExpired(int64) (int64, error) { return 0, nil }

func (s *statsStore) Stats() (reliable.QueueStats, error) { return s.stats, s.err }

func (s *statsStore) Close() error { return nil }

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

func TestApplicationRunCancelsWorkersWhenServerStops(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "runtime.db")

	configYAML := "service:\n  host: 127.0.0.1\n  port: 0\nlogging:\n  level: info\n  format: json\nhttpReport:\n  baseURL: http://127.0.0.1:1\n  path: /report\n  timeoutSec: 1\nreliableQueue:\n  enabled: false\n  sqlitePath: " + sqlitePath + "\n  replayIntervalMs: 10\n  replayRatePerSec: 1\n  retentionDays: 1\n"
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	worker := &testWorker{started: make(chan struct{}), stopped: make(chan struct{})}
	app, err := New(AppConfig{ConfigPath: cfgPath, Workers: []workerpkg.Worker{worker}})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	parentCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan error, 1)
	go func() {
		runDone <- app.Run(parentCtx)
	}()

	select {
	case <-worker.started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not start")
	}

	if err := app.httpServer.Close(); err != nil {
		t.Fatalf("http server close: %v", err)
	}

	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after server close")
	}

	select {
	case <-worker.stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop after Run returned")
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

func TestApplicationStatusDoesNotLatchUnhealthyOnStatsError(t *testing.T) {
	store := &statsStore{err: errors.New("sqlite busy")}
	app := &Application{
		store:   store,
		logger:  logging.New("error", "json"),
		healthy: true,
		ready:   true,
	}

	status := app.status()
	if status.Healthy {
		t.Fatal("status should report unhealthy while stats are failing")
	}
	if status.Ready != true {
		t.Fatalf("ready = %v, want true", status.Ready)
	}
	if len(status.RecentErrors) == 0 || status.RecentErrors[0] != "sqlite busy" {
		t.Fatalf("recentErrors = %#v", status.RecentErrors)
	}

	store.err = nil
	store.stats = reliable.QueueStats{PendingCount: 2, OldestPendingCreatedAt: time.Now().Add(-time.Second).UnixMilli()}

	status = app.status()
	if !status.Healthy {
		t.Fatal("healthy should recover after transient stats failure")
	}
	if status.PendingCount != 2 {
		t.Fatalf("pending count = %d, want 2", status.PendingCount)
	}
}
