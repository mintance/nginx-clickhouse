# CLAUDE.md

## Project Overview

nginx-clickhouse is a Go microservice that tails NGINX access logs and batch-inserts parsed entries into ClickHouse using the native TCP protocol. It supports both traditional text log formats and JSON access logs (`log_format escape=json`), and provides log enrichment (auto-hostname, environment, service tags, status class derivation). It features retry with exponential backoff, optional disk buffering for crash recovery, circuit breaker, structured JSON logging, and Prometheus metrics.

## Architecture

```
main.go                    → Entry point: tail, buffer, flush loop, graceful shutdown, /healthz
config/config.go           → YAML config + env var overrides (structured types)
nginx/nginx.go             → Parses NGINX log lines using gonx (configurable log format)
nginx/json.go              → Parses NGINX JSON access logs (log_format escape=json) and applies enrichments
clickhouse/clickhouse.go   → Client struct: connection mgmt, retry-wrapped Save, health check
retry/retry.go             → Exponential backoff with full jitter
buffer/buffer.go           → Buffer interface + MemoryBuffer
buffer/disk.go             → DiskBuffer: segment-file append, rotation, crash recovery replay
circuitbreaker/circuitbreaker.go → Circuit breaker (closed/open/half-open states)
```

Flow: startup replay (disk buffer) → tail log → buffer lines (memory or disk) → periodic flush (or buffer-full trigger) → parse → retry-wrapped batch insert → graceful shutdown on SIGTERM.

## Build & Run

```bash
make build                # Static Linux binary (CGO_ENABLED=0)
make docker               # Docker image (multi-stage, scratch-based)
make test                 # Unit tests with race detector
make test-integration     # Integration tests (requires ClickHouse on :9000)
make lint                 # gofmt + go vet
go run main.go            # Run locally (reads config/config.yml by default)
```

## Configuration

- YAML config: see `config-sample.yml` for full reference
- Default ClickHouse port: 9000 (native TCP protocol)
- All settings overridable via env vars (see README for full table)
- Key config sections: settings (interval, buffer, retry, circuit_breaker), clickhouse (connection, columns mapping), nginx (log format)

## Dependencies

- Go 1.25 (as declared in go.mod)
- `ClickHouse/clickhouse-go/v2` — ClickHouse native TCP client
- `papertrail/go-tail` — log file tailing
- `satyrius/gonx` — NGINX log format parsing
- `sirupsen/logrus` — structured JSON logging
- `prometheus/client_golang` — Prometheus metrics
- `gopkg.in/yaml.v2` — YAML config parsing
- No external deps for retry, buffer, or circuit breaker (pure Go stdlib)

## Code Conventions

- Standard Go formatting (`gofmt`), verified by `go vet`
- Google Go Style Guide: doc comments on all exports, `errors.Is`, lowercase error strings, import grouping (stdlib, third-party, project)
- Naming: initialisms ALL_CAPS (`DB`), no `Get` prefix, short receiver names
- Error handling: `logrus.Fatal` for startup, `logrus.WithError(err).Error()` for runtime, `fmt.Errorf("context: %w", err)` for wrapping
- Logging: JSON formatter, `WithFields` for structured context
- Concurrency: `sync.Mutex` in buffer, clickhouse client, circuit breaker
- Modern Go: `maps.Keys`, `slices.Collect`, `slices.Sort`, `any`, `errors.Is`, `math/rand/v2`

## Testing

```bash
go test ./... -v -race              # 47 unit tests across 6 packages
go test ./clickhouse/ -v -tags integration  # Integration tests (requires ClickHouse)
```

Packages with tests: retry (7), buffer (10), circuitbreaker (5), clickhouse (8 unit + 4 integration), config (12), nginx (10).

## CI/CD

- `.github/workflows/test.yml` — lint (gofmt + vet), unit tests, integration tests with ClickHouse service container. Runs on PRs to master.
- `.github/workflows/release.yml` — auto version bump, GitHub release with binary, multi-arch Docker push to Docker Hub + GitHub Container Registry. Runs on PR merge to master.
