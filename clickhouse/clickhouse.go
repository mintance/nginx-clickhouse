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

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/satyrius/gonx"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

var conn driver.Conn

// Save batch-inserts the parsed log entries into ClickHouse. It reuses an
// existing connection or establishes a new one based on cfg.
func Save(cfg *config.Config, logs []gonx.Entry) error {
	if len(logs) == 0 || len(cfg.ClickHouse.Columns) == 0 {
		return nil
	}

	c, err := openConn(cfg)
	if err != nil {
		return err
	}

	columns := slices.Collect(maps.Keys(cfg.ClickHouse.Columns))
	table := cfg.ClickHouse.DB + "." + cfg.ClickHouse.Table
	query := fmt.Sprintf("INSERT INTO %s (%s)", table, strings.Join(columns, ", "))

	batch, err := c.PrepareBatch(context.Background(), query)
	if err != nil {
		return fmt.Errorf("prepare batch: %w", err)
	}

	for _, entry := range logs {
		row := buildRow(columns, cfg.ClickHouse.Columns, entry)
		if err := batch.Append(row...); err != nil {
			logrus.Errorf("append row: %v", err)
		}
	}

	return batch.Send()
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

// openConn returns the cached ClickHouse connection or establishes a new one
// using the native TCP protocol.
func openConn(cfg *config.Config) (driver.Conn, error) {
	if conn != nil {
		return conn, nil
	}

	port, err := strconv.Atoi(cfg.ClickHouse.Port)
	if err != nil {
		return nil, fmt.Errorf("invalid port %q: %w", cfg.ClickHouse.Port, err)
	}

	c, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%d", cfg.ClickHouse.Host, port)},
		Auth: clickhouse.Auth{
			Database: cfg.ClickHouse.DB,
			Username: cfg.ClickHouse.Credentials.User,
			Password: cfg.ClickHouse.Credentials.Password,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open connection: %w", err)
	}

	if err := c.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("ping: %w", err)
	}

	conn = c
	return conn, nil
}
