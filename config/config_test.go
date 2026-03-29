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
    max_retries: 5
    backoff_initial_secs: 2
    backoff_max_secs: 60
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

func TestReadBufferConfig(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
  buffer:
    type: disk
    disk_path: /var/lib/buf
    max_disk_bytes: 2147483648
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

	if cfg.Settings.Buffer.Type != "disk" {
		t.Errorf("expected Buffer.Type=disk, got %s", cfg.Settings.Buffer.Type)
	}
	if cfg.Settings.Buffer.DiskPath != "/var/lib/buf" {
		t.Errorf("expected Buffer.DiskPath=/var/lib/buf, got %s", cfg.Settings.Buffer.DiskPath)
	}
	if cfg.Settings.Buffer.MaxDiskBytes != 2147483648 {
		t.Errorf("expected Buffer.MaxDiskBytes=2147483648, got %d", cfg.Settings.Buffer.MaxDiskBytes)
	}
}

func TestSetEnvVariablesBuffer(t *testing.T) {
	cfg := &Config{}

	t.Setenv("BUFFER_TYPE", "disk")
	t.Setenv("BUFFER_DISK_PATH", "/tmp/buf")
	t.Setenv("BUFFER_MAX_DISK_BYTES", "1073741824")

	cfg.SetEnvVariables()

	if cfg.Settings.Buffer.Type != "disk" {
		t.Errorf("expected Buffer.Type=disk, got %s", cfg.Settings.Buffer.Type)
	}
	if cfg.Settings.Buffer.DiskPath != "/tmp/buf" {
		t.Errorf("expected Buffer.DiskPath=/tmp/buf, got %s", cfg.Settings.Buffer.DiskPath)
	}
	if cfg.Settings.Buffer.MaxDiskBytes != 1073741824 {
		t.Errorf("expected Buffer.MaxDiskBytes=1073741824, got %d", cfg.Settings.Buffer.MaxDiskBytes)
	}
}

func TestReadCircuitBreakerConfig(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
  circuit_breaker:
    enabled: true
    threshold: 10
    cooldown_secs: 30
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

	if !cfg.Settings.CircuitBreaker.Enabled {
		t.Error("expected CircuitBreaker.Enabled=true")
	}
	if cfg.Settings.CircuitBreaker.Threshold != 10 {
		t.Errorf("expected CircuitBreaker.Threshold=10, got %d", cfg.Settings.CircuitBreaker.Threshold)
	}
	if cfg.Settings.CircuitBreaker.CooldownSecs != 30 {
		t.Errorf("expected CircuitBreaker.CooldownSecs=30, got %d", cfg.Settings.CircuitBreaker.CooldownSecs)
	}
}

func TestSetEnvVariablesCircuitBreaker(t *testing.T) {
	cfg := &Config{}

	t.Setenv("CIRCUIT_BREAKER_ENABLED", "true")
	t.Setenv("CIRCUIT_BREAKER_THRESHOLD", "5")
	t.Setenv("CIRCUIT_BREAKER_COOLDOWN", "60")

	cfg.SetEnvVariables()

	if !cfg.Settings.CircuitBreaker.Enabled {
		t.Error("expected CircuitBreaker.Enabled=true")
	}
	if cfg.Settings.CircuitBreaker.Threshold != 5 {
		t.Errorf("expected CircuitBreaker.Threshold=5, got %d", cfg.Settings.CircuitBreaker.Threshold)
	}
	if cfg.Settings.CircuitBreaker.CooldownSecs != 60 {
		t.Errorf("expected CircuitBreaker.CooldownSecs=60, got %d", cfg.Settings.CircuitBreaker.CooldownSecs)
	}
}

func TestReadTLSConfig(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
clickhouse:
  db: test
  table: t
  host: localhost
  port: "9440"
  tls: true
  tls_insecure_skip_verify: false
  ca_cert: /etc/ssl/ca.pem
  tls_cert_path: /etc/ssl/client.crt
  tls_key_path: /etc/ssl/client.key
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

	if !cfg.ClickHouse.TLS {
		t.Error("expected TLS=true")
	}
	if cfg.ClickHouse.TLSInsecureSkipVerify {
		t.Error("expected TLSInsecureSkipVerify=false")
	}
	if cfg.ClickHouse.CACert != "/etc/ssl/ca.pem" {
		t.Errorf("expected CACert=/etc/ssl/ca.pem, got %s", cfg.ClickHouse.CACert)
	}
	if cfg.ClickHouse.Port != "9440" {
		t.Errorf("expected Port=9440, got %s", cfg.ClickHouse.Port)
	}
	if cfg.ClickHouse.TLSCertPath != "/etc/ssl/client.crt" {
		t.Errorf("expected TLSCertPath=/etc/ssl/client.crt, got %s", cfg.ClickHouse.TLSCertPath)
	}
	if cfg.ClickHouse.TLSKeyPath != "/etc/ssl/client.key" {
		t.Errorf("expected TLSKeyPath=/etc/ssl/client.key, got %s", cfg.ClickHouse.TLSKeyPath)
	}
}

