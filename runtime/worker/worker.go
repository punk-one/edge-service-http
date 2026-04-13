package worker

import (
    "context"

    "github.com/punk-one/edge-service-http/reporting"
)

// Reporter submits reporting messages for workers.
type Reporter interface {
    Report(context.Context, reporting.Message) error
}

// Worker defines the contract for runtime tasks.
type Worker interface {
    Name() string
    Start(context.Context, Reporter) error
}
