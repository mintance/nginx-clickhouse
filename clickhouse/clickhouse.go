// Package clickhouse handles batch-inserting parsed NGINX log entries into
// ClickHouse using the native TCP protocol.
package clickhouse

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/satyrius/gonx"
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
func (c *Client) Save(entries []gonx.Entry) error {
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
		table := c.cfg.ClickHouse.DB + "." + c.cfg.ClickHouse.Table
		query := fmt.Sprintf("INSERT INTO %s (%s)", table, strings.Join(columns, ", "))

		batch, err := c.conn.PrepareBatch(context.Background(), query)
		if err != nil {
			c.resetConn()
			return fmt.Errorf("prepare batch: %w", err)
		}

		for _, entry := range entries {
			row := buildRow(columns, c.cfg.ClickHouse.Columns, entry)
			if err := batch.Append(row...); err != nil {
				logrus.Errorf("append row: %v", err)
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

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", c.cfg.ClickHouse.Host, port)},
		Auth: clickhouse.Auth{
			Database: c.cfg.ClickHouse.DB,
			Username: c.cfg.ClickHouse.Credentials.User,
			Password: c.cfg.ClickHouse.Credentials.Password,
		},
	})
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
// by the given column keys.
func buildRow(keys []string, columns map[string]string, entry gonx.Entry) []any {
	row := make([]any, 0, len(keys))
	for _, col := range keys {
		field := columns[col]
		value, err := entry.Field(field)
		if err != nil {
			logrus.Errorf("read field %s: %v", field, err)
			row = append(row, "")
			continue
		}
		row = append(row, nginx.ParseField(field, value))
	}
	return row
}
