# nginx-clickhouse

[![Share on X](https://img.shields.io/badge/share-000000?logo=x&logoColor=white)](https://x.com/intent/tweet?text=Simple%20NGINX%20logs%20parser%20and%20transporter%20to%20ClickHouse%20database.&url=https://github.com/mintance/nginx-clickhouse&hashtags=nginx,clickhouse,golang)
[![Share on Reddit](https://img.shields.io/badge/share-FF4500?logo=reddit&logoColor=white)](https://www.reddit.com/submit?url=https://github.com/mintance/nginx-clickhouse&title=nginx-clickhouse%20-%20NGINX%20logs%20to%20ClickHouse)

[![Go Reference](https://pkg.go.dev/badge/github.com/mintance/nginx-clickhouse.svg)](https://pkg.go.dev/github.com/mintance/nginx-clickhouse)
[![Go Report Card](https://goreportcard.com/badge/github.com/mintance/nginx-clickhouse)](https://goreportcard.com/report/github.com/mintance/nginx-clickhouse)
[![CI](https://github.com/mintance/nginx-clickhouse/actions/workflows/test.yml/badge.svg)](https://github.com/mintance/nginx-clickhouse/actions/workflows/test.yml)
[![License: Apache 2](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](https://github.com/mintance/nginx-clickhouse/blob/master/LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/mintance/nginx-clickhouse.svg)](https://hub.docker.com/r/mintance/nginx-clickhouse/)
[![GitHub issues](https://img.shields.io/github/issues/mintance/nginx-clickhouse.svg)](https://github.com/mintance/nginx-clickhouse/issues)

Simple NGINX access log parser and transporter to ClickHouse database. Uses the native TCP protocol for fast, compressed batch inserts.

## Features

- Tails NGINX access logs in real-time
- Configurable log format parsing via [gonx](https://github.com/satyrius/gonx)
- Batch inserts into ClickHouse using the [official Go client](https://github.com/ClickHouse/clickhouse-go) (native TCP, LZ4 compression)
- Prometheus metrics endpoint on `:2112`
- Configuration via YAML file or environment variables
- Minimal Docker image (scratch-based)

## Quick Start

### Using Docker

```sh
docker pull mintance/nginx-clickhouse

docker run --rm --net=host --name nginx-clickhouse \
  -v /var/log/nginx:/logs \
  -v /path/to/config:/config \
  -d mintance/nginx-clickhouse
```

### Build from Source

Requires Go 1.25+.

```sh
go build -o nginx-clickhouse .
./nginx-clickhouse -config_path=config/config.yml
```

### Build Docker Image

No local Go toolchain required -- builds inside Docker using multi-stage build.

```sh
make docker
```

## How It Works

1. Tails the NGINX access log file specified in configuration
2. Buffers incoming log lines in memory
3. On a configurable interval, parses the buffered lines using the NGINX log format
4. Batch-inserts parsed entries into ClickHouse via the native TCP protocol

## Configuration

Configuration is loaded from a YAML file (default: `config/config.yml`). All values can be overridden with environment variables.

### Environment Variables

| Variable | Description |
|---|---|
| `LOG_PATH` | Path to NGINX access log file |
| `FLUSH_INTERVAL` | Batch flush interval in seconds |
| `CLICKHOUSE_HOST` | ClickHouse server hostname |
| `CLICKHOUSE_PORT` | ClickHouse native TCP port (default: `9000`) |
| `CLICKHOUSE_DB` | ClickHouse database name |
| `CLICKHOUSE_TABLE` | ClickHouse table name |
| `CLICKHOUSE_USER` | ClickHouse username |
| `CLICKHOUSE_PASSWORD` | ClickHouse password |
| `NGINX_LOG_TYPE` | NGINX log format name |
| `NGINX_LOG_FORMAT` | NGINX log format string |

### Full Config Example

See [`config-sample.yml`](config-sample.yml) for a ready-to-use template.

```yaml
settings:
  interval: 5                    # flush interval in seconds
  log_path: /var/log/nginx/access.log
  seek_from_end: false           # start reading from end of file

clickhouse:
  db: metrics
  table: nginx
  host: localhost
  port: 9000                     # native TCP port
  credentials:
    user: default
    password:
  columns:                       # ClickHouse column -> NGINX variable mapping
    RemoteAddr: remote_addr
    RemoteUser: remote_user
    TimeLocal: time_local
    Request: request
    Status: status
    BytesSent: bytes_sent
    HttpReferer: http_referer
    HttpUserAgent: http_user_agent

nginx:
  log_type: main
  log_format: '$remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"'
```

## NGINX Setup

### 1. Define a Log Format

In `/etc/nginx/nginx.conf`:

```nginx
http {
    log_format main '$remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"';
}
```

### 2. Enable Access Log

In your site config (`/etc/nginx/sites-enabled/my-site.conf`):

```nginx
server {
    access_log /var/log/nginx/my-site-access.log main;
}
```

## ClickHouse Setup

Create a table matching your column mapping:

```sql
CREATE TABLE metrics.nginx (
    RemoteAddr    String,
    RemoteUser    String,
    TimeLocal     DateTime,
    Date          Date DEFAULT toDate(TimeLocal),
    Request       String,
    Status        Int32,
    BytesSent     Int64,
    HttpReferer   String,
    HttpUserAgent String
) ENGINE = MergeTree()
ORDER BY (Status, TimeLocal)
```

## Prometheus Metrics

Available at `http://localhost:2112/metrics`:

| Metric | Description |
|---|---|
| `nginx_clickhouse_lines_processed_total` | Total log lines successfully saved |
| `nginx_clickhouse_lines_not_processed_total` | Total log lines that failed to save |

## Grafana Dashboard

A pre-built Grafana dashboard is included in [`grafana/dashboard.json`](grafana/dashboard.json). Import it into Grafana to visualize your NGINX metrics.

![Grafana Dashboard](grafana/dashboard.png)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, code style, and pull request guidelines.

## License

[Apache License 2.0](LICENSE)
