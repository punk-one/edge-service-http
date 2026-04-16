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

	"github.com/gin-gonic/gin"
	"github.com/punk-one/edge-service-http/config"
	"github.com/punk-one/edge-service-http/logging"
	opshttp "github.com/punk-one/edge-service-http/ops/http"
	"github.com/punk-one/edge-service-http/reliable"
	"github.com/punk-one/edge-service-http/reporting"
	workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
	transporthttp "github.com/punk-one/edge-service-http/transport/http"
)

const defaultConfigPath = "./configs/config.yaml"

type AppConfig struct {
	ConfigPath string
	Workers    []workerpkg.Worker
	Options    Options
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
	options    Options

	mu           sync.RWMutex
	healthy      bool
	ready        bool
	recentErrors []string
}

func New(appCfg AppConfig) (*Application, error) {
	opts := appCfg.Options
	if opts.ConfigPath == "" {
		opts.ConfigPath = appCfg.ConfigPath
	}

	var (
		cfg config.Config
		err error
	)
	if opts.Config != nil {
		cfg = config.Normalize(*opts.Config)
	} else {
		cfg, err = config.Load(opts.ConfigPath)
		if err != nil {
			return nil, err
		}
	}

	logger := logging.New(logging.Config{
		Level:      cfg.Logging.Level,
		Format:     cfg.Logging.Format,
		File:       cfg.Logging.File,
		MaxSize:    cfg.Logging.MaxSize,
		MaxFiles:   cfg.Logging.MaxFiles,
		MaxBackups: cfg.Logging.MaxBackups,
		Compress:   cfg.Logging.Compress,
	})
	transport := transporthttp.NewClient(transporthttp.Config{
		URL:                        cfg.Report.URL,
		Timeout:                    time.Duration(cfg.Report.TimeoutSec) * time.Second,
		DeviceToken:                cfg.Report.Token,
		DeviceMac:                  cfg.Report.Mac,
		DeviceCodeField:            cfg.Report.DeviceCodeField,
		AcceptedFalseIsSuccess:     cfg.Report.AcceptedFalseIsSuccess,
		OverwritePayloadDeviceCode: cfg.Report.OverwritePayloadDeviceCode,
		RetryableStatusCodes:       append([]int(nil), cfg.Report.RetryableStatusCodes...),
	}, logger)

	store, err := reliable.NewSQLiteStore(cfg.Queue.SQLitePath)
	if err != nil {
		return nil, fmt.Errorf("create sqlite store: %w", err)
	}

	dispatcher := reliable.NewDispatcher(reliable.Config{
		Enabled:          cfg.Queue.Enabled,
		ReplayIntervalMs: cfg.Queue.ReplayIntervalMs,
		ReplayRatePerSec: cfg.Queue.ReplayRatePerSec,
		RetentionDays:    cfg.Queue.RetentionDays,
	}, transport, store, logger, opts.DeliveryObservers...)

	application := &Application{
		cfg:        cfg,
		logger:     logger,
		store:      store,
		dispatcher: dispatcher,
		reporter:   reporting.New(dispatcher),
		workers:    append([]workerpkg.Worker(nil), appCfg.Workers...),
		options: Options{
			ConfigPath:        opts.ConfigPath,
			RouteRegistrars:   append([]RouteRegistrar(nil), opts.RouteRegistrars...),
			DeliveryObservers: append([]reliable.DeliveryObserver(nil), opts.DeliveryObservers...),
		},
		healthy: true,
	}
	application.ops = opshttp.NewServer(
		opshttp.StatusProviderFunc(application.status),
		runtimeRouteRegistrars(application.options.RouteRegistrars)...,
	)
	application.httpServer = &stdhttp.Server{
		Addr:    cfg.App.Listen,
		Handler: application.ops.Handler(),
	}

	return application, nil
}

func Bootstrap(serviceName, version string, workers ...workerpkg.Worker) {
	BootstrapWithOptions(serviceName, version, Options{ConfigPath: defaultConfigPath}, workers...)
}

func BootstrapWithOptions(serviceName, version string, options Options, workers ...workerpkg.Worker) {
	options.ConfigPath = resolveConfigPath(options.ConfigPath)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := New(AppConfig{
		Workers: workers,
		Options: options,
	})
	if err != nil {
		panic(fmt.Sprintf("%s %s bootstrap failed: %v", serviceName, version, err))
	}
	if err := application.Run(ctx); err != nil {
		panic(fmt.Sprintf("%s %s runtime failed: %v", serviceName, version, err))
	}
}

func resolveConfigPath(path string) string {
	if path == "" {
		return defaultConfigPath
	}
	return path
}

func runtimeRouteRegistrars(registrars []RouteRegistrar) []opshttp.RouteRegistrar {
	if len(registrars) == 0 {
		return nil
	}

	converted := make([]opshttp.RouteRegistrar, 0, len(registrars))
	for _, register := range registrars {
		if register == nil {
			continue
		}
		r := register
		converted = append(converted, func(engine *gin.Engine) {
			r(engine)
		})
	}
	return converted
}

func (a *Application) Run(ctx context.Context) error {
	listener, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		return fmt.Errorf("listen ops server: %w", err)
	}
	defer listener.Close()

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	a.setReady(true)
	defer a.setReady(false)

	a.dispatcher.StartReplayLoop(runCtx)

	for _, worker := range a.workers {
		if worker == nil {
			continue
		}
		go func(w workerpkg.Worker) {
			if err := w.Start(runCtx, runtimeReporter{reporter: a.reporter, onError: a.recordError}); err != nil && !errors.Is(err, context.Canceled) {
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
		cancel()
		a.closeResources()
		return err
	case <-ctx.Done():
		cancel()
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
		if a.logger != nil {
			a.logger.Warn("read runtime queue stats", "error", err)
		}
		recentErrors = append(recentErrors, err.Error())
		if len(recentErrors) > 10 {
			recentErrors = recentErrors[len(recentErrors)-10:]
		}
		return opshttp.Status{
			Healthy:      false,
			Ready:        ready,
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
