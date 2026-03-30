// Package clickhouse handles batch-inserting parsed NGINX log entries into
// ClickHouse using the native TCP protocol.
package clickhouse

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
	"github.com/mintance/nginx-clickhouse/retry"
)

// Client manages the ClickHouse connection with automatic reconnection
// and retry logic.
type Client struct {
	cfg  *config.Config
	conn driver.Conn
	mu   sync.Mutex
}

// NewClient creates a Client for the given configuration. The actual
// connection to ClickHouse is established lazily on the first call to Save.
func NewClient(cfg *config.Config) *Client {
	return &Client{cfg: cfg}
}

// Save batch-inserts the parsed log entries into ClickHouse. It retries
// transient failures using exponential backoff based on the retry
// configuration in cfg.
func (c *Client) Save(entries []nginx.LogEntry) error {
	if len(entries) == 0 || len(c.cfg.ClickHouse.Columns) == 0 {
		return nil
	}

	maxRetries := c.cfg.Settings.Retry.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3
	}
	initial := time.Duration(c.cfg.Settings.Retry.BackoffInitialSecs) * time.Second
	if initial == 0 {
		initial = 1 * time.Second
	}
	maxDelay := time.Duration(c.cfg.Settings.Retry.BackoffMaxSecs) * time.Second
	if maxDelay == 0 {
		maxDelay = 30 * time.Second
	}

	return retry.Do(maxRetries, initial, maxDelay, func() error {
		c.mu.Lock()
		defer c.mu.Unlock()

		if err := c.connect(); err != nil {
			c.resetConn()
			return err
		}

		columns := slices.Collect(maps.Keys(c.cfg.ClickHouse.Columns))
		slices.Sort(columns)
		table := fmt.Sprintf("`%s`.`%s`", c.cfg.ClickHouse.DB, c.cfg.ClickHouse.Table)
		quoted := make([]string, len(columns))
		for i, col := range columns {
			quoted[i] = "`" + col + "`"
		}
		query := fmt.Sprintf("INSERT INTO %s (%s)", table, strings.Join(quoted, ", "))

		ctx := context.Background()
		if c.cfg.ClickHouse.UseServerSideBatching {
			// Enable ClickHouse async inserts with wait=true so the server
			// confirms the data has been flushed to disk before returning,
			// preserving at-least-once delivery guarantees.
			ctx = clickhouse.Context(ctx, clickhouse.WithSettings(clickhouse.Settings{
				"async_insert":                 1,
				"wait_for_async_insert":        1,
				"async_insert_busy_timeout_ms": 200,
			}))
		}

		batch, err := c.conn.PrepareBatch(ctx, query)
		if err != nil {
			c.resetConn()
			return fmt.Errorf("prepare batch: %w", err)
		}

		for _, entry := range entries {
			row := buildRow(columns, c.cfg.ClickHouse.Columns, entry, &c.cfg.Settings.Enrichments)
			if err := batch.Append(row...); err != nil {
				logrus.WithError(err).Error("append row")
			}
		}

		if err := batch.Send(); err != nil {
			c.resetConn()
			return fmt.Errorf("send batch: %w", err)
		}
		return nil
	})
}

// Healthy reports whether the client can reach ClickHouse. It returns false
// if no connection has been established or if a ping fails.
func (c *Client) Healthy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return false
	}
	return c.conn.Ping(context.Background()) == nil
}

// Close closes the underlying ClickHouse connection if one is open.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}

