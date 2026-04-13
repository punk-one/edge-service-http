package reliable

import (
    "database/sql"
    "os"
    "path/filepath"
    "time"

    _ "modernc.org/sqlite"
)

const sqliteSchema = `
CREATE TABLE IF NOT EXISTS report_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source TEXT NOT NULL DEFAULT '',
    device_code TEXT NOT NULL DEFAULT '',
    payload_json BLOB NOT NULL,
    collected_at INTEGER NOT NULL DEFAULT 0,
    trace_id TEXT NOT NULL DEFAULT '',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    next_retry_at INTEGER NOT NULL,
    last_error TEXT NOT NULL DEFAULT '',
    last_http_status INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_report_jobs_next_retry ON report_jobs(next_retry_at);
`

type SQLiteStore struct {
    db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
    dir := filepath.Dir(path)
    if dir != "." && dir != "" {
        if err := os.MkdirAll(dir, 0o755); err != nil {
            return nil, err
        }
    }

    db, err := sql.Open("sqlite", path)
    if err != nil {
        return nil, err
    }

    store := &SQLiteStore{db: db}
    if err := store.init(); err != nil {
        _ = db.Close()
        return nil, err
    }

    return store, nil
}

func (s *SQLiteStore) init() error {
    _, err := s.db.Exec(sqliteSchema)
    return err
}

func (s *SQLiteStore) Append(job StoredJob) error {
    if job.CreatedAt == 0 {
        job.CreatedAt = time.Now().UnixMilli()
    }
    if job.NextRetryAt == 0 {
        job.NextRetryAt = job.CreatedAt
    }

    _, err := s.db.Exec(
        `INSERT INTO report_jobs
            (source, device_code, payload_json, collected_at, trace_id, attempt_count, created_at, next_retry_at, last_error, last_http_status)
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
        job.Source,
        job.DeviceCode,
        job.PayloadJSON,
        job.CollectedAt,
        job.TraceID,
        job.AttemptCount,
        job.CreatedAt,
        job.NextRetryAt,
        job.LastError,
        job.LastHTTPStatus,
    )
    return err
}

func (s *SQLiteStore) FetchPending(limit int) ([]StoredJob, error) {
    if limit <= 0 {
        return []StoredJob{}, nil
    }

    rows, err := s.db.Query(
        `SELECT id, source, device_code, payload_json, collected_at, trace_id, attempt_count, created_at, next_retry_at, last_error, last_http_status
         FROM report_jobs
         WHERE next_retry_at <= ?
         ORDER BY id ASC
         LIMIT ?`,
        time.Now().UnixMilli(),
        limit,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    jobs := make([]StoredJob, 0, limit)
    for rows.Next() {
        var job StoredJob
        if err := rows.Scan(
            &job.ID,
            &job.Source,
            &job.DeviceCode,
            &job.PayloadJSON,
            &job.CollectedAt,
            &job.TraceID,
            &job.AttemptCount,
            &job.CreatedAt,
            &job.NextRetryAt,
            &job.LastError,
            &job.LastHTTPStatus,
        ); err != nil {
            return nil, err
        }
        jobs = append(jobs, job)
    }
    if err := rows.Err(); err != nil {
        return nil, err
    }

    return jobs, nil
}

func (s *SQLiteStore) Ack(ids []int64) error {
    if len(ids) == 0 {
        return nil
    }

    tx, err := s.db.Begin()
    if err != nil {
        return err
    }

    for _, id := range ids {
        if _, err := tx.Exec("DELETE FROM report_jobs WHERE id = ?", id); err != nil {
            _ = tx.Rollback()
            return err
        }
    }

    return tx.Commit()
}

func (s *SQLiteStore) UpdateFailure(jobID int64, attemptCount int, nextRetryAt int64, lastError string, lastHTTPStatus int) error {
    _, err := s.db.Exec(
        `UPDATE report_jobs
         SET attempt_count = ?, next_retry_at = ?, last_error = ?, last_http_status = ?
         WHERE id = ?`,
        attemptCount,
        nextRetryAt,
        lastError,
        lastHTTPStatus,
        jobID,
    )
    return err
}

func (s *SQLiteStore) PurgeExpired(cutoffMillis int64) (int64, error) {
    res, err := s.db.Exec("DELETE FROM report_jobs WHERE created_at < ?", cutoffMillis)
    if err != nil {
        return 0, err
    }
    return res.RowsAffected()
}

func (s *SQLiteStore) Stats() (QueueStats, error) {
    var stats QueueStats
    var oldest sql.NullInt64

    err := s.db.QueryRow("SELECT COUNT(*), MIN(created_at) FROM report_jobs").Scan(&stats.PendingCount, &oldest)
    if err != nil {
        return QueueStats{}, err
    }
    if oldest.Valid {
        stats.OldestPendingCreatedAt = oldest.Int64
    }

    return stats, nil
}

func (s *SQLiteStore) Close() error {
    if s.db == nil {
        return nil
    }
    return s.db.Close()
}
