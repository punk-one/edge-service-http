# edge-service-http Design

Date: 2026-04-13
Status: Approved for planning

## 1. Context

`edge-service-http` is a new shared SDK for edge-side data collection applications that push data to MES over HTTP.

The reference point is `edge-service-sdk`, which already provides a reusable runtime around configuration, logging, SQLite-backed reliability, runtime HTTP endpoints, and worker orchestration. However, that SDK is built around MQTT transport and MQTT-oriented runtime behavior. The current goal is not to retrofit that codebase in place, but to design a new HTTP-oriented SDK that borrows the useful runtime structure while avoiding deep MQTT coupling.

The first concrete consumer will be a spectrum application located at `/home/sun/lh/edge-apps/edge-service-spectrum`. That application will monitor files, parse spectrum results, construct the MES request body defined in `docs/IoT设备推送MES对接规格.md`, and submit those payloads through `edge-service-http`.

Future applications may read SQL Server, Access, or other data sources, but those source integrations are application responsibilities, not SDK responsibilities.

## 2. Goals

- Provide a shared HTTP reporting runtime for multiple edge applications.
- Support configurable MES base URL and one fixed reporting path per application.
- Inject authentication headers required by MES, including `X-Device-Token` and `X-Device-Mac`.
- Inject `deviceCode` into the JSON request body by default.
- Persist failed outbound reports locally and replay them later.
- Treat `HTTP 200` with `data.accepted=false` as a successful delivery that must not be retried.
- Provide bootstrap, logging, queueing, health/readiness, and basic runtime observability.
- Let applications own source access, file watching, parsing, and business payload construction.

## 3. Non-Goals

- Do not implement SQL Server, Access, file monitoring, or file parsing inside the SDK.
- Do not reuse the existing MQTT runtime contract as-is.
- Do not implement dynamic multi-route dispatch in the first phase.
- Do not implement MQTT compatibility, MQTT query handling, property/command control flows, or device profile semantics from `edge-service-sdk` unless a later phase explicitly requires them.
- Do not build UI tooling or device management pages.

## 4. Compatibility Assessment With `edge-service-sdk`

Adapting `edge-service-sdk` directly to cover the full `edge-service-http` scope would require a medium-to-large refactor rather than a small extension.

The main reasons are:

- The root config model is MQTT-centric. `config/bootstrap.go` exposes MQTT broker config and multiple topic sections directly in the main runtime config.
- Runtime bootstrap is transport-specific. `runtime/app/bootstrap.go` constructs an MQTT publisher directly and wires MQTT property, command, query, and status flows during startup.
- Several runtime services depend on MQTT request/reply semantics and MQTT publisher interfaces.
- The SDK packages are not currently centered on a generic outbound reporting abstraction.

Useful reusable ideas still exist:

- logging model and bootstrap style
- SQLite-backed reliable queue strategy
- background replay loop design
- runtime health/readiness HTTP endpoints
- worker supervision patterns

Conclusion:

- Keep `edge-service-sdk` focused on MQTT-oriented services.
- Build `edge-service-http` as a separate SDK.
- Reuse structure and proven patterns where they fit, instead of forcing one SDK to do both jobs now.

## 5. Recommended Approach

Create `edge-service-http` as an independent SDK with an HTTP-first runtime. Follow the layering and operational style of `edge-service-sdk`, but replace MQTT transport semantics with HTTP reporting semantics.

Applications integrate by registering workers with the SDK bootstrap. A worker acquires data from its own source, builds the business payload, and submits it to the SDK reporter. The SDK then handles request decoration, sending, failure persistence, background replay, and runtime observability.

## 6. Architecture Overview

The SDK uses a split responsibility model:

- SDK responsibilities: bootstrap, config loading, logging, HTTP sending, response classification, offline persistence, replay, queue metrics, health endpoints, and worker lifecycle management.
- Application responsibilities: source connectivity, file watching, SQL queries, Access reading, domain mapping, and final business payload construction.

The core runtime model is intentionally more general than the current device/profile model in `edge-service-sdk`, but it still preserves `deviceCode` as a first-class field because downstream MES integration needs it.

