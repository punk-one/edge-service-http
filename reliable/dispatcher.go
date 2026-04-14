package reliable

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/punk-one/edge-service-http/logging"
	transporthttp "github.com/punk-one/edge-service-http/transport/http"
)

const (
	defaultReplayIntervalMs = 3000
	defaultReplayRatePerSec = 20
	defaultRetentionDays    = 7
	retryDelay              = 30 * time.Second
)

type Config struct {
	Enabled          bool
	ReplayIntervalMs int
	ReplayRatePerSec int
	RetentionDays    int
}

type OutboundMessage struct {
	Source      string
	DeviceCode  string
	PayloadJSON []byte
	CollectedAt int64
	TraceID     string
}

type Transport interface {
	SendRaw(context.Context, []byte, string) (transporthttp.DeliveryOutcome, error)
}

type Dispatcher struct {
	cfg       Config
	transport Transport
	store     Store
	logger    logging.Logger
	observers []DeliveryObserver
	stopCh    chan struct{}
	startOnce sync.Once
	closeOnce sync.Once
}

func NewDispatcher(cfg Config, transport Transport, store Store, logger logging.Logger, observers ...DeliveryObserver) *Dispatcher {
	if cfg.ReplayIntervalMs <= 0 {
		cfg.ReplayIntervalMs = defaultReplayIntervalMs
	}
	if cfg.ReplayRatePerSec <= 0 {
		cfg.ReplayRatePerSec = defaultReplayRatePerSec
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = defaultRetentionDays
	}
	if logger == nil {
		logger = logging.New("info", "json")
	}

	return &Dispatcher{
		cfg:       cfg,
		transport: transport,
		store:     store,
		logger:    logger,
		observers: append([]DeliveryObserver(nil), observers...),
		stopCh:    make(chan struct{}),
	}
}

func (d *Dispatcher) Submit(ctx context.Context, message OutboundMessage) error {
	if d.transport == nil {
		return fmt.Errorf("reliable dispatcher transport is nil")
	}

	outcome, err := d.transport.SendRaw(ctx, message.PayloadJSON, message.DeviceCode)
	event := DeliveryEvent{
		TraceID:       message.TraceID,
		DeviceCode:    message.DeviceCode,
		CollectedAt:   message.CollectedAt,
		AttemptCount:  1,
		ShouldRetry:   outcome.ShouldRetry,
		StatusCode:    outcome.StatusCode,
		Accepted:      outcome.Accepted,
		FailureReason: errorString(outcome, err),
		OccurredAt:    time.Now().UnixMilli(),
	}
	if outcome.Delivered && err == nil {
		event.Delivered = true
		event.FailureReason = ""
		d.observe(event)
		return nil
	}

	if !outcome.ShouldRetry || !d.cfg.Enabled {
		d.observe(event)
		return err
	}
	if d.store == nil {
		return fmt.Errorf("reliable dispatcher store is nil")
	}

	now := time.Now().UnixMilli()
	job := StoredJob{
		Source:         message.Source,
		DeviceCode:     message.DeviceCode,
		PayloadJSON:    append([]byte(nil), message.PayloadJSON...),
		CollectedAt:    message.CollectedAt,
		TraceID:        message.TraceID,
		AttemptCount:   1,
		CreatedAt:      now,
		NextRetryAt:    now,
		LastError:      errorString(outcome, err),
		LastHTTPStatus: outcome.StatusCode,
	}
	if appendErr := d.store.Append(job); appendErr != nil {
		return fmt.Errorf("append reliable job: %w", appendErr)
	}

	event.Queued = true
	d.observe(event)
	d.logger.Warn("reliable dispatcher queued retryable message", "device_code", message.DeviceCode, "status", outcome.StatusCode, "error", job.LastError)
	return nil
}

func (d *Dispatcher) StartReplayLoop(ctx context.Context) {
	if !d.cfg.Enabled || d.store == nil || d.transport == nil {
		return
	}

	d.startOnce.Do(func() {
		ticker := time.NewTicker(time.Duration(d.cfg.ReplayIntervalMs) * time.Millisecond)
		go func() {
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-d.stopCh:
					return
				case <-ticker.C:
					if err := d.replayOnce(ctx); err != nil {
						d.logger.Warn("reliable dispatcher replay failed", "error", err)
					}
				}
			}
		}()
	})
}

func (d *Dispatcher) replayOnce(ctx context.Context) error {
	if !d.cfg.Enabled || d.store == nil || d.transport == nil {
		return nil
	}

	cutoff := time.Now().Add(-time.Duration(d.cfg.RetentionDays) * 24 * time.Hour).UnixMilli()
	if _, err := d.store.PurgeExpired(cutoff); err != nil {
		return err
	}

	jobs, err := d.store.FetchPending(d.cfg.ReplayRatePerSec)
	if err != nil {
		return err
	}

	ackIDs := make([]int64, 0, len(jobs))
	flushAck := func() error {
		if len(ackIDs) == 0 {
			return nil
		}
		ids := append([]int64(nil), ackIDs...)
		ackIDs = ackIDs[:0]
		return d.store.Ack(ids)
	}

	for _, job := range jobs {
		outcome, sendErr := d.transport.SendRaw(ctx, job.PayloadJSON, job.DeviceCode)
		event := DeliveryEvent{
			TraceID:       job.TraceID,
			DeviceCode:    job.DeviceCode,
			CollectedAt:   job.CollectedAt,
			AttemptCount:  job.AttemptCount + 1,
			Queued:        true,
			ShouldRetry:   outcome.ShouldRetry,
			StatusCode:    outcome.StatusCode,
			Accepted:      outcome.Accepted,
			FailureReason: errorString(outcome, sendErr),
			Replay:        true,
			OccurredAt:    time.Now().UnixMilli(),
		}
		if outcome.Delivered && sendErr == nil {
			ackIDs = append(ackIDs, job.ID)
			event.Delivered = true
			event.FailureReason = ""
			d.observe(event)
			continue
		}

		if !outcome.ShouldRetry {
			ackIDs = append(ackIDs, job.ID)
			d.observe(event)
			continue
		}

		if err := flushAck(); err != nil {
			return err
		}

		nextRetryAt := time.Now().Add(retryDelay).UnixMilli()
		if err := d.store.UpdateFailure(job.ID, job.AttemptCount+1, nextRetryAt, errorString(outcome, sendErr), outcome.StatusCode); err != nil {
			return err
		}
		d.observe(event)
		return nil
	}

	return flushAck()
}

func (d *Dispatcher) Close() error {
	d.closeOnce.Do(func() {
		close(d.stopCh)
	})
	return nil
}

func errorString(outcome transporthttp.DeliveryOutcome, err error) string {
	if err != nil {
		return err.Error()
	}
	if outcome.FailureReason != "" {
		return outcome.FailureReason
	}
	if outcome.StatusCode != 0 {
		return fmt.Sprintf("status code %d", outcome.StatusCode)
	}
	return "delivery failed"
}

func (d *Dispatcher) observe(event DeliveryEvent) {
	for _, observer := range d.observers {
		if observer != nil {
			observer.OnDelivery(event)
		}
	}
}
