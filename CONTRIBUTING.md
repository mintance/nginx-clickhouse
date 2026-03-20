# Contributing to nginx-clickhouse

Thank you for your interest in contributing! Here's how to get started.

## Development Setup

### Prerequisites

- Go 1.25+
- Docker (for integration tests and building Docker images)

### Clone and Build

```sh
git clone https://github.com/mintance/nginx-clickhouse.git
cd nginx-clickhouse
go build -o nginx-clickhouse .
```

### Available Make Targets

| Target | Description |
|---|---|
| `make build` | Build a static Linux binary |
| `make docker` | Build Docker image |
| `make lint` | Run `gofmt` and `go vet` |
| `make test` | Run unit tests with race detector |
| `make test-integration` | Run integration tests (requires ClickHouse on `:9000`) |

### Running Tests

```sh
# Unit tests
make test

# Integration tests (start ClickHouse first)
docker run -d --name clickhouse-test -p 9000:9000 clickhouse/clickhouse-server:latest
make test-integration

# Clean up
docker rm -f clickhouse-test
```

## Project Structure

```
main.go                      Entry point, log tailing, flush loop
config/config.go             YAML config parsing, env var overrides
nginx/nginx.go               NGINX log format parsing
clickhouse/clickhouse.go     ClickHouse batch insert via native TCP
clickhouse/integration_test.go  Integration tests (build tag: integration)
grafana/                     Pre-built Grafana dashboard
config-sample.yml            Configuration template
```

## Code Style

This project follows the [Google Go Style Guide](https://google.github.io/styleguide/go/). Key points:

- Run `gofmt` and `go vet` before committing
- Add doc comments to all exported names, starting with the name itself
- Group imports: stdlib, third-party, project (separated by blank lines)
- Use lowercase error strings without trailing punctuation
- Wrap errors with `fmt.Errorf("context: %w", err)` when callers need to inspect them
- Use initialisms consistently: `DB`, `HTTP`, `URL` (not `Db`, `Http`, `Url`)
- No `Get` prefix on getter functions
- Use `any` instead of `interface{}`
- Use modern Go stdlib: `maps`, `slices`, `errors.Is`

## Pull Request Process

1. Fork the repository and create a feature branch from `master`
2. Write tests for new functionality
3. Ensure all checks pass:
   ```sh
   make lint
   make test
   ```
4. Update `config-sample.yml` if configuration options change
5. Submit a pull request against `master`

CI will automatically run linting, unit tests, and integration tests (with a real ClickHouse instance) on your PR.

## Reporting Issues

- Use [GitHub Issues](https://github.com/mintance/nginx-clickhouse/issues)
- Include your Go version, OS, and ClickHouse version
- Include the relevant config (redact credentials) and error logs

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
