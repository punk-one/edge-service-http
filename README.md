# edge-service-http

`edge-service-http` is the shared HTTP reporting runtime for edge collectors that push data to MES.

## Capabilities

- HTTP POST reporting with JSON payloads
- `deviceCode` injection into request bodies
- `X-Device-Token` and `X-Device-Mac` header support
- SQLite-backed offline replay for retryable failures
- runtime bootstrap with worker registration
- health, readiness, queue, and recent delivery endpoints

## Minimal Usage

```go
package main

import (
    "context"

    "github.com/punk-one/edge-service-http/runtime/app"
    workerpkg "github.com/punk-one/edge-service-http/runtime/worker"
)

type noopWorker struct{}

func (noopWorker) Name() string { return "noop" }
func (noopWorker) Start(context.Context, workerpkg.Reporter) error { return nil }

func main() {
    app.Bootstrap("edge-service-example", "v0.1.0", noopWorker{})
}
```
