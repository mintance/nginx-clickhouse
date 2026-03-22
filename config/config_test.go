package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRead(t *testing.T) {
	content := `
settings:
  interval: 10
  log_path: /var/log/nginx/access.log
  seek_from_end: true
clickhouse:
  db: metrics
  table: nginx
  host: localhost
  port: "8123"
  credentials:
    user: default
    password: secret
  columns:
    RemoteAddr: remote_addr
nginx:
  log_type: main
  log_format: "$remote_addr - $remote_user"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configPath = tmpFile

	cfg := Read()

	if cfg.Settings.Interval != 10 {
		t.Errorf("expected Interval=10, got %d", cfg.Settings.Interval)
	}
	if cfg.Settings.LogPath != "/var/log/nginx/access.log" {
		t.Errorf("expected LogPath=/var/log/nginx/access.log, got %s", cfg.Settings.LogPath)
	}
	if !cfg.Settings.SeekFromEnd {
		t.Error("expected SeekFromEnd=true")
	}
	if cfg.ClickHouse.DB != "metrics" {
		t.Errorf("expected DB=metrics, got %s", cfg.ClickHouse.DB)
	}
	if cfg.ClickHouse.Table != "nginx" {
		t.Errorf("expected Table=nginx, got %s", cfg.ClickHouse.Table)
	}
	if cfg.ClickHouse.Host != "localhost" {
		t.Errorf("expected Host=localhost, got %s", cfg.ClickHouse.Host)
	}
	if cfg.ClickHouse.Port != "8123" {
		t.Errorf("expected Port=8123, got %s", cfg.ClickHouse.Port)
	}
	if cfg.ClickHouse.Credentials.User != "default" {
		t.Errorf("expected User=default, got %s", cfg.ClickHouse.Credentials.User)
	}
	if cfg.ClickHouse.Credentials.Password != "secret" {
		t.Errorf("expected Password=secret, got %s", cfg.ClickHouse.Credentials.Password)
	}
	if cfg.Nginx.LogType != "main" {
		t.Errorf("expected LogType=main, got %s", cfg.Nginx.LogType)
	}
	if cfg.Nginx.LogFormat != "$remote_addr - $remote_user" {
		t.Errorf("expected LogFormat=$remote_addr - $remote_user, got %s", cfg.Nginx.LogFormat)
	}
}

func TestSetEnvVariables(t *testing.T) {
	cfg := &Config{}

	envVars := map[string]string{
		"LOG_PATH":            "/tmp/test.log",
		"FLUSH_INTERVAL":      "30",
		"CLICKHOUSE_HOST":     "ch-server",
		"CLICKHOUSE_PORT":     "9000",
		"CLICKHOUSE_DB":       "testdb",
		"CLICKHOUSE_TABLE":    "testtable",
		"CLICKHOUSE_USER":     "admin",
		"CLICKHOUSE_PASSWORD": "pass123",
		"NGINX_LOG_TYPE":      "combined",
		"NGINX_LOG_FORMAT":    "$remote_addr $status",
	}

	for k, v := range envVars {
		t.Setenv(k, v)
	}

	cfg.SetEnvVariables()

	if cfg.Settings.LogPath != "/tmp/test.log" {
		t.Errorf("expected LogPath=/tmp/test.log, got %s", cfg.Settings.LogPath)
	}
	if cfg.Settings.Interval != 30 {
		t.Errorf("expected Interval=30, got %d", cfg.Settings.Interval)
	}
	if cfg.ClickHouse.Host != "ch-server" {
		t.Errorf("expected Host=ch-server, got %s", cfg.ClickHouse.Host)
	}
	if cfg.ClickHouse.Port != "9000" {
		t.Errorf("expected Port=9000, got %s", cfg.ClickHouse.Port)
	}
	if cfg.ClickHouse.DB != "testdb" {
		t.Errorf("expected DB=testdb, got %s", cfg.ClickHouse.DB)
	}
	if cfg.ClickHouse.Table != "testtable" {
		t.Errorf("expected Table=testtable, got %s", cfg.ClickHouse.Table)
	}
	if cfg.ClickHouse.Credentials.User != "admin" {
		t.Errorf("expected User=admin, got %s", cfg.ClickHouse.Credentials.User)
	}
	if cfg.ClickHouse.Credentials.Password != "pass123" {
		t.Errorf("expected Password=pass123, got %s", cfg.ClickHouse.Credentials.Password)
	}
	if cfg.Nginx.LogType != "combined" {
		t.Errorf("expected LogType=combined, got %s", cfg.Nginx.LogType)
	}
	if cfg.Nginx.LogFormat != "$remote_addr $status" {
		t.Errorf("expected LogFormat=$remote_addr $status, got %s", cfg.Nginx.LogFormat)
	}
}

func TestSetEnvVariablesPartial(t *testing.T) {
	cfg := &Config{}
	cfg.Settings.LogPath = "/original/path.log"
	cfg.Settings.Interval = 5
	cfg.ClickHouse.Host = "original-host"

	t.Setenv("LOG_PATH", "/new/path.log")

	cfg.SetEnvVariables()

	if cfg.Settings.LogPath != "/new/path.log" {
		t.Errorf("expected LogPath=/new/path.log, got %s", cfg.Settings.LogPath)
	}
	if cfg.Settings.Interval != 5 {
		t.Errorf("expected Interval=5 (unchanged), got %d", cfg.Settings.Interval)
	}
	if cfg.ClickHouse.Host != "original-host" {
		t.Errorf("expected Host=original-host (unchanged), got %s", cfg.ClickHouse.Host)
	}
}

func TestSetEnvVariablesMaxBufferSize(t *testing.T) {
	cfg := &Config{}
	t.Setenv("MAX_BUFFER_SIZE", "5000")

	cfg.SetEnvVariables()

	if cfg.Settings.MaxBufferSize != 5000 {
		t.Errorf("expected MaxBufferSize=5000, got %d", cfg.Settings.MaxBufferSize)
	}
}

func TestReadMaxBufferSize(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
  max_buffer_size: 20000
clickhouse:
  db: test
  table: t
  host: localhost
  port: "9000"
nginx:
  log_type: main
  log_format: "$remote_addr"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configPath = tmpFile
	cfg := Read()

	if cfg.Settings.MaxBufferSize != 20000 {
		t.Errorf("expected MaxBufferSize=20000, got %d", cfg.Settings.MaxBufferSize)
	}
}

func TestReadRetryConfig(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
  retry:
    max_retries: 3
    backoff_initial_secs: 1
    backoff_max_secs: 30
clickhouse:
  db: test
  table: t
  host: localhost
  port: "9000"
nginx:
  log_type: main
  log_format: "$remote_addr"
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configPath = tmpFile
	cfg := Read()

	if cfg.Settings.Retry.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.Settings.Retry.MaxRetries)
	}
	if cfg.Settings.Retry.BackoffInitialSecs != 1 {
		t.Errorf("expected BackoffInitialSecs=1, got %d", cfg.Settings.Retry.BackoffInitialSecs)
	}
	if cfg.Settings.Retry.BackoffMaxSecs != 30 {
		t.Errorf("expected BackoffMaxSecs=30, got %d", cfg.Settings.Retry.BackoffMaxSecs)
	}
}

func TestSetEnvVariablesRetry(t *testing.T) {
	cfg := &Config{}

	t.Setenv("RETRY_MAX", "5")
	t.Setenv("RETRY_BACKOFF_INITIAL", "2")
	t.Setenv("RETRY_BACKOFF_MAX", "60")

	cfg.SetEnvVariables()

	if cfg.Settings.Retry.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", cfg.Settings.Retry.MaxRetries)
	}
	if cfg.Settings.Retry.BackoffInitialSecs != 2 {
		t.Errorf("expected BackoffInitialSecs=2, got %d", cfg.Settings.Retry.BackoffInitialSecs)
	}
	if cfg.Settings.Retry.BackoffMaxSecs != 60 {
		t.Errorf("expected BackoffMaxSecs=60, got %d", cfg.Settings.Retry.BackoffMaxSecs)
	}
}

func TestSetEnvVariablesInvalidInterval(t *testing.T) {
	cfg := &Config{}
	cfg.Settings.Interval = 5

	t.Setenv("FLUSH_INTERVAL", "not-a-number")

	cfg.SetEnvVariables()

	if cfg.Settings.Interval != 0 {
		t.Errorf("expected Interval=0 after invalid conversion, got %d", cfg.Settings.Interval)
	}
}
