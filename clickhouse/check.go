package clickhouse

import (
	"context"
	"fmt"
	"maps"
	"slices"
)

// CheckResult holds the result of a single validation check.
type CheckResult struct {
	Name    string
	OK      bool
	Message string
}

// Check validates the ClickHouse configuration by testing connectivity,
// verifying the database and table exist, and confirming that configured
// columns match the table schema.
func (c *Client) Check() []CheckResult {
	var results []CheckResult

	// 1. Test connection
	c.mu.Lock()
	err := c.connect()
	c.mu.Unlock()

	if err != nil {
		results = append(results, CheckResult{
			Name:    "ClickHouse connection",
			OK:      false,
			Message: fmt.Sprintf("FAIL: %v", err),
		})
		return results
	}
	results = append(results, CheckResult{
		Name:    "ClickHouse connection",
		OK:      true,
		Message: fmt.Sprintf("OK (%s:%s)", c.cfg.ClickHouse.Host, c.cfg.ClickHouse.Port),
	})

	// 2. Check database exists
	ctx := context.Background()
	var dbCount uint64

	c.mu.Lock()
	err = c.conn.QueryRow(ctx,
		"SELECT count() FROM system.databases WHERE name = ?",
		c.cfg.ClickHouse.DB).Scan(&dbCount)
	c.mu.Unlock()

	if err != nil || dbCount == 0 {
		msg := fmt.Sprintf("FAIL: database %q not found", c.cfg.ClickHouse.DB)
		if err != nil {
			msg = fmt.Sprintf("FAIL: %v", err)
		}
		results = append(results, CheckResult{
			Name:    "Database",
			OK:      false,
			Message: msg,
		})
		return results
	}
	results = append(results, CheckResult{
		Name:    "Database",
		OK:      true,
		Message: fmt.Sprintf("OK (%q exists)", c.cfg.ClickHouse.DB),
	})

	// 3. Check table exists
	var tableCount uint64
	fullTable := c.cfg.ClickHouse.DB + "." + c.cfg.ClickHouse.Table

	c.mu.Lock()
	err = c.conn.QueryRow(ctx,
		"SELECT count() FROM system.tables WHERE database = ? AND name = ?",
		c.cfg.ClickHouse.DB, c.cfg.ClickHouse.Table).Scan(&tableCount)
	c.mu.Unlock()

	if err != nil || tableCount == 0 {
		msg := fmt.Sprintf("FAIL: table %q not found", fullTable)
		if err != nil {
			msg = fmt.Sprintf("FAIL: %v", err)
		}
		results = append(results, CheckResult{
			Name:    "Table",
			OK:      false,
			Message: msg,
		})
		return results
	}
	results = append(results, CheckResult{
		Name:    "Table",
		OK:      true,
		Message: fmt.Sprintf("OK (%q exists)", fullTable),
	})

	// 4. Check columns match
	tableColumns := make(map[string]bool)

	c.mu.Lock()
	rows, err := c.conn.Query(ctx,
		"SELECT name FROM system.columns WHERE database = ? AND table = ?",
		c.cfg.ClickHouse.DB, c.cfg.ClickHouse.Table)
	c.mu.Unlock()

	if err != nil {
		results = append(results, CheckResult{
			Name:    "Columns",
			OK:      false,
			Message: fmt.Sprintf("FAIL: %v", err),
		})
		return results
	}
	defer rows.Close()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		tableColumns[name] = true
	}

	configColumns := slices.Collect(maps.Keys(c.cfg.ClickHouse.Columns))
	slices.Sort(configColumns)

	var missing []string
	for _, col := range configColumns {
		if !tableColumns[col] {
			missing = append(missing, col)
		}
	}

	if len(missing) > 0 {
		results = append(results, CheckResult{
			Name:    "Columns",
			OK:      false,
			Message: fmt.Sprintf("FAIL: columns not found in table: %v", missing),
		})
	} else {
		results = append(results, CheckResult{
			Name:    "Columns",
			OK:      true,
			Message: fmt.Sprintf("OK (%d/%d columns match)", len(configColumns), len(configColumns)),
		})
	}

	return results
}
