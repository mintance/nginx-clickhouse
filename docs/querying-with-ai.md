# Querying NGINX Logs with an AI Assistant

nginx-clickhouse only writes data, and querying is ClickHouse's job. To ask an AI assistant (Claude Code, Claude Desktop, or any MCP client) questions like *"which endpoints started returning 5xx in the last hour?"*, connect it to ClickHouse using the official [ClickHouse MCP server](https://github.com/ClickHouse/mcp-clickhouse). Then give it the schema notes below so it writes correct queries for this table.

## 1. Create a Read-Only User

Use a dedicated user that can only read the logs table. The user's grants are the real security boundary; the MCP server's own read-only mode is a second layer.

```sql
CREATE USER ai_reader IDENTIFIED BY 'change-me'
    SETTINGS max_execution_time = 30, max_result_rows = 10000, result_overflow_mode = 'break';

GRANT SELECT ON metrics.nginx TO ai_reader;
```

The settings cap runaway queries: 30s per query, and results are truncated at 10,000 rows.

## 2. Connect the MCP Server

> **Port gotcha:** nginx-clickhouse writes over the **native TCP** protocol (port `9000` / `9440`), but the ClickHouse MCP server speaks the **HTTP** interface (port `8123`, or `8443` with TLS). Make sure the HTTP port is reachable from wherever the assistant runs. Pointing the MCP server at `9000` fails with `Port 9000 is for clickhouse-client program`.

