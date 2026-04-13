package worker

import (
    "context"

    "github.com/punk-one/edge-service-http/reporting"
)

type Reporter interface {
    Report(context.Context, reporting.Message) error
}

type Worker interface {
    Name() string
    Start(context.Context, Reporter) error
}
