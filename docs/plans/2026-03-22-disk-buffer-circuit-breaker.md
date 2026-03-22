# Disk Buffer & Circuit Breaker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan.

**Goal:** Add optional disk-backed buffering for crash recovery and a circuit breaker to fast-fail when ClickHouse is down for extended periods.

**Architecture:** New `buffer` package with a `Buffer` interface and two implementations: `MemoryBuffer` (current behavior) and `DiskBuffer` (segment-file based). New `circuitbreaker` package with a simple consecutive-failure counter. Both are optional and configurable.

**Tech Stack:** Go 1.25, os/bufio for file I/O, no external dependencies.

---

### File Map

| File | Action | Responsibility |
|---|---|---|
| `buffer/buffer.go` | Create | Buffer interface + MemoryBuffer implementation |
| `buffer/disk.go` | Create | DiskBuffer: segment-file append, read, cleanup, replay |
| `buffer/buffer_test.go` | Create | Tests for MemoryBuffer |
| `buffer/disk_test.go` | Create | Tests for DiskBuffer (uses t.TempDir) |
| `circuitbreaker/circuitbreaker.go` | Create | CircuitBreaker with consecutive failure tracking |
| `circuitbreaker/circuitbreaker_test.go` | Create | Tests for state transitions |
| `config/config.go` | Modify | Add BufferConfig and CircuitBreakerConfig |
| `config/config_test.go` | Modify | Tests for new config fields |
| `main.go` | Modify | Wire buffer and circuit breaker |
| `clickhouse/clickhouse.go` | Modify | Accept circuit breaker check |
| `config-sample.yml` | Modify | Add buffer and circuit_breaker sections |
| `README.md` | Modify | Document new features |

---

### Task 1: Buffer interface + MemoryBuffer

Create `buffer/buffer.go` with:
- `Buffer` interface: `Write(line string) error`, `ReadAll() ([]string, error)`, `Replay() ([]string, error)`
- `MemoryBuffer` struct with mu, lines, maxSize
- `NewMemoryBuffer(maxSize int) *MemoryBuffer`
- `Write` appends to slice, returns error if full
- `ReadAll` drains the slice (returns lines, clears buffer)
- `Replay` returns nil (nothing to replay from memory)
- `Len() int` — current buffer size

Tests: TestMemoryBufferWrite, TestMemoryBufferReadAll, TestMemoryBufferFull, TestMemoryBufferReplay

---

### Task 2: DiskBuffer with segment files

Create `buffer/disk.go` with:
- `DiskBuffer` struct with mu, dir, maxSize, currentSegment *os.File, segmentCounter, linesInSegment
- `NewDiskBuffer(dir string, maxDiskBytes int64) (*DiskBuffer, error)` — creates dir if needed
- `Write(line string)` — append line + "\n" to current segment file. Rotate segment at ~10MB.
- `ReadAll() ([]string, error)` — read all segment files in order, delete them, return lines
- `Replay() ([]string, error)` — on startup, read any existing segment files (crash recovery)
- `Len() int` — approximate count
- Segment files named `segment-NNNNNN.log` (zero-padded counter)

Tests: TestDiskBufferWriteRead, TestDiskBufferReplay, TestDiskBufferRotation, TestDiskBufferEmpty

---

### Task 3: Circuit breaker

Create `circuitbreaker/circuitbreaker.go` with:
- `CircuitBreaker` struct with mu, failures, threshold, cooldown, openedAt
- States: Closed (normal), Open (failing, skip writes), HalfOpen (probe with one attempt)
- `NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker`
- `Allow() bool` — returns true if Closed or HalfOpen (cooldown elapsed)
- `RecordSuccess()` — reset failures to 0, set Closed
- `RecordFailure()` — increment failures, if >= threshold set Open + openedAt
- `State() string` — "closed", "open", "half-open"

Tests: TestCircuitBreakerClosed, TestCircuitBreakerOpens, TestCircuitBreakerHalfOpen, TestCircuitBreakerResets

---

### Task 4: Config for buffer and circuit breaker

Add to config/config.go:
```go
type BufferConfig struct {
    Type         string `yaml:"type"`           // "memory" or "disk"
    DiskPath     string `yaml:"disk_path"`
    MaxDiskBytes int64  `yaml:"max_disk_bytes"`
}

type CircuitBreakerConfig struct {
    Enabled      bool `yaml:"enabled"`
    Threshold    int  `yaml:"threshold"`
    CooldownSecs int  `yaml:"cooldown_secs"`
}
```

Add to SettingsConfig: `Buffer BufferConfig`, `CircuitBreaker CircuitBreakerConfig`
Add env vars: `BUFFER_TYPE`, `BUFFER_DISK_PATH`, `BUFFER_MAX_DISK_BYTES`, `CIRCUIT_BREAKER_ENABLED`, `CIRCUIT_BREAKER_THRESHOLD`, `CIRCUIT_BREAKER_COOLDOWN`

---

### Task 5: Wire everything in main.go

- Create buffer based on config (memory or disk)
- On startup: call buffer.Replay() and flush any recovered lines
- Replace `var logs []string` with the buffer interface
- Create circuit breaker if enabled
- In flush: check circuit breaker before Save, record success/failure
- Add Prometheus gauge: `nginx_clickhouse_circuit_breaker_state` (0=closed, 1=open, 2=half-open)
- Add Prometheus counter: `nginx_clickhouse_circuit_breaker_rejections_total`

---

### Task 6: Update README and config-sample.yml

Document buffer config, circuit breaker config, env vars, behavior.