The server runs with [uv](https://docs.astral.sh/uv/) and is read-only by default (`CLICKHOUSE_ALLOW_WRITE_ACCESS=false`).

**Claude Code:**

```sh
claude mcp add clickhouse \
  -e CLICKHOUSE_HOST=clickhouse.internal \
  -e CLICKHOUSE_PORT=8123 \
  -e CLICKHOUSE_SECURE=false \
  -e CLICKHOUSE_USER=ai_reader \
  -e CLICKHOUSE_PASSWORD=change-me \
  -e CLICKHOUSE_DATABASE=metrics \
  -- uv run --with mcp-clickhouse --python 3.12 mcp-clickhouse
```

**Claude Desktop** (`~/Library/Application Support/Claude/claude_desktop_config.json`), or any client that uses the `mcpServers` format:

```json
{
  "mcpServers": {
    "clickhouse": {
      "command": "uv",
      "args": ["run", "--with", "mcp-clickhouse", "--python", "3.12", "mcp-clickhouse"],
      "env": {
        "CLICKHOUSE_HOST": "clickhouse.internal",
        "CLICKHOUSE_PORT": "8123",
        "CLICKHOUSE_SECURE": "false",
        "CLICKHOUSE_USER": "ai_reader",
        "CLICKHOUSE_PASSWORD": "change-me",
        "CLICKHOUSE_DATABASE": "metrics"
      }
    }
  }
}
```

With TLS, set `CLICKHOUSE_SECURE=true` and `CLICKHOUSE_PORT=8443`, plus `CLICKHOUSE_CA_CERT` for a private CA. See the [mcp-clickhouse README](https://github.com/ClickHouse/mcp-clickhouse) for every option, including running it as a shared HTTP server.

## 3. Give the Assistant the Schema Notes

The assistant can discover column names by itself, but not what they *mean*. Paste the block below into your project's `CLAUDE.md` or the assistant's instructions, and adjust it to match your column mapping and filters.

````markdown
## NGINX access logs in ClickHouse (`metrics.nginx`)

Written by nginx-clickhouse. One row per request. Query via the `clickhouse` MCP server.

Columns (only the ones mapped in nginx-clickhouse's `columns` config are populated):
- `TimeLocal` DateTime: request time, in the ClickHouse server's time zone. `Date` = toDate(TimeLocal).
- `Request` String: the raw request line, `"METHOD /path?query HTTP/1.1"`. Path without query:
  `splitByChar('?', splitByChar(' ', Request)[2])[1]`. Method: `splitByChar(' ', Request)[1]`.
- `Status` UInt16, `StatusClass` String (`'2xx'`…`'5xx'`).
- `RequestTime` Float32: seconds, not milliseconds.
- `BytesSent` UInt64, `RemoteAddr` IPv4, `HttpReferer`, `HttpUserAgent`.
- `Hostname`, `Environment`, `Service`: which server/deployment produced the row.
- `RefererDomain`: '' when there is no referer. `UrlExtension`: `'js'`, `'css'`, … or '' for none.
- `IsBot` UInt8 (1 = bot), `BotName` (e.g. 'Googlebot', 'GPTBot'),
  `BotClass` ('search', 'social', 'ai', 'seo-tool', 'monitor', 'scraper').
- `Browser`, `BrowserVersion` (major only), `OS`, `DeviceType` ('desktop', 'mobile', 'tablet', …).

Query rules:
- Always filter on BOTH `Date` (partition pruning) and `TimeLocal`, e.g.
  `WHERE Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 HOUR`.
- The table is ordered by `(Status, TimeLocal)`, so filtering on `Status` is cheap.
- Use `quantile(0.95)(RequestTime)` for latency, never avg alone.
- Retention is 180 days (TTL).
- Sampling: <describe your filters, e.g. "health checks are dropped; only 10% of 2xx are stored,
  so multiply 2xx counts by 10 and compute error rates with that correction">.
````

The sampling line matters most. If your `filters` drop or sample traffic, raw `count()` values and error rates come out skewed, and the assistant can't know that unless you tell it.

## Example Queries

These assume the schema from the [README](../README.md#clickhouse-setup). They make a good starting set to add to the schema notes, or to ask the assistant to adapt.

**5xx error rate, 5-minute buckets, last hour:**

```sql
SELECT
    toStartOfFiveMinutes(TimeLocal) AS t,
    count() AS requests,
    countIf(Status >= 500) AS errors,
    round(100 * errors / requests, 2) AS error_pct
FROM metrics.nginx
WHERE Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 HOUR
GROUP BY t
ORDER BY t;
```

**Endpoints with the most 5xx, last 24 hours:**

```sql
SELECT
    splitByChar('?', splitByChar(' ', Request)[2])[1] AS path,
    count() AS errors,
    groupUniqArray(5)(Status) AS statuses
FROM metrics.nginx
WHERE Status >= 500 AND Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 DAY
GROUP BY path
ORDER BY errors DESC
LIMIT 10;
```

**Slowest endpoints by p95, ignoring rarely-hit paths:**

```sql
SELECT
    splitByChar('?', splitByChar(' ', Request)[2])[1] AS path,
    count() AS requests,
    round(quantile(0.5)(RequestTime), 3) AS p50,
    round(quantile(0.95)(RequestTime), 3) AS p95,
    round(quantile(0.99)(RequestTime), 3) AS p99
FROM metrics.nginx
WHERE Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 DAY
GROUP BY path
HAVING requests >= 100
ORDER BY p95 DESC
LIMIT 10;
```

**Bot vs. human traffic share:**

```sql
SELECT
    if(IsBot = 1, BotClass, 'human') AS class,
    count() AS requests,
    round(100 * count() / sum(count()) OVER (), 2) AS pct
FROM metrics.nginx
WHERE Date >= today() - 7
GROUP BY class
ORDER BY requests DESC;
```

**AI crawlers and how much they download:**

```sql
SELECT
    BotName,
    count() AS requests,
    formatReadableSize(sum(BytesSent)) AS downloaded
FROM metrics.nginx
WHERE BotClass = 'ai' AND Date >= today() - 7
GROUP BY BotName
ORDER BY requests DESC;
```

**Top external referrers (humans only):**

```sql
SELECT RefererDomain, count() AS requests
FROM metrics.nginx
WHERE RefererDomain != '' AND IsBot = 0 AND Date >= today() - 7
GROUP BY RefererDomain
ORDER BY requests DESC
LIMIT 20;
```

**Is one host misbehaving?**

```sql
SELECT
    Hostname,
    count() AS requests,
    round(100 * countIf(Status >= 500) / requests, 2) AS error_pct,
    round(quantile(0.95)(RequestTime), 3) AS p95
FROM metrics.nginx
WHERE Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 HOUR
GROUP BY Hostname
ORDER BY error_pct DESC;
```

**Bandwidth by file type:**

```sql
SELECT
    if(UrlExtension = '', '(none)', UrlExtension) AS ext,
    count() AS requests,
    formatReadableSize(sum(BytesSent)) AS bytes
FROM metrics.nginx
WHERE Date >= today() - 1
GROUP BY ext
ORDER BY sum(BytesSent) DESC
LIMIT 15;
```

**Device and browser mix (humans only):**

```sql
SELECT DeviceType, Browser, count() AS requests
FROM metrics.nginx
WHERE IsBot = 0 AND Date >= today() - 7
GROUP BY DeviceType, Browser
ORDER BY requests DESC
LIMIT 20;
```

**Most recent slow requests:**

```sql
SELECT TimeLocal, Hostname, Status, RequestTime, Request
FROM metrics.nginx
WHERE RequestTime > 1 AND Date >= today() - 1 AND TimeLocal >= now() - INTERVAL 1 HOUR
ORDER BY TimeLocal DESC
LIMIT 20;
```

## Debugging the Pipeline Itself

To find out why lines aren't arriving, you don't need MCP:

- **Ingest health:** the Prometheus metrics on `:2112/metrics` (parse errors, filtered lines, buffer size, circuit breaker state) and the [Grafana dashboard](../README.md#grafana-dashboard).
- **Parsing and mapping:** pipe sample lines through [`-parse`](../README.md#parse-preview--parse) to see exactly what each line becomes and which filter rule drops it. Coding assistants can run it from the shell too.
