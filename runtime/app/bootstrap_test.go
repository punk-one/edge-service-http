package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/punk-one/edge-service-http/config"
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

func TestNewUsesOptionsConfigPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "runtime.db")

	configYAML := "service:\n  host: 127.0.0.1\n  port: 0\nlogging:\n  level: info\n  format: json\nhttpReport:\n  baseURL: http://127.0.0.1:1\n  path: /report\n  timeoutSec: 1\nreliableQueue:\n  enabled: false\n  sqlitePath: " + sqlitePath + "\n  replayIntervalMs: 10\n  replayRatePerSec: 1\n  retentionDays: 1\n"
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app, err := New(AppConfig{
		Options: Options{
			ConfigPath:      cfgPath,
			RouteRegistrars: []RouteRegistrar{nil},
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if app.options.ConfigPath != cfgPath {
		t.Fatalf("options config path = %q, want %q", app.options.ConfigPath, cfgPath)
	}
	if got, want := len(app.options.RouteRegistrars), 1; got != want {
		t.Fatalf("route registrar count = %d, want %d", got, want)
	}
}

func TestNewUsesOptionsConfigOverride(t *testing.T) {
	app, err := New(AppConfig{
		ConfigPath: "/definitely/missing/config.yaml",
		Options: Options{
			Config: &config.Config{
				Service: config.ServiceConfig{Host: "127.0.0.1", Port: 0},
				HTTPReport: config.HTTPReportConfig{
					BaseURL:                "http://override.local",
					Path:                   "/report",
					TimeoutSec:             1,
					DeviceCodeField:        "deviceCode",
					AcceptedFalseIsSuccess: true,
				},
				ReliableQueue: config.ReliableQueueConfig{
					Enabled:          false,
					SQLitePath:       filepath.Join(t.TempDir(), "runtime.db"),
					ReplayIntervalMs: 10,
					ReplayRatePerSec: 1,
					RetentionDays:    1,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if got := app.cfg.HTTPReport.BaseURL; got != "http://override.local" {
		t.Fatalf("httpReport.baseURL = %q, want override value", got)
	}
}

func TestResolveConfigPathFallsBackToDefault(t *testing.T) {
	if got := resolveConfigPath(""); got != defaultConfigPath {
		t.Fatalf("resolveConfigPath(\"\") = %q, want %q", got, defaultConfigPath)
	}
}

func TestResolveConfigPathPreservesExplicitValue(t *testing.T) {
	const path = "/tmp/custom-config.yaml"
	if got := resolveConfigPath(path); got != path {
		t.Fatalf("resolveConfigPath(%q) = %q, want %q", path, got, path)
	}
}

func TestRouteRegistrarTypeDoesNotReferenceGin(t *testing.T) {
	typ := reflect.TypeOf(RouteRegistrar(nil))
	if typ.Kind() != reflect.Func {
		t.Fatalf("RouteRegistrar kind = %v, want func", typ.Kind())
	}
	if typ.NumIn() != 1 {
		t.Fatalf("RouteRegistrar args = %d, want 1 generic target arg", typ.NumIn())
	}

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not locate test file path")
	}

	optionsPath := filepath.Join(filepath.Dir(thisFile), "options.go")
	data, err := os.ReadFile(optionsPath)
	if err != nil {
		t.Fatalf("read options.go: %v", err)
	}

	source := string(data)
	if strings.Contains(source, "github.com/gin-gonic/gin") || strings.Contains(source, "*gin.Engine") {
		t.Fatalf("route options should be runtime-agnostic, options.go still references gin")
	}
}

func TestNewConfiguresFileLoggerWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	sqlitePath := filepath.Join(dir, "runtime.db")
	logPath := filepath.Join(dir, "runtime.log")

	configYAML := "service:\n  host: 127.0.0.1\n  port: 0\nlogging:\n  level: info\n  format: json\n  file: " + logPath + "\n  maxSize: 1\n  maxFiles: 1\n  maxBackups: 1\n  compress: false\nhttpReport:\n  baseURL: http://127.0.0.1:1\n  path: /report\n  timeoutSec: 1\nreliableQueue:\n  enabled: false\n  sqlitePath: " + sqlitePath + "\n  replayIntervalMs: 10\n  replayRatePerSec: 1\n  retentionDays: 1\n"
	if err := os.WriteFile(cfgPath, []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	app, err := New(AppConfig{ConfigPath: cfgPath})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	app.logger.Info("file logger smoke")

	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(logPath)
		if err == nil {
			if !strings.Contains(string(data), "file logger smoke") {
				t.Fatalf("log file does not include expected message: %s", string(data))
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected log file %q to be created: %v", logPath, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
