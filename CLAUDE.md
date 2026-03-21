# CLAUDE.md

## Project Overview

nginx-clickhouse is a Go microservice that tails NGINX access logs and batch-inserts parsed entries into ClickHouse using the native TCP protocol. It exposes Prometheus metrics on port 2112.

## Architecture

```
main.go          → Entry point: tails log file, buffers lines, flushes on interval
config/config.go → YAML config + env var overrides (structured types)
nginx/nginx.go   → Parses NGINX log lines using gonx (configurable log format)
clickhouse/clickhouse.go → Batch-saves parsed entries via clickhouse-go/v2 native TCP
```

Flow: tail log → buffer lines (mutex-protected) → periodic flush → parse → batch insert into ClickHouse.

## Build & Run

```bash
make build                # Static Linux binary (CGO_ENABLED=0)
make docker               # Docker image (multi-stage, scratch-based)
go run main.go            # Run locally (reads config/config.yml by default)
go run main.go -config_path=/path/to/config.yml
```

## Configuration

- YAML config: see `config-sample.yml` for full reference
- Default ClickHouse port: 9000 (native TCP protocol)
- All settings can be overridden via env vars: `LOG_PATH`, `FLUSH_INTERVAL`, `CLICKHOUSE_HOST`, `CLICKHOUSE_PORT`, `CLICKHOUSE_DB`, `CLICKHOUSE_TABLE`, `CLICKHOUSE_USER`, `CLICKHOUSE_PASSWORD`, `NGINX_LOG_TYPE`, `NGINX_LOG_FORMAT`

## Dependencies

- Go 1.25 (as declared in go.mod)
- Key libraries: `ClickHouse/clickhouse-go/v2`, `go-tail`, `gonx`, `logrus`, `prometheus/client_golang`, `yaml.v2`

## Code Conventions

- Standard Go formatting (`gofmt`), verified by `go vet`
- Follows Google Go Style Guide: doc comments on all exports, `errors.Is` for error comparison, lowercase error strings, proper import grouping (stdlib, third-party, project)
- Naming: initialisms are ALL_CAPS (`DB` not `Db`), no `Get` prefix on getters
- Package names: lowercase, single word, with package-level doc comments
- Error handling: `logrus.Fatal` for startup errors, `logrus.Error` for runtime, `fmt.Errorf` with `%w` for wrapping
- Concurrency: `sync.Mutex` protects the shared log buffer
- Modern Go: uses `maps.Keys`, `slices.Collect`, `any`, `errors.Is`

## Testing

```bash
go test ./... -v -race              # Unit tests
go test ./clickhouse/ -v -tags integration  # Integration tests (requires ClickHouse on :9000)
```

Unit tests cover config parsing, env var overrides, NGINX field parsing, and ClickHouse row building. Integration tests verify end-to-end batch inserts against a real ClickHouse instance.

## CI/CD

- `.github/workflows/test.yml` — Unit + integration tests on PRs (ClickHouse service container)
- `.github/workflows/release.yml` — Auto-release with version bump on PR merge to master
