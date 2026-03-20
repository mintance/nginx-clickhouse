# CLAUDE.md

## Project Overview

nginx-clickhouse is a Go microservice that tails NGINX access logs and batch-inserts parsed entries into ClickHouse via its HTTP API. It exposes Prometheus metrics on port 2112.

## Architecture

```
main.go          → Entry point: tails log file, buffers lines, flushes on interval
config/config.go → YAML config + env var overrides
nginx/nginx.go   → Parses NGINX log lines using gonx (configurable log format)
clickhouse/clickhouse.go → Batch-saves parsed entries to ClickHouse via HTTP
```

Flow: tail log → buffer lines (mutex-protected) → periodic flush → parse → insert into ClickHouse.

## Build & Run

```bash
make build                # Static Linux binary (CGO_ENABLED=0)
make docker               # Docker image (multi-stage, scratch-based)
go run main.go            # Run locally (reads config/config.yml by default)
go run main.go -config=/path/to/config.yml
```

## Configuration

- YAML config: see `config-sample.yml` for full reference
- All settings can be overridden via env vars: `LOG_PATH`, `FLUSH_INTERVAL`, `CLICKHOUSE_HOST`, `CLICKHOUSE_PORT`, `CLICKHOUSE_DB`, `CLICKHOUSE_TABLE`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `NGINX_LOG_TYPE`, `NGINX_LOG_FORMAT`

## Dependencies

- Go 1.25 (as declared in go.mod)
- Key libraries: `go-clickhouse`, `go-tail`, `gonx`, `logrus`, `prometheus/client_golang`, `yaml.v2`

## Code Conventions

- Standard Go formatting (`gofmt`)
- Package names: lowercase, single word
- Error handling: `logrus.Fatal` for startup errors, `logrus.Error` for runtime errors
- Concurrency: `sync.Mutex` protects the shared log buffer
- No unit tests exist in the project currently

## Testing

No `*_test.go` files exist. When adding tests, follow standard Go testing conventions with `go test ./...`.