// connect opens a new ClickHouse connection if one does not already exist.
// The caller must hold c.mu.
func (c *Client) connect() error {
	if c.conn != nil {
		return nil
	}

	port, err := strconv.Atoi(c.cfg.ClickHouse.Port)
	if err != nil {
		return fmt.Errorf("invalid port %q: %w", c.cfg.ClickHouse.Port, err)
	}

	opts := &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", c.cfg.ClickHouse.Host, port)},
		Auth: clickhouse.Auth{
			Database: c.cfg.ClickHouse.DB,
			Username: c.cfg.ClickHouse.Credentials.User,
			Password: c.cfg.ClickHouse.Credentials.Password,
		},
	}

	if c.cfg.ClickHouse.TLS {
		tlsCfg := &tls.Config{
			InsecureSkipVerify: c.cfg.ClickHouse.TLSInsecureSkipVerify,
		}
		if c.cfg.ClickHouse.CACert != "" {
			caCert, err := os.ReadFile(c.cfg.ClickHouse.CACert)
			if err != nil {
				return fmt.Errorf("read CA cert %q: %w", c.cfg.ClickHouse.CACert, err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caCert) {
				return fmt.Errorf("invalid CA cert in %q", c.cfg.ClickHouse.CACert)
			}
			tlsCfg.RootCAs = pool
		}
		if c.cfg.ClickHouse.TLSCertPath != "" && c.cfg.ClickHouse.TLSKeyPath != "" {
			cert, err := tls.LoadX509KeyPair(c.cfg.ClickHouse.TLSCertPath, c.cfg.ClickHouse.TLSKeyPath)
			if err != nil {
				return fmt.Errorf("load client certificate: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
		}
		opts.TLS = tlsCfg
	}

	conn, err := clickhouse.Open(opts)
	if err != nil {
		return fmt.Errorf("open connection: %w", err)
	}

	if err := conn.Ping(context.Background()); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

	c.conn = conn
	return nil
}

// resetConn closes the current connection and sets it to nil so the next
// call to connect will establish a fresh connection. The caller must hold c.mu.
func (c *Client) resetConn() {
	if c.conn != nil {
		_ = c.conn.Close()
		c.conn = nil
	}
}

// buildRow converts a single parsed log entry into a slice of values ordered
// by the given column keys. Fields prefixed with "_" are resolved from
// enrichment configuration rather than from the log entry.
func buildRow(keys []string, columns map[string]string, entry nginx.LogEntry, enrichments *config.EnrichmentConfig) []any {
	row := make([]any, 0, len(keys))
	for _, col := range keys {
		field := columns[col]

		if strings.HasPrefix(field, "_") {
			row = append(row, resolveEnrichment(field, entry, enrichments))
			continue
		}

		value, err := entry.Field(field)
		if err != nil {
			logrus.WithField("field", field).WithError(err).Error("read field")
			row = append(row, "")
			continue
		}
		row = append(row, nginx.ParseField(field, value))
	}
	return row
}

// resolveEnrichment returns the value for a special "_" prefixed field name.
// Supported fields:
//   - _hostname: from EnrichmentConfig.Hostname
//   - _environment: from EnrichmentConfig.Environment
//   - _service: from EnrichmentConfig.Service
//   - _status_class: derived from the entry's "status" field (e.g. "200" -> "2xx")
//   - _referrer_domain: domain extracted from the entry's "http_referer" field
//   - _url_extension: file extension extracted from the request URL (e.g. "html", "js")
//   - _is_bot: "1" if the user agent looks like a bot/crawler, "0" otherwise
//   - _extra.<key>: from EnrichmentConfig.Extra map
func resolveEnrichment(field string, entry nginx.LogEntry, e *config.EnrichmentConfig) any {
	switch field {
	case "_hostname":
		return e.Hostname
	case "_environment":
		return e.Environment
	case "_service":
		return e.Service
	case "_status_class":
		status, err := entry.Field("status")
		if err != nil || len(status) == 0 {
			return ""
		}
		if status[0] < '1' || status[0] > '5' {
			return ""
		}
		return string(status[0]) + "xx"
	case "_referrer_domain":
		return extractReferrerDomain(entry)
	case "_url_extension":
		return extractURLExtension(entry)
	case "_is_bot":
		return detectBot(entry)
	default:
		if strings.HasPrefix(field, "_extra.") {
			key := strings.TrimPrefix(field, "_extra.")
			if e.Extra != nil {
				return e.Extra[key]
			}
			return ""
		}
		return ""
	}
}

// extractReferrerDomain parses the http_referer field and returns its hostname.
// Returns "" for missing, empty, or "-" referers.
func extractReferrerDomain(entry nginx.LogEntry) string {
	referer, err := entry.Field("http_referer")
	if err != nil || referer == "" || referer == "-" {
		return ""
	}
	u, err := url.Parse(referer)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// extractURLExtension extracts the file extension from the request URL.
// It expects the "request" field in the format "METHOD /path HTTP/version".
// Returns the extension without the dot (e.g. "html", "js"), or "" if none.
func extractURLExtension(entry nginx.LogEntry) string {
	request, err := entry.Field("request")
	if err != nil || request == "" {
		return ""
	}
	// Extract the path from "GET /path?query HTTP/1.1".
	parts := strings.SplitN(request, " ", 3)
	if len(parts) < 2 {
		return ""
	}
	urlPath := parts[1]
	// Strip query string and fragment.
	if i := strings.IndexAny(urlPath, "?#"); i >= 0 {
		urlPath = urlPath[:i]
	}
	ext := path.Ext(urlPath)
	if ext == "" {
		return ""
	}
	return ext[1:] // strip leading dot
}

// botPatterns contains lowercase substrings commonly found in bot/crawler
// user-agent strings.
var botPatterns = []string{
	"bot", "crawl", "spider", "slurp", "wget", "curl",
	"mediapartners", "feedfetcher", "lighthouse", "pingdom",
	"uptimerobot", "headlesschrome", "phantomjs", "httrack",
	"ahrefsbot", "semrushbot", "dotbot", "mj12bot", "baiduspider",
	"yandexbot", "duckduckbot", "facebookexternalhit", "twitterbot",
	"linkedinbot", "whatsapp", "telegrambot", "discordbot",
	"applebot", "petalbot", "bytespider", "gptbot", "claudebot",
}

// detectBot checks if the http_user_agent field matches known bot patterns.
// Returns "1" for bots, "0" for non-bots.
func detectBot(entry nginx.LogEntry) string {
	ua, err := entry.Field("http_user_agent")
	if err != nil || ua == "" || ua == "-" {
		return "0"
	}
	lower := strings.ToLower(ua)
	for _, pattern := range botPatterns {
		if strings.Contains(lower, pattern) {
			return "1"
		}
	}
	return "0"
}
