package reliable

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/punk-one/edge-service-http/logging"
	transporthttp "github.com/punk-one/edge-service-http/transport/http"
)

type fakeTransport struct {
	outcomes []transporthttp.DeliveryOutcome
	errs     []error
	calls    []sendRawCall
}

type sendRawCall struct {
	payload    []byte
	deviceCode string
}

type blockingTransport struct {
	calls   chan struct{}
	release chan struct{}
}

func (f *fakeTransport) SendRaw(_ context.Context, payloadJSON []byte, deviceCode string) (transporthttp.DeliveryOutcome, error) {
	call := sendRawCall{payload: append([]byte(nil), payloadJSON...), deviceCode: deviceCode}
	f.calls = append(f.calls, call)

	var outcome transporthttp.DeliveryOutcome
	if len(f.outcomes) > 0 {
		outcome = f.outcomes[0]
		f.outcomes = f.outcomes[1:]
	}

	var err error
	if len(f.errs) > 0 {
		err = f.errs[0]
		f.errs = f.errs[1:]
	}

	return outcome, err
}

func (b *blockingTransport) SendRaw(_ context.Context, _ []byte, _ string) (transporthttp.DeliveryOutcome, error) {
	b.calls <- struct{}{}
	<-b.release
	return transporthttp.DeliveryOutcome{Delivered: true}, nil
}

func TestDispatcherSubmitQueuesRetryableFailure(t *testing.T) {
	store := newTestDispatcherStore(t)
	transportErr := errors.New("temporary upstream failure")
	transport := &fakeTransport{
		outcomes: []transporthttp.DeliveryOutcome{{ShouldRetry: true, StatusCode: 503, FailureReason: "temporary upstream failure"}},
		errs:     []error{transportErr},
	}
	dispatcher := NewDispatcher(Config{Enabled: true}, transport, store, logging.New("error", "json"))

	msg := OutboundMessage{
		Source:      "edge",
		DeviceCode:  "device-01",
		PayloadJSON: []byte(`{"sampleId":"S-001"}`),
		CollectedAt: 1700000000000,
		TraceID:     "trace-01",
	}
	payloadAlias := msg.PayloadJSON

	if err := dispatcher.Submit(context.Background(), msg); err != nil {
		t.Fatalf("Submit returned error: %v", err)
	}

	jobs, err := store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("pending jobs = %d, want 1", len(jobs))
	}

	job := jobs[0]
	if job.Source != msg.Source || job.DeviceCode != msg.DeviceCode || job.TraceID != msg.TraceID || job.CollectedAt != msg.CollectedAt {
		t.Fatalf("stored job metadata mismatch: %+v", job)
	}
	if string(job.PayloadJSON) != string(msg.PayloadJSON) {
		t.Fatalf("stored payload = %s, want %s", job.PayloadJSON, msg.PayloadJSON)
	}
	if &job.PayloadJSON[0] == &payloadAlias[0] {
		t.Fatalf("stored payload reuses caller slice")
	}
	if job.LastError != transportErr.Error() {
		t.Fatalf("last error = %q, want %q", job.LastError, transportErr.Error())
	}
	if job.LastHTTPStatus != 503 {
		t.Fatalf("last http status = %d, want 503", job.LastHTTPStatus)
	}
	if job.AttemptCount != 1 {
		t.Fatalf("attempt count = %d, want 1", job.AttemptCount)
	}
	if job.CreatedAt == 0 || job.NextRetryAt == 0 {
		t.Fatalf("expected timestamps to be set, got created_at=%d next_retry_at=%d", job.CreatedAt, job.NextRetryAt)
	}
}

func TestDispatcherSubmitDoesNotQueueDeliveredOrNonRetryable(t *testing.T) {
	t.Run("delivered outcome does not queue", func(t *testing.T) {
		store := newTestDispatcherStore(t)
		transport := &fakeTransport{
			outcomes: []transporthttp.DeliveryOutcome{{Delivered: true, ShouldRetry: false}},
		}
		dispatcher := NewDispatcher(Config{Enabled: true}, transport, store, logging.New("error", "json"))

		if err := dispatcher.Submit(context.Background(), OutboundMessage{DeviceCode: "device-01", PayloadJSON: []byte(`{"ok":true}`)}); err != nil {
			t.Fatalf("Submit returned error: %v", err)
		}

		assertPendingCount(t, store, 0)
	})

	t.Run("non-retryable failure returns transport error", func(t *testing.T) {
		store := newTestDispatcherStore(t)
		transportErr := errors.New("bad request")
		transport := &fakeTransport{
			outcomes: []transporthttp.DeliveryOutcome{{ShouldRetry: false, StatusCode: 400, FailureReason: "bad request"}},
			errs:     []error{transportErr},
		}
		dispatcher := NewDispatcher(Config{Enabled: true}, transport, store, logging.New("error", "json"))

		err := dispatcher.Submit(context.Background(), OutboundMessage{DeviceCode: "device-01", PayloadJSON: []byte(`{"ok":false}`)})
		if !errors.Is(err, transportErr) {
			t.Fatalf("Submit error = %v, want %v", err, transportErr)
		}

		assertPendingCount(t, store, 0)
	})
}

