# Reliability & Observability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make nginx-clickhouse production-ready with retry logic, connection recovery, graceful shutdown, health endpoints, expanded metrics, and structured logging.

**Architecture:** Extract the ClickHouse client into a `Client` struct that owns the connection and retry logic. Add a `retry` package for reusable exponential backoff. Wire graceful shutdown via `os/signal` in main. Register `/healthz` on the existing HTTP server. Switch logrus to JSON formatter with structured fields.

**Tech Stack:** Go 1.25, logrus (JSON formatter), prometheus/client_golang, os/signal, math/rand/v2

---

### File Map

| File | Action | Responsibility |
|---|---|---|
| `retry/retry.go` | Create | Exponential backoff with jitter (generic, testable) |
| `retry/retry_test.go` | Create | Unit tests for backoff calculation and retry loop |
| `clickhouse/clickhouse.go` | Rewrite | `Client` struct: connection mgmt, health check, retry-wrapped Save |
| `clickhouse/clickhouse_test.go` | Rewrite | Unit tests for Client methods, connection recovery, empty guards |
| `clickhouse/integration_test.go` | Modify | Update to use new Client API |
| `config/config.go` | Modify | Add `RetryConfig` struct (max_retries, backoff_initial, backoff_max) |
| `config/config_test.go` | Modify | Test new retry config fields |
| `main.go` | Rewrite | Graceful shutdown, /healthz, structured logging, new Client usage |
| `README.md` | Modify | New config options, /healthz docs, reliability section |
| `config-sample.yml` | Modify | Add retry config example |

---

### Task 1: Retry package with exponential backoff + jitter

**Files:**
- Create: `retry/retry.go`
- Create: `retry/retry_test.go`

- [ ] **Step 1: Write retry_test.go with tests for backoff calculation**

Test that `Backoff(attempt, initial, max)` returns exponentially increasing durations capped at max, and that `Do()` retries the correct number of times.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./retry/ -v`

- [ ] **Step 3: Implement retry.go**

```go
// Package retry provides exponential backoff with full jitter.
package retry

// Backoff returns a duration for the given attempt using exponential backoff
// with full jitter: random value in [0, min(initial * 2^attempt, maxDelay)].
func Backoff(attempt int, initial, maxDelay time.Duration) time.Duration

// Do calls fn up to maxRetries times. If fn returns nil, Do returns nil.
// Between attempts it sleeps for Backoff(attempt, initial, max) with jitter.
// If all attempts fail, returns the last error.
func Do(maxRetries int, initial, maxDelay time.Duration, fn func() error) error
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./retry/ -v -race`

- [ ] **Step 5: Commit**

```
git add retry/
git commit -m "feat: add retry package with exponential backoff and jitter"
```

---

### Task 2: Retry config fields

**Files:**
- Modify: `config/config.go`
- Modify: `config/config_test.go`
- Modify: `config-sample.yml`

- [ ] **Step 1: Write test for retry config parsing**

Add test that reads YAML with `retry` section and verifies `MaxRetries`, `BackoffInitial`, `BackoffMax` fields.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./config/ -run TestReadRetryConfig -v`

- [ ] **Step 3: Add RetryConfig to config.go**

Add `RetryConfig` struct with `MaxRetries int`, `BackoffInitialSecs int`, `BackoffMaxSecs int` to `SettingsConfig`. Add env var overrides: `RETRY_MAX`, `RETRY_BACKOFF_INITIAL`, `RETRY_BACKOFF_MAX`.

- [ ] **Step 4: Run all config tests**

Run: `go test ./config/ -v -race`

- [ ] **Step 5: Update config-sample.yml**

- [ ] **Step 6: Commit**

```
git add config/ config-sample.yml
git commit -m "feat: add retry configuration (max_retries, backoff)"
```

---

### Task 3: ClickHouse Client with connection recovery and retry

**Files:**
- Rewrite: `clickhouse/clickhouse.go`
- Rewrite: `clickhouse/clickhouse_test.go`
- Modify: `clickhouse/integration_test.go`

- [ ] **Step 1: Write tests for new Client**

Test: `NewClient` creates a client, `Client.Healthy()` returns bool, `Client.Save()` with empty logs returns nil, `Client.resetConn()` clears cached connection.

