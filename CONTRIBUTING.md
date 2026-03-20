# Contributing to nginx-clickhouse

Thank you for your interest in contributing! Here's how to get started.

## Development Setup

### Prerequisites

- Go 1.25+
- Docker (for integration tests)

### Clone and Build

```sh
git clone https://github.com/mintance/nginx-clickhouse.git
cd nginx-clickhouse
make build
```

### Run Tests

```sh
# Unit tests
go test ./... -v -race

# Integration tests (requires ClickHouse running on localhost:9000)
docker run -d --name clickhouse-test -p 9000:9000 clickhouse/clickhouse-server:latest
go test ./clickhouse/ -v -race -tags integration

# Clean up
docker rm -f clickhouse-test
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

## Project Structure

```
main.go              Entry point, log tailing, flush loop
config/config.go     YAML config parsing, env var overrides
nginx/nginx.go       NGINX log format parsing
clickhouse/clickhouse.go   ClickHouse batch insert via native TCP
```

## Pull Request Process

1. Fork the repository and create a feature branch from `master`
2. Write tests for new functionality
3. Ensure all tests pass: `go test ./... -race`
4. Ensure code is formatted: `gofmt -l .` should show no project files
5. Ensure no vet issues: `go vet ./...`
6. Update `config-sample.yml` if configuration options change
7. Submit a pull request against `master`

CI will automatically run unit tests and integration tests (with a real ClickHouse instance) on your PR.

## Reporting Issues

- Use [GitHub Issues](https://github.com/mintance/nginx-clickhouse/issues)
- Include your Go version, OS, and ClickHouse version
- Include the relevant config (redact credentials) and error logs

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
