# nginx-clickhouse

[![License: Apache 2](https://img.shields.io/hexpm/l/plug.svg)](https://github.com/mintance/nginx-clickhouse/blob/master/LICENSE)
![Go Version](https://img.shields.io/badge/go-1.25%2B-blue.svg)
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
make build
./nginx-clickhouse -config_path=config/config.yml
```

### Build Docker Image

No local Go toolchain required -- builds inside Docker using multi-stage build.

```sh
make docker
```

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

A pre-built Grafana dashboard is included in `Grafana-dashboard.json`.

![Grafana Dashboard](https://github.com/mintance/nginx-clickhouse/blob/master/grafana.png)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup and guidelines.

## License

[Apache License 2.0](LICENSE)
