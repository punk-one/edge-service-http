package main

import (
	"context"
	"time"

	"github.com/punk-one/edge-service-http/reporting"
	"github.com/punk-one/edge-service-http/runtime/app"
	workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
)

type tickerWorker struct{}

func (w *tickerWorker) Name() string { return "ticker" }

func (w *tickerWorker) Start(ctx context.Context, reporter workerpkg.Reporter) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case tick := <-ticker.C:
			if err := reporter.Report(ctx, reporting.Message{
				Source:     w.Name(),
				DeviceCode: "demo-device",
				Payload: map[string]any{
					"message":   "hello from edge-service-http",
					"timestamp": tick.UnixMilli(),
				},
			}); err != nil {
				return err
			}
		}
	}
}

func main() {
	app.Bootstrap("minimal-example", "dev", &tickerWorker{})
}