The conceptual outbound message model is:

- `source`: logical source name from the application
- `deviceCode`: required business identifier
- `payload`: application-constructed JSON object
- `collectedAt`: original business event time
- `traceId` or idempotency key: optional but recommended for tracing and replay diagnostics

For the first phase, the reporting path is fixed per application. The SDK reads that path from configuration and does not require each message to carry routing information.

## 7. Package Boundaries

Suggested package layout for `edge-service-http`:

- `config`
  Loads and normalizes runtime configuration.
- `logging`
  Shared logging contracts and default implementation.
- `runtime/app`
  Bootstrap entry point, dependency wiring, worker startup, and shutdown orchestration.
- `runtime/worker`
  Worker registration contracts used by applications.
- `reporting`
  Public reporter API used by workers to submit outbound business messages.
- `transport/http`
  HTTP client, request assembly, authentication header injection, timeout handling, response parsing, and retry classification.
- `reliable`
  SQLite-backed persistence and replay for failed HTTP report jobs.
- `ops/http`
  Health, readiness, queue statistics, and recent delivery diagnostics.

The key design principle is that transport concerns stay in `transport/http`, queue durability stays in `reliable`, and application-facing submission remains simple through `reporting`.

## 8. Application Integration Contract

Applications should not manually build the runtime. Instead, the SDK exposes a full bootstrap path.

Expected integration shape:

1. The application starts through `runtime/app`.
2. The application registers one or more workers.
3. Each worker manages its own source logic.
4. Each worker sends normalized report messages through the SDK reporter.
5. The SDK handles immediate send, fallback persistence, and replay.

This keeps all source-specific dependencies in the application repository.

For `edge-service-spectrum` specifically:

- The app owns file watching.
- The app owns spectrum file parsing.
- The app owns construction of the request body required by `/api/external/iot/spectrum`.
- The SDK owns delivery to MES.

## 9. Request Model And Payload Assembly

The SDK should accept a structured report message from applications and convert it into the final HTTP request body.

Rules for the first phase:

- `deviceCode` is a top-level required field in the SDK submission API.
- The SDK injects `deviceCode` into the JSON body before sending.
- The default JSON field name is `deviceCode`.
- The application-provided payload remains the primary business object.
- If the payload already contains `deviceCode`, the SDK should either validate equality with the explicit field or overwrite it consistently based on one explicit policy. The preferred policy is validation plus explicit overwrite only when configured.

This preserves a stable integration contract for future source types while keeping the outgoing body compatible with MES expectations.

## 10. HTTP Transport Rules

The outbound transport is HTTP POST with JSON UTF-8 encoding.

The SDK must support:

- configurable `baseURL`
- configurable fixed `path`
- configurable timeout
- `Content-Type: application/json`
- `X-Device-Token`
- `X-Device-Mac`

The first phase assumes one fixed report path per application instance. Example:

- spectrum app -> `/api/external/iot/spectrum`

No dynamic path routing is required in phase one.

## 11. Delivery Outcome Classification

The delivery classifier controls whether a message is completed or persisted for retry.

Success cases:

- HTTP 2xx with a valid success response body
- HTTP 200 with `data.accepted=false`

Important policy:

- `accepted=false` is still a successful delivery from the SDK perspective.
- It must not enter the offline replay queue.
- It should still be visible in logs and runtime diagnostics.

Retryable failures by default:

- network errors
- DNS/connectivity failures
- request timeout
- HTTP 429
- HTTP 5xx

Non-retryable failures by default:

- ordinary HTTP 4xx other than explicitly configured retryable codes
- payload serialization errors
- permanent local validation errors

This matches the MES contract, where an unapproved device still returns HTTP 200 and should not trigger sender retries.

## 12. Reliable Queue Design

The reliable queue follows the same high-level idea as `edge-service-sdk`: attempt realtime delivery first, then persist for later replay on retryable failure.

Persisted record shape should remain structured, not raw HTTP bytes. Suggested stored fields:

