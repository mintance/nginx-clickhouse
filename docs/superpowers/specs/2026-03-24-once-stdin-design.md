# Design: `-once` and `-stdin` input modes

## Problem

nginx-clickhouse only supports tailing a log file continuously. Users cannot:
- Bulk-load a historical log file and exit
- Pipe logs from stdin (e.g., `zcat`, `journalctl -f`, other tools)

## Solution

Add two new CLI flags that change the line source while reusing the existing buffer/flush/parse/save pipeline.

## Flags

### `-once`

Read the log file from start to EOF, flush remaining buffer, exit.

- Always reads from the beginning of the file (ignores `seek_from_end`)
- Uses the existing buffer/flush pipeline — processes the file in chunks, handles large files
- After EOF: triggers a final flush, closes client, exits 0
- No YAML config — operational flag only

### `-stdin`

Read from `os.Stdin` continuously, feeding lines into the buffer/flush pipeline.

- Supports both piped input (`zcat log.gz | nginx-clickhouse -stdin`) and streaming (`journalctl -f | nginx-clickhouse -stdin`)
- After stdin EOF: triggers a final flush, closes client, exits 0
- SIGTERM/SIGINT still work for early termination
- No YAML config — operational flag only

## Priority order

`--check` > `-stdin` > `-once` > normal tail mode

If both `-stdin` and `-once` are set, `-stdin` takes priority and a warning is logged.

## Architecture

### Line source abstraction

All three modes (tail, once, stdin) produce lines into the same pipeline. Extract a `lineSource` that returns a `<-chan string`:

- **Tail mode**: wraps existing `follower.New()` — follows file, reopens on rotation
- **Once mode**: opens file with `bufio.Scanner`, sends lines, closes channel at EOF
- **Stdin mode**: `bufio.Scanner(os.Stdin)`, sends lines, closes channel at EOF

The main loop becomes:

```go
lines := newLineSource(cfg, onceMode, stdinMode)
for line := range lines {
    // existing buffer/flush logic (unchanged)
}
// EOF reached — final flush, shutdown
```

### Shutdown on EOF

When the line source channel closes (once/stdin modes), the main goroutine:
1. Falls out of the `for range` loop
2. Calls `flush()` one final time
3. Closes the ClickHouse client
4. Closes the disk buffer if applicable
5. Exits 0

This reuses the same shutdown logic as SIGTERM, just triggered by EOF instead of a signal.

### What does NOT change

- `config/` — no new config fields
- `clickhouse/` — no changes to Save or connection logic
- `nginx/` — no changes to parsing
- `buffer/` — no changes to buffering
- `retry/` — no changes
- `circuitbreaker/` — no changes
- Disk buffer replay still runs before any mode
- `--check` still takes highest priority

## Testing

### Unit tests

- `-once`: create a temp file with known lines, verify they reach the buffer and the process completes
- `-stdin`: create a pipe, write lines, close it, verify lines reach the buffer and the process completes
- Priority: when both `-stdin` and `-once` are set, `-stdin` wins

### Integration tests

Not needed — these modes only change the line source, not the ClickHouse interaction. Existing integration tests cover `Save()`.

## README updates

- Add `-once` and `-stdin` to the Features list
- Add a "Bulk Loading & Stdin" section with usage examples:
  ```sh
  # Load a historical log file and exit
  ./nginx-clickhouse -once -config_path=config/config.yml

  # Pipe a compressed log
  zcat /var/log/nginx/access.log.1.gz | ./nginx-clickhouse -stdin

  # Stream from journald
  journalctl -f -u nginx --output cat | ./nginx-clickhouse -stdin
  ```

## CLAUDE.md updates

- Update test counts after implementation
