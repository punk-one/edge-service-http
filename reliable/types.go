package reliable

type StoredJob struct {
	ID             int64
	Source         string
	DeviceCode     string
	PayloadJSON    []byte
	CollectedAt    int64
	TraceID        string
	AttemptCount   int
	CreatedAt      int64
	NextRetryAt    int64
	LastError      string
	LastHTTPStatus int
}

type QueueStats struct {
	PendingCount           int64
	OldestPendingCreatedAt int64
}

type DeliveryEvent struct {
	TraceID       string
	DeviceCode    string
	CollectedAt   int64
	AttemptCount  int
	Delivered     bool
	Queued        bool
	ShouldRetry   bool
	StatusCode    int
	Accepted      *bool
	FailureReason string
	Replay        bool
	OccurredAt    int64
}

type DeliveryObserver interface {
	OnDelivery(DeliveryEvent)
}

type DeliveryObserverFunc func(DeliveryEvent)

func (f DeliveryObserverFunc) OnDelivery(event DeliveryEvent) {
	f(event)
}

type Store interface {
	Append(job StoredJob) error
	FetchPending(limit int) ([]StoredJob, error)
	Ack(ids []int64) error
	UpdateFailure(jobID int64, attemptCount int, nextRetryAt int64, lastError string, lastHTTPStatus int) error
	PurgeExpired(cutoffMillis int64) (int64, error)
	Stats() (QueueStats, error)
	Close() error
}