- [ ] **Step 2: Run tests to verify they fail**

- [ ] **Step 3: Rewrite clickhouse.go with Client struct**

```go
// Client manages the ClickHouse connection with automatic reconnection
// and retry logic.
type Client struct {
    cfg  *config.Config
    conn driver.Conn
    mu   sync.Mutex
}

// NewClient creates a new ClickHouse client.
func NewClient(cfg *config.Config) *Client

// Save batch-inserts entries with retry. On connection failure, resets
// the cached connection and retries.
func (c *Client) Save(entries []gonx.Entry) error

// Healthy reports whether the ClickHouse connection is alive.
func (c *Client) Healthy() bool

// Close closes the underlying connection.
func (c *Client) Close() error
```

Key behaviors:
- `Save` wraps the insert in `retry.Do()` using config retry settings
- On any error, call `resetConn()` to clear the cached connection before next retry
- `Healthy()` pings ClickHouse, used by /healthz

- [ ] **Step 4: Run unit tests**

Run: `go test ./clickhouse/ -v -race`

- [ ] **Step 5: Update integration_test.go to use Client**

- [ ] **Step 6: Commit**

```
git add clickhouse/
git commit -m "feat: ClickHouse Client with connection recovery and retry"
```

---

### Task 4: Expanded Prometheus metrics

**Files:**
- Modify: `main.go` (metrics declarations)
- Create or keep inline in main.go

- [ ] **Step 1: Define new metrics**

Add to existing metrics:
```go
// Counters
linesRead        // total lines read from log file
parseErrors      // total lines that failed to parse
retriesTotal     // total retry attempts

// Gauges
bufferSize       // current number of lines in buffer
clickhouseUp     // 1 if ClickHouse is reachable, 0 if not

// Histograms
flushDuration    // seconds per flush (parse + save)
batchSize        // number of entries per flush
```

- [ ] **Step 2: Wire metrics into flush() and line-reading loop**

- [ ] **Step 3: Commit**

```
git commit -m "feat: expand Prometheus metrics (buffer, latency, retries)"
```

---

### Task 5: Structured JSON logging

**Files:**
- Modify: `main.go` (init logging)
- Modify: all files using logrus (use `WithFields`)

- [ ] **Step 1: Set logrus JSON formatter in main**

```go
logrus.SetFormatter(&logrus.JSONFormatter{})
```

- [ ] **Step 2: Convert key log calls to use WithFields**

Replace string concatenation with structured fields:
```go
logrus.WithFields(logrus.Fields{
    "entries": len(batch),
}).Info("preparing to save log entries")
```

- [ ] **Step 3: Run tests to verify nothing broke**

Run: `go test ./... -race`

- [ ] **Step 4: Commit**

```
git commit -m "feat: switch to structured JSON logging"
```

---

### Task 6: Graceful shutdown + /healthz endpoint

**Files:**
- Modify: `main.go`

- [ ] **Step 1: Add /healthz endpoint**

Register on the existing HTTP mux. Returns 200 if `client.Healthy()`, 503 otherwise.

```go
http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
    if client.Healthy() {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("ok"))
    } else {
        w.WriteHeader(http.StatusServiceUnavailable)
        w.Write([]byte("clickhouse unreachable"))
    }
})
```

- [ ] **Step 2: Add signal handler for graceful shutdown**

Catch SIGTERM/SIGINT, flush remaining buffer, close ClickHouse client, then exit.

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
```

- [ ] **Step 3: Run full test suite**

Run: `go test ./... -v -race`

- [ ] **Step 4: Commit**

```
git commit -m "feat: add /healthz endpoint and graceful shutdown"
```

---

### Task 7: Update README

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add Reliability section**

Document retry behavior, connection recovery, graceful shutdown, buffer limits.

- [ ] **Step 2: Add /healthz to docs**

Document health endpoint for Kubernetes probes.

- [ ] **Step 3: Update config example and env var table**

Add retry config options: `RETRY_MAX`, `RETRY_BACKOFF_INITIAL`, `RETRY_BACKOFF_MAX`.

- [ ] **Step 4: Commit**

```
git commit -m "docs: update README with reliability and observability features"
```
