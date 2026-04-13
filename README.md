# edge-service-http

`edge-service-http` is the shared HTTP reporting runtime for edge collectors that push data to MES.

## Capabilities

- HTTP POST reporting with JSON payloads
- `deviceCode` injection into request bodies
- `X-Device-Token` and `X-Device-Mac` header support
- SQLite-backed offline replay for retryable failures
- runtime bootstrap with worker registration
- health, readiness, queue, and recent delivery endpoints

## Runtime Configuration

Store runtime settings in `config.yaml` as YAML. A representative configuration looks like:

```yaml
service:
  host: 0.0.0.0
  port: 59994

httpReport:
  baseURL: http://mes-server:9389
  path: /api/external/iot/spectrum
  timeoutSec: 15
  deviceToken: your-device-token
  deviceMac: AA:BB:CC:DD:EE:FF
  deviceCodeField: deviceCode
  acceptedFalseIsSuccess: true
  retryableStatusCodes: [408, 429, 500, 502, 503, 504]

reliableQueue:
  enabled: true
  sqlitePath: ./data/runtime.db
  batchSize: 100
  flushIntervalMs: 1000
  replayIntervalMs: 3000
  replayRatePerSec: 20
  retentionDays: 7
```

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
