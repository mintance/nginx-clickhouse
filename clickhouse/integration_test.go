//go:build integration

package clickhouse

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func testConfig() *config.Config {
	cfg := &config.Config{}
	cfg.ClickHouse.Host = envOrDefault("CLICKHOUSE_HOST", "localhost")
	cfg.ClickHouse.Port = envOrDefault("CLICKHOUSE_PORT", "9000")
	cfg.ClickHouse.DB = "test_nginx"
	cfg.ClickHouse.Table = "access_log"
	cfg.ClickHouse.Credentials.User = envOrDefault("CLICKHOUSE_USER", "default")
	cfg.ClickHouse.Credentials.Password = envOrDefault("CLICKHOUSE_PASSWORD", "")
	cfg.ClickHouse.Columns = map[string]string{
		"RemoteAddr":    "remote_addr",
		"RemoteUser":    "remote_user",
		"TimeLocal":     "time_local",
		"Request":       "request",
		"Status":        "status",
		"BytesSent":     "bytes_sent",
		"HttpReferer":   "http_referer",
		"HttpUserAgent": "http_user_agent",
	}

	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"`

	return cfg
}

// testConnOpts returns ClickHouse connection options using the same env vars
// as testConfig, with an optional database override.
func testConnOpts(database string) *clickhouse.Options {
	return &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s",
			envOrDefault("CLICKHOUSE_HOST", "localhost"),
			envOrDefault("CLICKHOUSE_PORT", "9000"))},
		Auth: clickhouse.Auth{
			Database: database,
			Username: envOrDefault("CLICKHOUSE_USER", "default"),
			Password: envOrDefault("CLICKHOUSE_PASSWORD", ""),
		},
	}
}

func setupTestDB(t *testing.T) {
	t.Helper()

	c, err := clickhouse.Open(testConnOpts(""))
	if err != nil {
		t.Fatalf("connect to clickhouse: %v", err)
	}
	defer c.Close()

	ctx := context.Background()

	if err := c.Exec(ctx, "CREATE DATABASE IF NOT EXISTS test_nginx"); err != nil {
		t.Fatalf("create database: %v", err)
	}

	if err := c.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_nginx.access_log (
			RemoteAddr    String,
			RemoteUser    String,
			TimeLocal     DateTime,
			Request       String,
			Status        Int32,
			BytesSent     Int64,
			HttpReferer   String,
			HttpUserAgent String
		) ENGINE = MergeTree()
		ORDER BY TimeLocal
	`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	if err := c.Exec(ctx, "TRUNCATE TABLE test_nginx.access_log"); err != nil {
		t.Fatalf("truncate table: %v", err)
	}
}

func teardownTestDB(t *testing.T) {
	t.Helper()

	c, err := clickhouse.Open(testConnOpts(""))
	if err != nil {
		return
	}
	defer c.Close()

	_ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS test_nginx")
}

func TestIntegrationSave(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}

	logLines := []string{
		`192.168.1.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326 "https://example.com" "Mozilla/5.0"`,
		`10.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "POST /form HTTP/1.1" 301 512 "-" "curl/7.68.0"`,
	}

	entries := nginx.ParseLogs(parser, logLines)
	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d", len(entries))
	}

	if err := client.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, _ := clickhouse.Open(testConnOpts("test_nginx"))
	defer c.Close()

	var count uint64
	if err := c.QueryRow(context.Background(), "SELECT count() FROM test_nginx.access_log").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	var remoteAddr string
	var status int32
	if err := c.QueryRow(context.Background(),
		"SELECT RemoteAddr, Status FROM test_nginx.access_log WHERE Status = 200").Scan(&remoteAddr, &status); err != nil {
		t.Fatalf("query row: %v", err)
	}
	if remoteAddr != "192.168.1.1" {
		t.Errorf("expected RemoteAddr=192.168.1.1, got %s", remoteAddr)
	}
}

func TestIntegrationSaveEmpty(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}

	entries := nginx.ParseLogs(parser, []string{})
	if err := client.Save(entries); err != nil {
		t.Fatalf("Save with empty entries should not fail: %v", err)
	}
}

func TestIntegrationSaveMultipleBatches(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}

	entries1 := nginx.ParseLogs(parser, []string{
		`192.168.1.1 - user1 [10/Oct/2000:13:55:36 -0700] "GET /page1 HTTP/1.0" 200 1024 "-" "Mozilla/5.0"`,
	})
	if err := client.Save(entries1); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	entries2 := nginx.ParseLogs(parser, []string{
		`10.0.0.1 - user2 [10/Oct/2000:13:55:37 -0700] "GET /page2 HTTP/1.1" 404 512 "-" "curl/7.68.0"`,
		`172.16.0.1 - - [10/Oct/2000:13:55:38 -0700] "POST /api HTTP/1.1" 500 256 "-" "Python/3.9"`,
	})
	if err := client.Save(entries2); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	c, _ := clickhouse.Open(testConnOpts("test_nginx"))
	defer c.Close()

	var count uint64
	if err := c.QueryRow(context.Background(), "SELECT count() FROM test_nginx.access_log").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 total rows, got %d", count)
	}

	for _, expected := range []int32{200, 404, 500} {
		var s int32
		err := c.QueryRow(context.Background(),
			fmt.Sprintf("SELECT Status FROM test_nginx.access_log WHERE Status = %d", expected)).Scan(&s)
		if err != nil {
			t.Errorf("expected row with status %d: %v", expected, err)
		}
	}
}

func TestIntegrationConnectionReuse(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		t.Fatalf("create parser: %v", err)
	}

	entries := nginx.ParseLogs(parser, []string{
		`192.168.1.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326 "https://example.com" "Mozilla/5.0"`,
	})

	if err := client.Save(entries); err != nil {
		t.Fatalf("first Save: %v", err)
	}

	if !client.Healthy() {
		t.Error("expected Healthy to return true after successful Save")
	}

	if err := client.Save(entries); err != nil {
		t.Fatalf("second Save: %v", err)
	}

	if !client.Healthy() {
		t.Error("expected Healthy to remain true after second Save")
	}
}
