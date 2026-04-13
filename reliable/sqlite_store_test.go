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
