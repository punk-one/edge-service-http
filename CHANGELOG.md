# Changelog

## v0.1.0 - 2026-04-13

Initial release of `edge-service-http`.

### Added
- runtime config loading and normalization for service, logging, HTTP reporting, and reliable queue settings
- HTTP POST reporting with JSON payload assembly, `deviceCode` injection, response classification, and retryability handling
- SQLite-backed reliable queue persistence and replay dispatcher for retryable delivery failures
- application-facing reporting service and worker contracts for collector applications
- runtime bootstrap wiring, health/readiness/runtime status endpoints, and a minimal example application