func TestSetEnvVariablesTLS(t *testing.T) {
	cfg := &Config{}

	t.Setenv("CLICKHOUSE_TLS", "true")
	t.Setenv("CLICKHOUSE_TLS_SKIP_VERIFY", "true")
	t.Setenv("CLICKHOUSE_CA_CERT", "/custom/ca.pem")
	t.Setenv("CLICKHOUSE_TLS_CERT_PATH", "/custom/client.crt")
	t.Setenv("CLICKHOUSE_TLS_KEY_PATH", "/custom/client.key")

	cfg.SetEnvVariables()

	if !cfg.ClickHouse.TLS {
		t.Error("expected TLS=true")
	}
	if !cfg.ClickHouse.TLSInsecureSkipVerify {
		t.Error("expected TLSInsecureSkipVerify=true")
	}
	if cfg.ClickHouse.CACert != "/custom/ca.pem" {
		t.Errorf("expected CACert=/custom/ca.pem, got %s", cfg.ClickHouse.CACert)
	}
	if cfg.ClickHouse.TLSCertPath != "/custom/client.crt" {
		t.Errorf("expected TLSCertPath=/custom/client.crt, got %s", cfg.ClickHouse.TLSCertPath)
	}
	if cfg.ClickHouse.TLSKeyPath != "/custom/client.key" {
		t.Errorf("expected TLSKeyPath=/custom/client.key, got %s", cfg.ClickHouse.TLSKeyPath)
	}
}

func TestReadLogFormatType(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
clickhouse:
  db: test
  table: t
  host: localhost
  port: "9000"
nginx:
  log_type: main
  log_format: ""
  log_format_type: json
`
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "config.yml")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	configPath = tmpFile
	cfg := Read()

	if cfg.Nginx.LogFormatType != "json" {
		t.Errorf("expected LogFormatType=json, got %s", cfg.Nginx.LogFormatType)
	}
}

func TestSetEnvVariablesLogFormatType(t *testing.T) {
	cfg := &Config{}

	t.Setenv("NGINX_LOG_FORMAT_TYPE", "json")

	cfg.SetEnvVariables()

	if cfg.Nginx.LogFormatType != "json" {
		t.Errorf("expected LogFormatType=json, got %s", cfg.Nginx.LogFormatType)
	}
}

func TestReadEnrichmentConfig(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
  enrichments:
    hostname: auto
    environment: production
    service: my-api
    extra:
      datacenter: us-east-1
      cluster: web-prod
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

	if cfg.Settings.Enrichments.Hostname != "auto" {
		t.Errorf("expected Hostname=auto, got %s", cfg.Settings.Enrichments.Hostname)
	}
	if cfg.Settings.Enrichments.Environment != "production" {
		t.Errorf("expected Environment=production, got %s", cfg.Settings.Enrichments.Environment)
	}
	if cfg.Settings.Enrichments.Service != "my-api" {
		t.Errorf("expected Service=my-api, got %s", cfg.Settings.Enrichments.Service)
	}
	if cfg.Settings.Enrichments.Extra["datacenter"] != "us-east-1" {
		t.Errorf("expected Extra[datacenter]=us-east-1, got %s", cfg.Settings.Enrichments.Extra["datacenter"])
	}
	if cfg.Settings.Enrichments.Extra["cluster"] != "web-prod" {
		t.Errorf("expected Extra[cluster]=web-prod, got %s", cfg.Settings.Enrichments.Extra["cluster"])
	}
}

func TestSetEnvVariablesEnrichments(t *testing.T) {
	cfg := &Config{}

	t.Setenv("ENRICHMENT_HOSTNAME", "override-host")
	t.Setenv("ENRICHMENT_ENVIRONMENT", "staging")
	t.Setenv("ENRICHMENT_SERVICE", "override-svc")

	cfg.SetEnvVariables()

	if cfg.Settings.Enrichments.Hostname != "override-host" {
		t.Errorf("expected Hostname=override-host, got %s", cfg.Settings.Enrichments.Hostname)
	}
	if cfg.Settings.Enrichments.Environment != "staging" {
		t.Errorf("expected Environment=staging, got %s", cfg.Settings.Enrichments.Environment)
	}
	if cfg.Settings.Enrichments.Service != "override-svc" {
		t.Errorf("expected Service=override-svc, got %s", cfg.Settings.Enrichments.Service)
	}
}

func TestReadServerSideBatching(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
clickhouse:
  db: test
  table: t
  host: localhost
  port: "9000"
  use_server_side_batching: true
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

	if !cfg.ClickHouse.UseServerSideBatching {
		t.Error("expected UseServerSideBatching=true")
	}
}

func TestSetEnvVariablesServerSideBatching(t *testing.T) {
	cfg := &Config{}

	t.Setenv("CLICKHOUSE_USE_SERVER_SIDE_BATCHING", "true")

	cfg.SetEnvVariables()

	if !cfg.ClickHouse.UseServerSideBatching {
		t.Error("expected UseServerSideBatching=true")
	}
}

func TestServerSideBatchingDefaultFalse(t *testing.T) {
	content := `
settings:
  interval: 5
  log_path: /tmp/test.log
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

	if cfg.ClickHouse.UseServerSideBatching {
		t.Error("expected UseServerSideBatching=false by default")
	}
}