- internal record ID
- source
- deviceCode
- payload JSON
- collectedAt
- traceId
- attemptCount
- createdAt
- nextRetryAt
- lastError
- lastHTTPStatus

Why structured storage is preferred:

- replay logic can evolve without migrating opaque blobs
- diagnostics become easier
- filtering and export are simpler
- request decoration can still be applied consistently at replay time

Replay behavior:

- background loop scans pending records
- records are replayed in batches
- replay uses rate limiting
- successful resend acknowledges and removes the record
- expired records are purged based on retention policy

## 13. Runtime Observability

The SDK should expose runtime HTTP endpoints similar in spirit to the current shared runtime, but focused on delivery state instead of MQTT state.

Minimum useful endpoints:

- `/api/v1/health`
- `/api/v1/ready`
- `/api/v1/runtime/status`
- `/api/v1/runtime/queue`
- `/api/v1/runtime/deliveries/recent`

Expected visibility:

- service running state
- queue depth
- oldest pending age
- last successful delivery time
- last replay time
- recent delivery errors

This gives operators enough visibility to diagnose outage and replay behavior without adding source-specific complexity to the SDK.

## 14. Configuration Model

The first-phase config should be smaller and more HTTP-specific than `edge-service-sdk`.

Suggested top-level sections:

- `service`
- `logging`
- `storage`
- `httpReport`
- `reliableQueue`

Suggested `httpReport` fields:

- `baseURL`
- `path`
- `timeoutSec`
- `deviceToken`
- `deviceMac`
- `deviceCodeField`
- `acceptedFalseIsSuccess`
- `retryableStatusCodes`

Default behavior:

- `deviceCodeField = deviceCode`
- `acceptedFalseIsSuccess = true`
- retryable statuses include `429` and `5xx`

## 15. Phase-One Scope

Phase one should deliver only what is needed to establish the shared SDK and the first consumer.

In scope:

- new `edge-service-http` SDK skeleton
- HTTP reporting client
- request body `deviceCode` injection
- authentication header injection
- SQLite-backed offline replay
- runtime bootstrap and worker registration
- basic ops HTTP endpoints
- first consumer app skeleton at `/home/sun/lh/edge-apps/edge-service-spectrum`
- spectrum app integration against fixed path `/api/external/iot/spectrum`

Out of scope for phase one:

- built-in SQL Server support
- built-in Access support
- built-in file monitoring or parsing utilities inside the SDK
- multiple routes per app
- MQTT compatibility layer
- command/property/query abstractions from the MQTT SDK

## 16. Testing Strategy

SDK tests should focus on reusable runtime behavior.

Required SDK test areas:

- request assembly and headers
- `deviceCode` body injection
- response parsing and delivery classification
- replay queue persistence and ack behavior
- retention cleanup
- bootstrap and worker lifecycle behavior
- ops endpoint status reporting

Application tests for `edge-service-spectrum` should focus on domain logic:

- file parsing correctness
- request body shape compliance with the MES spectrum spec
- integration with SDK reporting path

This split keeps the SDK stable and generic while allowing applications to own their source-specific correctness.

## 17. Risks And Design Guardrails

Main risks:

- recreating too much of `edge-service-sdk` without enough pruning
- letting source-specific logic leak back into the SDK
- under-specifying response classification and causing incorrect replay behavior
- over-generalizing routing before a second real consumer exists

Guardrails:

- keep one fixed path per app in phase one
- keep source access and parsing outside the SDK
- keep `deviceCode` explicit and required
- keep retry rules simple and aligned with MES behavior
- only add abstraction after a second real use case needs it

## 18. Final Recommendation

Proceed with a new `edge-service-http` SDK rather than extending `edge-service-sdk` in place.

Use `edge-service-http` as a shared HTTP delivery runtime with:

- full bootstrap/runtime support
- worker registration
- HTTP POST reporting
- configurable fixed path
- `deviceCode` injection into the JSON body
- `X-Device-Token` and `X-Device-Mac` support
- SQLite-backed offline replay
- health and queue observability

Keep all collectors and parsers in application repositories, with `/home/sun/lh/edge-apps/edge-service-spectrum` as the first consumer.
