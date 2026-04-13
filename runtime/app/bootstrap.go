package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	stdhttp "net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/punk-one/edge-service-http/config"
	"github.com/punk-one/edge-service-http/logging"
	opshttp "github.com/punk-one/edge-service-http/ops/http"
	"github.com/punk-one/edge-service-http/reliable"
	"github.com/punk-one/edge-service-http/reporting"
	workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
	transporthttp "github.com/punk-one/edge-service-http/transport/http"
)

type AppConfig struct {
	ConfigPath string
	Workers    []workerpkg.Worker
}

type Application struct {
	cfg        config.Config
	logger     logging.Logger
	store      reliable.Store
	dispatcher *reliable.Dispatcher
	reporter   *reporting.Service
	ops        *opshttp.Server
	httpServer *stdhttp.Server
	workers    []workerpkg.Worker

	mu           sync.RWMutex
	healthy      bool
	ready        bool
	recentErrors []string
}

func New(appCfg AppConfig) (*Application, error) {
	cfg, err := config.Load(appCfg.ConfigPath)
	if err != nil {
		return nil, err
	}

	logger := logging.New(cfg.Logging.Level, cfg.Logging.Format)
	transport := transporthttp.NewClient(transporthttp.Config{
		BaseURL:                    cfg.HTTPReport.BaseURL,
		Path:                       cfg.HTTPReport.Path,
		Timeout:                    time.Duration(cfg.HTTPReport.TimeoutSec) * time.Second,
		DeviceToken:                cfg.HTTPReport.DeviceToken,
		DeviceMac:                  cfg.HTTPReport.DeviceMac,
		DeviceCodeField:            cfg.HTTPReport.DeviceCodeField,
		AcceptedFalseIsSuccess:     cfg.HTTPReport.AcceptedFalseIsSuccess,
		OverwritePayloadDeviceCode: cfg.HTTPReport.OverwritePayloadDeviceCode,
		RetryableStatusCodes:       append([]int(nil), cfg.HTTPReport.RetryableStatusCodes...),
	}, logger)

	store, err := reliable.NewSQLiteStore(cfg.ReliableQueue.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("create sqlite store: %w", err)
	}

	dispatcher := reliable.NewDispatcher(reliable.Config{
		Enabled:          cfg.ReliableQueue.Enabled,
		ReplayIntervalMs: cfg.ReliableQueue.ReplayIntervalMs,
		ReplayRatePerSec: cfg.ReliableQueue.ReplayRatePerSec,
		RetentionDays:    cfg.ReliableQueue.RetentionDays,
	}, transport, store, logger)

	application := &Application{
		cfg:        cfg,
		logger:     logger,
		store:      store,
		dispatcher: dispatcher,
		reporter:   reporting.New(dispatcher),
		workers:    append([]workerpkg.Worker(nil), appCfg.Workers...),
		healthy:    true,
	}
	application.ops = opshttp.NewServer(opshttp.StatusProviderFunc(application.status))
	application.httpServer = &stdhttp.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Service.Host, cfg.Service.Port),
		Handler: application.ops.Handler(),
	}

	return application, nil
}

func Bootstrap(serviceName, version string, workers ...workerpkg.Worker) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := New(AppConfig{
		ConfigPath: "./configs/config.yaml",
		Workers:    workers,
	})
	if err != nil {
		panic(fmt.Sprintf("%s %s bootstrap failed: %v", serviceName, version, err))
	}
	if err := application.Run(ctx); err != nil {
		panic(fmt.Sprintf("%s %s runtime failed: %v", serviceName, version, err))
	}
}

func (a *Application) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen ops server: %w", err)
	}
	defer listener.Close()

	a.setReady(true)
	defer a.setReady(false)

	a.dispatcher.StartReplayLoop(ctx)

	for _, worker := range a.workers {
		if worker == nil {
			continue
		}
		go func(w workerpkg.Worker) {
			if err := w.Start(ctx, runtimeReporter{reporter: a.reporter, onError: a.recordError}); err != nil && !errors.Is(err, context.Canceled) {
				a.recordError(fmt.Sprintf("%s: %v", w.Name(), err))
			}
		}(worker)
	}

	serverErrCh := make(chan error, 1)
	go func() {
		err := a.httpServer.Serve(listener)
		if err != nil && !errors.Is(err, stdhttp.ErrServerClosed) {
			a.recordError(err.Error())
			serverErrCh <- err
			return
		}
		serverErrCh <- nil
	}()

	select {
	case err := <-serverErrCh:
		a.closeResources()
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.httpServer.Shutdown(shutdownCtx)
		a.closeResources()
		if err := <-serverErrCh; err != nil {
			return err
		}
		return nil
	}
}

type runtimeReporter struct {
	reporter reportingReporter
	onError  func(string)
}

type reportingReporter interface {
	Report(context.Context, reporting.Message) error
}

func (r runtimeReporter) Report(ctx context.Context, message reporting.Message) error {
	err := r.reporter.Report(ctx, message)
	if err != nil && r.onError != nil {
		r.onError(err.Error())
	}
	return err
}

func (a *Application) status() opshttp.Status {
	a.mu.RLock()
	healthy := a.healthy
	ready := a.ready
	recentErrors := append([]string(nil), a.recentErrors...)
	a.mu.RUnlock()

	stats, err := a.store.Stats()
	if err != nil {
		a.recordError(err.Error())
		return opshttp.Status{
			Healthy:      false,
			Ready:        false,
			RecentErrors: recentErrors,
		}
	}

	oldestPendingAge := int64(0)
	if stats.OldestPendingCreatedAt > 0 {
		oldestPendingAge = time.Now().UnixMilli() - stats.OldestPendingCreatedAt
		if oldestPendingAge < 0 {
			oldestPendingAge = 0
		}
	}

	return opshttp.Status{
		Healthy:          healthy,
		Ready:            ready,
		PendingCount:     stats.PendingCount,
		OldestPendingAge: oldestPendingAge,
		RecentErrors:     recentErrors,
	}
}

func (a *Application) setReady(ready bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ready = ready
}

func (a *Application) closeResources() {
	a.mu.Lock()
	a.ready = false
	a.mu.Unlock()
	_ = a.httpServer.Close()
	_ = a.dispatcher.Close()
	_ = a.store.Close()
}

func (a *Application) recordError(message string) {
	if message == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.healthy = false
	const maxRecentErrors = 10
	if len(a.recentErrors) == maxRecentErrors {
		copy(a.recentErrors, a.recentErrors[1:])
		a.recentErrors[maxRecentErrors-1] = message
		return
	}
	a.recentErrors = append(a.recentErrors, message)
}
