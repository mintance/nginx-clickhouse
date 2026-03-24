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
- **JSON access log support** (`log_format escape=json`) alongside traditional text format
- **Log enrichment**: auto-hostname, environment, service tags, status class derivation
- Batch inserts into ClickHouse using the [official Go client](https://github.com/ClickHouse/clickhouse-go) (native TCP, LZ4 compression)
- **Retry with exponential backoff** and full jitter on ClickHouse failures
- **Automatic connection recovery** — reconnects transparently after outages
- **Optional disk buffer** with segment files for crash recovery (at-least-once delivery)
- **Server-side batching** via ClickHouse [async inserts](https://clickhouse.com/docs/en/optimize/asynchronous-inserts) (`async_insert=1, wait_for_async_insert=1`)
- **Circuit breaker** to fast-fail when ClickHouse is persistently down
- **Graceful shutdown** — flushes buffer on SIGTERM/SIGINT
- `/healthz` endpoint for Kubernetes liveness/readiness probes
- Prometheus metrics: buffer size, flush latency, parse errors, circuit breaker state
- Structured JSON logging
- Configuration via YAML file or environment variables
- Minimal Docker image (scratch-based)

## Quick Start

### Using Docker

```sh
docker pull mintance/nginx-clickhouse

docker run --rm --net=host --name nginx-clickhouse \
  -v /var/log/nginx:/logs \
  -v /path/to/config:/config \
  -v /var/lib/nginx-clickhouse:/data \
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

1. On startup, replays any unprocessed disk buffer segments from a previous crash (if disk buffer enabled)
2. Tails the NGINX access log file specified in configuration
3. Buffers incoming log lines in memory or on disk (up to `max_buffer_size`)
4. On a configurable interval (or when the buffer is full), parses the buffered lines using the NGINX log format
5. Batch-inserts parsed entries into ClickHouse via the native TCP protocol, with automatic retry on failure
6. On shutdown (SIGTERM/SIGINT), flushes remaining buffer before exiting

## Configuration

Configuration is loaded from a YAML file (default: `config/config.yml`). All values can be overridden with environment variables.

### Environment Variables

| Variable | Description |
|---|---|
| `LOG_PATH` | Path to NGINX access log file |
| `FLUSH_INTERVAL` | Batch flush interval in seconds |
| `MAX_BUFFER_SIZE` | Max log lines to buffer before forcing a flush (default: `10000`) |
| `RETRY_MAX` | Max retry attempts on ClickHouse failure (default: `3`) |
| `RETRY_BACKOFF_INITIAL` | Initial retry backoff in seconds (default: `1`) |
| `RETRY_BACKOFF_MAX` | Maximum retry backoff in seconds (default: `30`) |
| `BUFFER_TYPE` | Buffer type: `memory` (default) or `disk` |
| `BUFFER_DISK_PATH` | Directory for disk buffer segments |
| `BUFFER_MAX_DISK_BYTES` | Max disk usage for buffer in bytes |
| `CIRCUIT_BREAKER_ENABLED` | Enable circuit breaker (`true`/`false`) |
| `CIRCUIT_BREAKER_THRESHOLD` | Consecutive failures before opening (default: `5`) |
| `CIRCUIT_BREAKER_COOLDOWN` | Seconds before half-open probe (default: `60`) |
| `CLICKHOUSE_HOST` | ClickHouse server hostname |
| `CLICKHOUSE_PORT` | ClickHouse native TCP port (default: `9000`) |
| `CLICKHOUSE_DB` | ClickHouse database name |
| `CLICKHOUSE_TABLE` | ClickHouse table name |
| `CLICKHOUSE_USER` | ClickHouse username |
| `CLICKHOUSE_PASSWORD` | ClickHouse password |
| `CLICKHOUSE_TLS` | Enable TLS (`true`/`false`) |
| `CLICKHOUSE_TLS_SKIP_VERIFY` | Skip TLS certificate verification (`true`/`false`) |
| `CLICKHOUSE_CA_CERT` | Path to CA certificate file |
| `CLICKHOUSE_TLS_CERT_PATH` | Path to client TLS certificate (for mTLS) |
| `CLICKHOUSE_TLS_KEY_PATH` | Path to client TLS private key (for mTLS) |
| `CLICKHOUSE_USE_SERVER_SIDE_BATCHING` | Delegate batching to ClickHouse async inserts (`true`/`false`) |
| `NGINX_LOG_TYPE` | NGINX log format name |
| `NGINX_LOG_FORMAT` | NGINX log format string |
| `NGINX_LOG_FORMAT_TYPE` | Log format type: `text` (default) or `json` |
| `ENRICHMENT_HOSTNAME` | Hostname to add to logs (`auto` for os.Hostname) |
| `ENRICHMENT_ENVIRONMENT` | Environment tag (e.g., `production`) |
| `ENRICHMENT_SERVICE` | Service name tag |

### Full Config Example

See [`config-sample.yml`](config-sample.yml) for a ready-to-use template.

```yaml
settings:
  interval: 5                    # flush interval in seconds
  log_path: /var/log/nginx/access.log
  seek_from_end: false           # start reading from end of file
  max_buffer_size: 10000         # flush when buffer exceeds this (prevents memory issues)
  retry:
    max_retries: 3
    backoff_initial_secs: 1
    backoff_max_secs: 30
  buffer:
    type: memory                 # "memory" (default) or "disk"
    # disk_path: /var/lib/nginx-clickhouse/buffer
    # max_disk_bytes: 1073741824 # 1GB
  circuit_breaker:
    enabled: false
    threshold: 5
    cooldown_secs: 60

clickhouse:
  db: metrics
  table: nginx
  host: localhost
  port: 9000                     # native TCP port (9440 for TLS)
  # tls: true                    # enable TLS
  # tls_insecure_skip_verify: false
  # ca_cert: /etc/ssl/clickhouse-ca.pem
  # tls_cert_path: /etc/ssl/client.crt  # client cert for mTLS
  # tls_key_path: /etc/ssl/client.key
  # use_server_side_batching: false     # delegate batching to ClickHouse async inserts
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
  log_format_type: text        # "text" (default) or "json"
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

## JSON Access Logs

nginx-clickhouse supports NGINX's native JSON log format (`escape=json`) as an alternative to the traditional text format. JSON logs are more robust — no custom regex parsing, no escaping edge cases.

### 1. Configure NGINX JSON Log Format

In `/etc/nginx/nginx.conf`:

```nginx
log_format json_combined escape=json
'{'
  '"remote_addr":"$remote_addr",'
  '"request_method":"$request_method",'
  '"request_uri":"$request_uri",'
  '"status":$status,'
  '"body_bytes_sent":$body_bytes_sent,'
  '"request_time":$request_time,'
  '"http_referer":"$http_referer",'
  '"http_user_agent":"$http_user_agent",'
  '"time_local":"$time_local"'
'}';
```

### 2. Set Log Format Type in Config

```yaml
nginx:
  log_format_type: json
```

With JSON format, the `log_type` and `log_format` fields are not needed. The JSON keys in the log are mapped directly via the `columns` config.

## ClickHouse Setup

Create a table matching your column mapping. This schema uses compression codecs, monthly partitioning, and a 180-day TTL retention policy.

```sql
CREATE TABLE metrics.nginx (
    TimeLocal     DateTime    CODEC(Delta(4), ZSTD(1)),
    Date          Date        DEFAULT toDate(TimeLocal),
    RemoteAddr    IPv4,
    RemoteUser    String      CODEC(ZSTD(1)),
    Request       String      CODEC(ZSTD(1)),
    Status        UInt16,
    BytesSent     UInt64      CODEC(Delta(4), ZSTD(1)),
    HttpReferer   String      CODEC(ZSTD(1)),
    HttpUserAgent String      CODEC(ZSTD(1)),
    Hostname      LowCardinality(String),
    Environment   LowCardinality(String),
    Service       LowCardinality(String),
    StatusClass   LowCardinality(String)
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(Date)
ORDER BY (Status, TimeLocal)
TTL TimeLocal + INTERVAL 180 DAY
SETTINGS ttl_only_drop_parts = 1;
```

**Codec rationale:**

| Column | Codec | Why |
|---|---|---|
| `TimeLocal` | `Delta(4), ZSTD(1)` | Sequential timestamps have small deltas; `Delta(4)` matches DateTime's 4-byte width |
| `BytesSent` | `Delta(4), ZSTD(1)` | Consecutive log entries often have similar response sizes |
| `RemoteAddr` | `IPv4` (native type) | Stored as 4 bytes; avoids string overhead. Use `IPv6` for mixed traffic |
| `Status` | `UInt16` | Already compact at 2 bytes; no codec needed |
| `Hostname`, `Environment`, `Service`, `StatusClass` | `LowCardinality(String)` | Few distinct values — dictionary encoding replaces strings with small integer references |
| `Request`, `HttpReferer`, `HttpUserAgent` | `ZSTD(1)` | High-cardinality strings; general-purpose compression works best |

**Partitioning and TTL:** Monthly partitions (`toYYYYMM`) align well with the 180-day retention. `ttl_only_drop_parts = 1` ensures ClickHouse drops whole parts efficiently rather than performing row-level deletions. Adjust the `INTERVAL 180 DAY` to your retention needs.

## Prometheus Metrics

Available at `http://localhost:2112/metrics`:

| Metric | Description |
|---|---|
| `nginx_clickhouse_lines_processed_total` | Total log lines successfully saved |
| `nginx_clickhouse_lines_not_processed_total` | Total log lines that failed to save |
| `nginx_clickhouse_lines_read_total` | Total lines read from the log file |
| `nginx_clickhouse_parse_errors_total` | Total lines that failed to parse |
| `nginx_clickhouse_buffer_size` | Current number of lines in the buffer |
| `nginx_clickhouse_clickhouse_up` | Whether ClickHouse is reachable (1/0) |
| `nginx_clickhouse_flush_duration_seconds` | Time spent per flush (histogram) |
| `nginx_clickhouse_batch_size` | Number of entries per flush (histogram) |
| `nginx_clickhouse_circuit_breaker_state` | Circuit breaker state (0=closed, 1=open, 2=half-open) |
| `nginx_clickhouse_circuit_breaker_rejections_total` | Flushes rejected by circuit breaker |

## Reliability

### Retry with Backoff

When a ClickHouse write fails, the client retries with exponential backoff and full jitter. Configure via:

- `max_retries` — number of retry attempts (default: 3, set to 0 to disable)
- `backoff_initial_secs` — initial delay between retries (default: 1s)
- `backoff_max_secs` — maximum delay cap (default: 30s)

The backoff doubles each attempt with random jitter to avoid thundering herd.

### TLS / Secure Connections

For ClickHouse Cloud or any TLS-secured cluster:

```yaml
clickhouse:
  host: your-cluster.clickhouse.cloud
  port: 9440
  tls: true
  # tls_insecure_skip_verify: true    # only for self-signed certs
  # ca_cert: /etc/ssl/custom-ca.pem   # custom CA certificate
  # tls_cert_path: /etc/ssl/client.crt  # client certificate (mTLS)
  # tls_key_path: /etc/ssl/client.key   # client private key (mTLS)
```

The default ClickHouse secure native port is `9440`. Set `tls: true` to enable encrypted connections. For mutual TLS (mTLS), provide both `tls_cert_path` and `tls_key_path`.

### Connection Recovery

If the ClickHouse connection drops, it is automatically reset and re-established on the next retry attempt. No manual intervention needed.

### Graceful Shutdown

On `SIGTERM` or `SIGINT`, the service:
1. Flushes any remaining buffered log lines to ClickHouse
2. Closes the ClickHouse connection
3. Exits cleanly

This ensures no data loss during deployments or container restarts.

### Buffer Limits

The in-memory buffer is capped at `max_buffer_size` (default: 10,000 lines). When the buffer is full, it flushes immediately rather than waiting for the next interval.

### Disk Buffer

For crash recovery, enable disk-backed buffering:

```yaml
settings:
  buffer:
    type: disk
    disk_path: /var/lib/nginx-clickhouse/buffer
    max_disk_bytes: 1073741824  # 1GB
```

When enabled, log lines are written to append-only segment files on disk. If the process crashes, unprocessed segments are automatically replayed on restart. This provides at-least-once delivery.

Segment files are rotated at 10MB and deleted after successful flush.

### Circuit Breaker

When ClickHouse is down for extended periods, the circuit breaker prevents wasting resources on retries:

```yaml
settings:
  circuit_breaker:
    enabled: true
    threshold: 5        # open after 5 consecutive failures
    cooldown_secs: 60   # wait 60s before probing
```

States:
- **Closed** (normal): all flushes proceed
- **Open**: flushes are skipped, lines counted as not processed
- **Half-open**: after cooldown, one probe flush is attempted. Success closes the circuit; failure re-opens it.

Monitor via `nginx_clickhouse_circuit_breaker_state` (0=closed, 1=open, 2=half-open) and `nginx_clickhouse_circuit_breaker_rejections_total`.

### Server-Side Batching

By default, nginx-clickhouse batches log lines client-side using its internal buffer. You can optionally delegate batching to ClickHouse's [async inserts](https://clickhouse.com/docs/en/optimize/asynchronous-inserts):

```yaml
clickhouse:
  use_server_side_batching: true
```

When enabled, each batch insert is sent with `async_insert=1` and `wait_for_async_insert=1`. ClickHouse buffers the data server-side and flushes based on its own thresholds (`async_insert_max_data_size`, `async_insert_busy_timeout_ms`). The `wait_for_async_insert=1` setting ensures the insert only returns after the server flush completes, preserving at-least-once delivery guarantees.

Client-side buffering (interval-based flush, `max_buffer_size`) still applies — ClickHouse recommends batching even with async inserts for best throughput. The disk buffer is redundant when server-side batching is enabled (a warning is logged if both are active).

## Enrichments

Enrichments let you automatically inject additional fields into every log entry — hostname, environment, service name, and derived status class — without any changes to the NGINX log format.

Configure enrichments in the `settings` block:

```yaml
settings:
  enrichments:
    hostname: auto
    environment: production
    service: my-api

clickhouse:
  columns:
    Hostname: _hostname
    Environment: _environment
    Service: _service
    StatusClass: _status_class
```

Map enrichment fields to ClickHouse columns using the `_` prefix in the column mapping. Available enrichment fields:

| Field | Description |
|---|---|
| `_hostname` | Hostname of the machine running nginx-clickhouse (`auto` resolves via `os.Hostname()`, or set a literal value) |
| `_environment` | Environment tag (e.g., `production`, `staging`) |
| `_service` | Service name tag |
| `_status_class` | HTTP status class derived from the `status` field (e.g., `2xx`, `4xx`, `5xx`) |
| `_extra.<key>` | Arbitrary key-value pairs from the `enrichments.extra` map |

### Health Check

`GET /healthz` on port 2112 returns:
- `200 OK` — ClickHouse connection is alive
- `503 Service Unavailable` — ClickHouse is unreachable

Use for Kubernetes liveness/readiness probes:

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 2112
  initialDelaySeconds: 10
  periodSeconds: 30
readinessProbe:
  httpGet:
    path: /healthz
    port: 2112
  initialDelaySeconds: 5
  periodSeconds: 10
```

## Logging

All logs are emitted as structured JSON (via logrus), making them easy to parse, ship, and alert on:

```json
{"entries":150,"level":"info","msg":"saved log entries","time":"2026-03-22T12:00:00Z"}
{"error":"connection refused","level":"error","msg":"can't save logs","time":"2026-03-22T12:00:05Z"}
```

## Grafana Dashboard

A pre-built Grafana dashboard is included in [`grafana/dashboard.json`](grafana/dashboard.json).

**Requirements:** [Official Grafana ClickHouse plugin](https://grafana.com/grafana/plugins/grafana-clickhouse-datasource/) (v4.0+), Grafana 10+.

**Import:** Grafana > Dashboards > Import > Upload JSON file. Set the `Database` and `Table` template variables to match your config.

**Panels (16):**

| Row | Panels |
|---|---|
| **Overview** | Total Requests, RPS, Error Rate %, Avg Response Time, P95, P99 |
| **Traffic** | Requests by Status Class (stacked bar), Requests by Method (donut) |
| **Performance** | Response Time (avg/p95/p99), Error Rate Over Time, Bandwidth, RPS Over Time |
| **Top N** | Top 10 URLs, Top 10 Client IPs, Top 10 User Agents |
| **Logs** | Slow & Error Requests table (status >= 400 or response time > 1s) |

Template variables: `database` (default: `metrics`), `table` (default: `nginx`).

## Config Validation

Run `--check` to validate your configuration without starting the service:

```sh
./nginx-clickhouse -config_path=config/config.yml --check
```

Output:
```
✓ Config loaded
✓ Log format: JSON
✓ Log file: /var/log/nginx/access.log
✓ ClickHouse connection: OK (localhost:9000)
✓ Database: OK ("metrics" exists)
✓ Table: OK ("metrics.nginx" exists)
✓ Columns: OK (8/8 columns match)

All checks passed.
```

Validates: config syntax, log file existence, ClickHouse connectivity, database/table existence, and column mapping against the actual table schema. Exits with code 1 if any check fails.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, code style, and pull request guidelines.

## License

[Apache License 2.0](LICENSE)