func TestDispatcherReplayOnceAcksDeliveredJobs(t *testing.T) {
	store := newTestDispatcherStore(t)
	createdAt := time.Now().Add(-time.Minute).UnixMilli()
	if err := store.Append(StoredJob{
		DeviceCode:     "device-01",
		PayloadJSON:    []byte(`{"sampleId":"S-001"}`),
		CreatedAt:      createdAt,
		NextRetryAt:    createdAt,
		AttemptCount:   1,
		LastError:      "temporary upstream failure",
		LastHTTPStatus: 503,
	}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	transport := &fakeTransport{outcomes: []transporthttp.DeliveryOutcome{{Delivered: true}}}
	dispatcher := NewDispatcher(Config{Enabled: true, ReplayRatePerSec: 1}, transport, store, logging.New("error", "json"))

	if err := dispatcher.replayOnce(context.Background()); err != nil {
		t.Fatalf("replayOnce returned error: %v", err)
	}

	assertPendingCount(t, store, 0)
	if len(transport.calls) != 1 {
		t.Fatalf("transport calls = %d, want 1", len(transport.calls))
	}
}

func TestDispatcherReplayOnceUpdatesRetryableFailure(t *testing.T) {
	store := newTestDispatcherStore(t)
	createdAt := time.Now().Add(-time.Minute).UnixMilli()
	if err := store.Append(StoredJob{
		DeviceCode:     "device-01",
		PayloadJSON:    []byte(`{"sampleId":"S-001"}`),
		CreatedAt:      createdAt,
		NextRetryAt:    createdAt,
		AttemptCount:   1,
		LastError:      "temporary upstream failure",
		LastHTTPStatus: 503,
	}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	transportErr := errors.New("temporary upstream failure")
	transport := &fakeTransport{
		outcomes: []transporthttp.DeliveryOutcome{{ShouldRetry: true, StatusCode: 503, FailureReason: "temporary upstream failure"}},
		errs:     []error{transportErr},
	}
	dispatcher := NewDispatcher(Config{Enabled: true, ReplayRatePerSec: 1}, transport, store, logging.New("error", "json"))

	before := time.Now().UnixMilli()
	if err := dispatcher.replayOnce(context.Background()); err != nil {
		t.Fatalf("replayOnce returned error: %v", err)
	}
	after := time.Now().UnixMilli()

	jobs, err := store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending returned error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("FetchPending returned %d jobs, want 0 because next retry should move into the future", len(jobs))
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.PendingCount != 1 {
		t.Fatalf("pending count = %d, want 1", stats.PendingCount)
	}

	pending, err := readAllJobs(store)
	if err != nil {
		t.Fatalf("readAllJobs returned error: %v", err)
	}
	job := pending[0]
	if job.AttemptCount != 2 {
		t.Fatalf("attempt count = %d, want 2", job.AttemptCount)
	}
	if job.LastError != transportErr.Error() {
		t.Fatalf("last error = %q, want %q", job.LastError, transportErr.Error())
	}
	if job.LastHTTPStatus != 503 {
		t.Fatalf("last http status = %d, want 503", job.LastHTTPStatus)
	}
	minNextRetry := before + 30000
	maxNextRetry := after + 30000 + 2000
	if job.NextRetryAt < minNextRetry || job.NextRetryAt > maxNextRetry {
		t.Fatalf("next retry at = %d, want between %d and %d", job.NextRetryAt, minNextRetry, maxNextRetry)
	}
}

func TestDispatcherStartReplayLoopStartsOnlyOnce(t *testing.T) {
	store := newTestDispatcherStore(t)
	createdAt := time.Now().Add(-time.Minute).UnixMilli()
	if err := store.Append(StoredJob{
		DeviceCode:   "device-01",
		PayloadJSON:  []byte(`{"sampleId":"S-001"}`),
		CreatedAt:    createdAt,
		NextRetryAt:  createdAt,
		AttemptCount: 1,
	}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	transport := &blockingTransport{
		calls:   make(chan struct{}, 4),
		release: make(chan struct{}),
	}
	dispatcher := NewDispatcher(Config{Enabled: true, ReplayIntervalMs: 10, ReplayRatePerSec: 1}, transport, store, logging.New("error", "json"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	t.Cleanup(func() { _ = dispatcher.Close() })

	dispatcher.StartReplayLoop(ctx)
	dispatcher.StartReplayLoop(ctx)

	select {
	case <-transport.calls:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("expected replay loop to call transport")
	}

	select {
	case <-transport.calls:
		t.Fatal("StartReplayLoop started multiple replay goroutines")
	case <-time.After(50 * time.Millisecond):
	}

	close(transport.release)

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		stats, err := store.Stats()
		if err != nil {
			if strings.Contains(err.Error(), "SQLITE_BUSY") {
				if time.Now().After(deadline) {
					t.Fatalf("Stats kept returning busy: %v", err)
				}
				time.Sleep(10 * time.Millisecond)
				continue
			}
			t.Fatalf("Stats returned error: %v", err)
		}
		if stats.PendingCount == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("expected queued job to be acked after transport release")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func newTestDispatcherStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "dispatcher.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore returned error: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func assertPendingCount(t *testing.T, store Store, want int64) {
	t.Helper()
	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.PendingCount != want {
		t.Fatalf("pending count = %d, want %d", stats.PendingCount, want)
	}
}

func readAllJobs(store *SQLiteStore) ([]StoredJob, error) {
	rows, err := store.db.Query(`SELECT id, source, device_code, payload_json, collected_at, trace_id, attempt_count, created_at, next_retry_at, last_error, last_http_status FROM report_jobs ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []StoredJob
	for rows.Next() {
		var job StoredJob
		if err := rows.Scan(&job.ID, &job.Source, &job.DeviceCode, &job.PayloadJSON, &job.CollectedAt, &job.TraceID, &job.AttemptCount, &job.CreatedAt, &job.NextRetryAt, &job.LastError, &job.LastHTTPStatus); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