func TestSetEnvVariablesInvalidInterval(t *testing.T) {
	cfg := &Config{}
	cfg.Settings.Interval = 5

	t.Setenv("FLUSH_INTERVAL", "not-a-number")

	cfg.SetEnvVariables()

	if cfg.Settings.Interval != 5 {
		t.Errorf("expected Interval=5 (unchanged) after invalid conversion, got %d", cfg.Settings.Interval)
	}
}

func TestSetEnvVariablesEnrichmentExtra(t *testing.T) {
	cfg := &Config{}

	t.Setenv("ENRICHMENT_POD_NAMESPACE", "default")

	cfg.SetEnvVariables()

	if cfg.Settings.Enrichments.Extra["pod_namespace"] != "default" {
		t.Errorf("expected Extra[pod_namespace]=default, got %s", cfg.Settings.Enrichments.Extra["pod_namespace"])
	}
}

func TestSetEnvVariablesEnrichmentExtraMultiple(t *testing.T) {
	cfg := &Config{}

	t.Setenv("ENRICHMENT_POD_NAMESPACE", "kube-system")
	t.Setenv("ENRICHMENT_NODE_NAME", "node-1")
	t.Setenv("ENRICHMENT_POD_IP", "10.0.0.5")

	cfg.SetEnvVariables()

	if cfg.Settings.Enrichments.Extra["pod_namespace"] != "kube-system" {
		t.Errorf("expected pod_namespace=kube-system, got %s", cfg.Settings.Enrichments.Extra["pod_namespace"])
	}
	if cfg.Settings.Enrichments.Extra["node_name"] != "node-1" {
		t.Errorf("expected node_name=node-1, got %s", cfg.Settings.Enrichments.Extra["node_name"])
	}
	if cfg.Settings.Enrichments.Extra["pod_ip"] != "10.0.0.5" {
		t.Errorf("expected pod_ip=10.0.0.5, got %s", cfg.Settings.Enrichments.Extra["pod_ip"])
	}
}

func TestSetEnvVariablesEnrichmentExtraOverridesYAML(t *testing.T) {
	cfg := &Config{}
	cfg.Settings.Enrichments.Extra = map[string]string{
		"datacenter": "us-east-1",
	}

	t.Setenv("ENRICHMENT_DATACENTER", "eu-west-1")

	cfg.SetEnvVariables()

	if cfg.Settings.Enrichments.Extra["datacenter"] != "eu-west-1" {
		t.Errorf("expected datacenter=eu-west-1, got %s", cfg.Settings.Enrichments.Extra["datacenter"])
	}
}

func TestSetEnvVariablesEnrichmentExtraEmptyValue(t *testing.T) {
	cfg := &Config{}

	t.Setenv("ENRICHMENT_EMPTY_KEY", "")

	cfg.SetEnvVariables()

	val, ok := cfg.Settings.Enrichments.Extra["empty_key"]
	if !ok {
		t.Error("expected empty_key to exist in Extra map")
	}
	if val != "" {
		t.Errorf("expected empty string, got %s", val)
	}
}

func TestSetEnvVariablesEnrichmentExtraSkipsKnownFields(t *testing.T) {
	cfg := &Config{}

	t.Setenv("ENRICHMENT_HOSTNAME", "my-host")
	t.Setenv("ENRICHMENT_ENVIRONMENT", "staging")
	t.Setenv("ENRICHMENT_SERVICE", "my-svc")
	t.Setenv("ENRICHMENT_POD_NAMESPACE", "default")

	cfg.SetEnvVariables()

	// Known fields go to their struct fields, not extra
	if cfg.Settings.Enrichments.Hostname != "my-host" {
		t.Errorf("expected Hostname=my-host, got %s", cfg.Settings.Enrichments.Hostname)
	}
	if cfg.Settings.Enrichments.Environment != "staging" {
		t.Errorf("expected Environment=staging, got %s", cfg.Settings.Enrichments.Environment)
	}
	if cfg.Settings.Enrichments.Service != "my-svc" {
		t.Errorf("expected Service=my-svc, got %s", cfg.Settings.Enrichments.Service)
	}
	// Unknown field goes to extra
	if cfg.Settings.Enrichments.Extra["pod_namespace"] != "default" {
		t.Errorf("expected Extra[pod_namespace]=default, got %s", cfg.Settings.Enrichments.Extra["pod_namespace"])
	}
	// Known fields should NOT be in extra
	if _, ok := cfg.Settings.Enrichments.Extra["hostname"]; ok {
		t.Error("expected hostname to NOT be in Extra map")
	}
}
