//go:build e2e

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClickHouse/clickhouse-go/v2"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func chHost() string { return envOrDefault("CLICKHOUSE_HOST", "localhost") }
func chPort() string { return envOrDefault("CLICKHOUSE_PORT", "9000") }

func testConnOpts(database string) *clickhouse.Options {
	return &clickhouse.Options{
		Addr: []string{fmt.Sprintf("%s:%s", chHost(), chPort())},
		Auth: clickhouse.Auth{
			Database: database,
			Username: envOrDefault("CLICKHOUSE_USER", "default"),
			Password: envOrDefault("CLICKHOUSE_PASSWORD", ""),
		},
	}
}

// buildBinary compiles the binary and returns its path.
func buildBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "nginx-clickhouse")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build binary: %v", err)
	}
	return bin
}

// writeTestConfig writes a config YAML pointing at the test ClickHouse and returns the path.
func writeTestConfig(t *testing.T, logPath string) string {
	t.Helper()
	cfg := fmt.Sprintf(`settings:
  interval: 1
  log_path: %s
  max_buffer_size: 100
clickhouse:
  db: test_e2e
  table: access_log
  host: %s
  port: "%s"
  credentials:
    user: %s
    password: "%s"
  columns:
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
  log_format: '$remote_addr - $remote_user [$time_local] "$request" $status $bytes_sent "$http_referer" "$http_user_agent"'
`,
		logPath,
		chHost(),
		chPort(),
		envOrDefault("CLICKHOUSE_USER", "default"),
		envOrDefault("CLICKHOUSE_PASSWORD", ""),
	)

	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}
	return cfgPath
}

func setupTestDB(t *testing.T) {
	t.Helper()
	c, err := clickhouse.Open(testConnOpts(""))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	ctx := context.Background()
	if err := c.Exec(ctx, "CREATE DATABASE IF NOT EXISTS test_e2e"); err != nil {
		t.Fatalf("create database: %v", err)
	}
	if err := c.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS test_e2e.access_log (
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
	if err := c.Exec(ctx, "TRUNCATE TABLE test_e2e.access_log"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

func teardownTestDB(t *testing.T) {
	t.Helper()
	c, err := clickhouse.Open(testConnOpts(""))
	if err != nil {
		return
	}
	defer c.Close()
	_ = c.Exec(context.Background(), "DROP DATABASE IF EXISTS test_e2e")
}

func queryCount(t *testing.T) uint64 {
	t.Helper()
	c, err := clickhouse.Open(testConnOpts("test_e2e"))
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	var count uint64
	if err := c.QueryRow(context.Background(), "SELECT count() FROM test_e2e.access_log").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	return count
}

var testLogLines = `192.168.1.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326 "https://example.com" "Mozilla/5.0"
10.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "POST /form HTTP/1.1" 301 512 "-" "curl/7.68.0"
172.16.0.1 - admin [10/Oct/2000:13:55:38 -0700] "DELETE /api/v1/users HTTP/1.1" 204 0 "-" "Python/3.11"
`

func TestIntegrationOnce(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	bin := buildBinary(t)

	// Write test log file.
	logFile := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(logFile, []byte(testLogLines), 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := writeTestConfig(t, logFile)

	cmd := exec.Command(bin, "-once", "-config_path", cfgPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("binary -once exited with error: %v", err)
	}

	count := queryCount(t)
	if count != 3 {
		t.Errorf("expected 3 rows after -once, got %d", count)
	}
}

func TestIntegrationStdin(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	bin := buildBinary(t)
	cfgPath := writeTestConfig(t, "/dev/null")

	cmd := exec.Command(bin, "-stdin", "-config_path", cfgPath)
	cmd.Stdin = bytes.NewBufferString(testLogLines)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("binary -stdin exited with error: %v", err)
	}

	count := queryCount(t)
	if count != 3 {
		t.Errorf("expected 3 rows after -stdin, got %d", count)
	}
}

func TestIntegrationCheck(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	bin := buildBinary(t)

	logFile := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(logFile, []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	cfgPath := writeTestConfig(t, logFile)

	var stdout bytes.Buffer
	cmd := exec.Command(bin, "-check", "-config_path", cfgPath)
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("binary -check exited with error: %v\noutput: %s", err, stdout.String())
	}

	output := stdout.String()
	if !strings.Contains(output, "All checks passed") {
		t.Errorf("expected 'All checks passed' in output, got:\n%s", output)
	}
}

func TestIntegrationCheckBadConfig(t *testing.T) {
	bin := buildBinary(t)

	// Config pointing to a non-existent ClickHouse.
	cfg := `settings:
  interval: 1
  log_path: /dev/null
clickhouse:
  db: nonexistent
  table: nonexistent
  host: 127.0.0.1
  port: "19999"
  credentials:
    user: default
    password: ""
  columns:
    RemoteAddr: remote_addr
nginx:
  log_type: main
  log_format: '$remote_addr'
`
	cfgPath := filepath.Join(t.TempDir(), "bad-config.yml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "-check", "-config_path", cfgPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected -check to fail with bad config, but it succeeded")
	}
}
