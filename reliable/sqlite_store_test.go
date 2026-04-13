package reliable

import (
	"path/filepath"
	"testing"
)

func TestSQLiteStoreAppendFetchAndAck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	err = store.Append(StoredJob{DeviceCode: "device-01", PayloadJSON: []byte(`{"sampleId":"S-001"}`), TraceID: "trace-1"})
	if err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	jobs, err := store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("pending jobs = %d, want 1", len(jobs))
	}

	if err := store.Ack([]int64{jobs[0].ID}); err != nil {
		t.Fatalf("Ack returned error: %v", err)
	}
	jobs, err = store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending after ack returned error: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("pending jobs after ack = %d, want 0", len(jobs))
	}
}

func TestSQLiteStorePurgesExpiredJobs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	if err := store.Append(StoredJob{DeviceCode: "device-01", PayloadJSON: []byte(`{"sampleId":"S-001"}`), TraceID: "trace-1", CreatedAt: 1}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	removed, err := store.PurgeExpired(2)
	if err != nil {
		t.Fatalf("PurgeExpired returned error: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
}

func TestSQLiteStoreUpdateFailurePersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	if err := store.Append(StoredJob{DeviceCode: "device-01", PayloadJSON: []byte(`{"sampleId":"S-001"}`), TraceID: "trace-1", CreatedAt: 10, NextRetryAt: 10}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	jobs, err := store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("pending jobs = %d, want 1", len(jobs))
	}

	const (
		wantAttempts   = 3
		wantNextRetry  = int64(20)
		wantLastError  = "timeout"
		wantStatusCode = 503
	)

	if err := store.UpdateFailure(jobs[0].ID, wantAttempts, wantNextRetry, wantLastError, wantStatusCode); err != nil {
		t.Fatalf("UpdateFailure returned error: %v", err)
	}

	jobs, err = store.FetchPending(10)
	if err != nil {
		t.Fatalf("FetchPending after update returned error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("pending jobs after update = %d, want 1", len(jobs))
	}
	got := jobs[0]
	if got.AttemptCount != wantAttempts {
		t.Fatalf("attempt_count = %d, want %d", got.AttemptCount, wantAttempts)
	}
	if got.NextRetryAt != wantNextRetry {
		t.Fatalf("next_retry_at = %d, want %d", got.NextRetryAt, wantNextRetry)
	}
	if got.LastError != wantLastError {
		t.Fatalf("last_error = %q, want %q", got.LastError, wantLastError)
	}
	if got.LastHTTPStatus != wantStatusCode {
		t.Fatalf("last_http_status = %d, want %d", got.LastHTTPStatus, wantStatusCode)
	}
}

func TestSQLiteStoreStats(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore returned error: %v", err)
	}
	defer store.Close()

	if err := store.Append(StoredJob{DeviceCode: "device-01", PayloadJSON: []byte(`{"sampleId":"S-001"}`), CreatedAt: 200}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}
	if err := store.Append(StoredJob{DeviceCode: "device-02", PayloadJSON: []byte(`{"sampleId":"S-002"}`), CreatedAt: 100}); err != nil {
		t.Fatalf("Append returned error: %v", err)
	}

	stats, err := store.Stats()
	if err != nil {
		t.Fatalf("Stats returned error: %v", err)
	}
	if stats.PendingCount != 2 {
		t.Fatalf("pending_count = %d, want 2", stats.PendingCount)
	}
	if stats.OldestPendingCreatedAt != 100 {
		t.Fatalf("oldest_created_at = %d, want 100", stats.OldestPendingCreatedAt)
	}
}
