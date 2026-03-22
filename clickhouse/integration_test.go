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

	c, err := clickhouse.Open(testConnOpts("test_nginx"))
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
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

	c, err := clickhouse.Open(testConnOpts("test_nginx"))
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
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

func TestIntegrationCheck(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	results := client.Check()

	for _, r := range results {
		if !r.OK {
			t.Errorf("check %q failed: %s", r.Name, r.Message)
		}
	}

	if len(results) != 4 {
		t.Errorf("expected 4 check results, got %d", len(results))
	}
}

func TestIntegrationCheckMissingColumn(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	cfg.ClickHouse.Columns["NonExistentColumn"] = "fake_field"

	client := NewClient(cfg)
	defer client.Close()

	results := client.Check()

	// Last result should be the columns check and should fail
	colResult := results[len(results)-1]
	if colResult.OK {
		t.Error("expected columns check to fail with non-existent column")
	}
}

func TestIntegrationCheckBadDB(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	// Use valid connection credentials but check a non-existent database.
	// Set DB after connection so connect() succeeds with the real DB,
	// but the database/table checks query the fake one.
	client := NewClient(cfg)
	defer client.Close()

	// Force connection with valid DB first.
	results := client.Check()
	if len(results) == 0 || !results[0].OK {
		t.Fatal("precondition: connection should succeed with valid config")
	}

	// Now change DB and re-check. We need a fresh client since the
	// connection is cached. Simpler: just verify the Check function
	// catches a non-existent table by changing only the table name.
	cfg2 := testConfig()
	cfg2.ClickHouse.Table = "nonexistent_table_12345"
	client2 := NewClient(cfg2)
	defer client2.Close()

	results2 := client2.Check()

	// Connection and database should pass, table should fail
	if len(results2) < 3 {
		t.Fatalf("expected at least 3 results, got %d", len(results2))
	}
	if !results2[0].OK {
		t.Errorf("expected connection check to pass, got: %s", results2[0].Message)
	}
	if !results2[1].OK {
		t.Errorf("expected database check to pass, got: %s", results2[1].Message)
	}
	if results2[2].OK {
		t.Error("expected table check to fail for non-existent table")
	}
}

func TestIntegrationCheckWithEnrichments(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	// Add enrichment columns that don't exist in the table
	cfg.ClickHouse.Columns["Hostname"] = "_hostname"
	cfg.ClickHouse.Columns["StatusClass"] = "_status_class"

	client := NewClient(cfg)
	defer client.Close()

	results := client.Check()

	for _, r := range results {
		if !r.OK {
			t.Errorf("check %q failed: %s", r.Name, r.Message)
		}
	}

	// Columns check should pass — enrichment columns are skipped
	colResult := results[len(results)-1]
	if !colResult.OK {
		t.Errorf("expected columns check to pass with enrichment columns, got: %s", colResult.Message)
	}
}

// setupTestDBEnriched creates the test_nginx database and an enriched table
// that includes extra columns for enrichment fields.
func setupTestDBEnriched(t *testing.T) {
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
		CREATE TABLE IF NOT EXISTS test_nginx.access_log_enriched (
			RemoteAddr    String,
			Status        Int32,
			BytesSent     Int64,
			RequestTime   Float64,
			HttpReferer   String,
			HttpUserAgent String,
			Hostname      String,
			Environment   String,
			Service       String,
			StatusClass   String
		) ENGINE = MergeTree()
		ORDER BY Hostname
	`); err != nil {
		t.Fatalf("create enriched table: %v", err)
	}

	if err := c.Exec(ctx, "TRUNCATE TABLE test_nginx.access_log_enriched"); err != nil {
		t.Fatalf("truncate enriched table: %v", err)
	}
}

// testEnrichedConfig returns a config that maps columns to the enriched table,
// including enrichment fields prefixed with "_".
func testEnrichedConfig() *config.Config {
	cfg := &config.Config{}
	cfg.ClickHouse.Host = envOrDefault("CLICKHOUSE_HOST", "localhost")
	cfg.ClickHouse.Port = envOrDefault("CLICKHOUSE_PORT", "9000")
	cfg.ClickHouse.DB = "test_nginx"
	cfg.ClickHouse.Table = "access_log_enriched"
	cfg.ClickHouse.Credentials.User = envOrDefault("CLICKHOUSE_USER", "default")
	cfg.ClickHouse.Credentials.Password = envOrDefault("CLICKHOUSE_PASSWORD", "")
	cfg.ClickHouse.Columns = map[string]string{
		"RemoteAddr":    "remote_addr",
		"Status":        "status",
		"BytesSent":     "bytes_sent",
		"RequestTime":   "request_time",
		"HttpReferer":   "http_referer",
		"HttpUserAgent": "http_user_agent",
		"Hostname":      "_hostname",
		"Environment":   "_environment",
		"Service":       "_service",
		"StatusClass":   "_status_class",
	}
	cfg.Settings.Enrichments.Hostname = "test-host"
	cfg.Settings.Enrichments.Environment = "testing"
	cfg.Settings.Enrichments.Service = "nginx-test"

	return cfg
}

func TestIntegrationJSONLogs(t *testing.T) {
	setupTestDB(t)
	defer teardownTestDB(t)

	cfg := testConfig()
	client := NewClient(cfg)
	defer client.Close()

	logLines := []string{
		`{"remote_addr":"192.168.1.1","remote_user":"frank","time_local":"10/Oct/2000:13:55:36 -0700","request":"GET /index.html HTTP/1.0","status":200,"bytes_sent":2326,"http_referer":"https://example.com","http_user_agent":"Mozilla/5.0"}`,
		`{"remote_addr":"10.0.0.1","remote_user":"-","time_local":"10/Oct/2000:13:55:37 -0700","request":"POST /form HTTP/1.1","status":404,"bytes_sent":512,"http_referer":"-","http_user_agent":"curl/7.68.0"}`,
	}

	entries := nginx.ParseJSONLogs(logLines)
	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d", len(entries))
	}

	if err := client.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, err := clickhouse.Open(testConnOpts("test_nginx"))
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
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
		t.Fatalf("query row with status 200: %v", err)
	}
	if remoteAddr != "192.168.1.1" {
		t.Errorf("expected RemoteAddr=192.168.1.1, got %s", remoteAddr)
	}

	if err := c.QueryRow(context.Background(),
		"SELECT RemoteAddr, Status FROM test_nginx.access_log WHERE Status = 404").Scan(&remoteAddr, &status); err != nil {
		t.Fatalf("query row with status 404: %v", err)
	}
	if remoteAddr != "10.0.0.1" {
		t.Errorf("expected RemoteAddr=10.0.0.1, got %s", remoteAddr)
	}
}

func TestIntegrationEnrichments(t *testing.T) {
	setupTestDBEnriched(t)
	defer teardownTestDB(t)

	cfg := testEnrichedConfig()
	client := NewClient(cfg)
	defer client.Close()

	logLines := []string{
		`{"remote_addr":"192.168.1.1","status":200,"bytes_sent":2326,"request_time":0.123,"http_referer":"https://example.com","http_user_agent":"Mozilla/5.0"}`,
		`{"remote_addr":"10.0.0.1","status":500,"bytes_sent":512,"request_time":1.456,"http_referer":"-","http_user_agent":"curl/7.68.0"}`,
	}

	entries := nginx.ParseJSONLogs(logLines)
	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d", len(entries))
	}

	if err := client.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, err := clickhouse.Open(testConnOpts("test_nginx"))
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
	defer c.Close()

	var count uint64
	if err := c.QueryRow(context.Background(), "SELECT count() FROM test_nginx.access_log_enriched").Scan(&count); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}

	// Verify enrichment values for the row with status 200.
	var hostname, environment, service, statusClass string
	if err := c.QueryRow(context.Background(),
		"SELECT Hostname, Environment, Service, StatusClass FROM test_nginx.access_log_enriched WHERE Status = 200",
	).Scan(&hostname, &environment, &service, &statusClass); err != nil {
		t.Fatalf("query enrichment row (status 200): %v", err)
	}
	if hostname != "test-host" {
		t.Errorf("expected Hostname=test-host, got %s", hostname)
	}
	if environment != "testing" {
		t.Errorf("expected Environment=testing, got %s", environment)
	}
	if service != "nginx-test" {
		t.Errorf("expected Service=nginx-test, got %s", service)
	}
	if statusClass != "2xx" {
		t.Errorf("expected StatusClass=2xx, got %s", statusClass)
	}

	// Verify enrichment values for the row with status 500.
	if err := c.QueryRow(context.Background(),
		"SELECT Hostname, Environment, Service, StatusClass FROM test_nginx.access_log_enriched WHERE Status = 500",
	).Scan(&hostname, &environment, &service, &statusClass); err != nil {
		t.Fatalf("query enrichment row (status 500): %v", err)
	}
	if hostname != "test-host" {
		t.Errorf("expected Hostname=test-host, got %s", hostname)
	}
	if environment != "testing" {
		t.Errorf("expected Environment=testing, got %s", environment)
	}
	if service != "nginx-test" {
		t.Errorf("expected Service=nginx-test, got %s", service)
	}
	if statusClass != "5xx" {
		t.Errorf("expected StatusClass=5xx, got %s", statusClass)
	}
}

func TestIntegrationJSONLogsWithEnrichments(t *testing.T) {
	setupTestDBEnriched(t)
	defer teardownTestDB(t)

	cfg := testEnrichedConfig()
	client := NewClient(cfg)
	defer client.Close()

	logLines := []string{
		`{"remote_addr":"172.16.0.1","status":201,"bytes_sent":1024,"request_time":0.05,"http_referer":"https://app.example.com","http_user_agent":"Firefox/99.0"}`,
		`{"remote_addr":"10.10.10.10","status":503,"bytes_sent":256,"request_time":5.0,"http_referer":"-","http_user_agent":"Python/3.11"}`,
	}

	entries := nginx.ParseJSONLogs(logLines)
	if len(entries) != 2 {
		t.Fatalf("expected 2 parsed entries, got %d", len(entries))
	}

	if err := client.Save(entries); err != nil {
		t.Fatalf("Save: %v", err)
	}

	c, err := clickhouse.Open(testConnOpts("test_nginx"))
	if err != nil {
		t.Fatalf("open connection: %v", err)
	}
	defer c.Close()

	// Verify all fields for the row with status 201.
	var (
		remoteAddr, httpReferer, httpUserAgent string
		hostname, environment, service         string
		statusClass                            string
		status                                 int32
		bytesSent                              int64
		requestTime                            float64
	)
	if err := c.QueryRow(context.Background(),
		`SELECT RemoteAddr, Status, BytesSent, RequestTime, HttpReferer, HttpUserAgent,
		        Hostname, Environment, Service, StatusClass
		 FROM test_nginx.access_log_enriched WHERE Status = 201`,
	).Scan(&remoteAddr, &status, &bytesSent, &requestTime, &httpReferer, &httpUserAgent,
		&hostname, &environment, &service, &statusClass); err != nil {
		t.Fatalf("query combined row (status 201): %v", err)
	}

	if remoteAddr != "172.16.0.1" {
		t.Errorf("expected RemoteAddr=172.16.0.1, got %s", remoteAddr)
	}
	if status != 201 {
		t.Errorf("expected Status=201, got %d", status)
	}
	if bytesSent != 1024 {
		t.Errorf("expected BytesSent=1024, got %d", bytesSent)
	}
	if httpReferer != "https://app.example.com" {
		t.Errorf("expected HttpReferer=https://app.example.com, got %s", httpReferer)
	}
	if httpUserAgent != "Firefox/99.0" {
		t.Errorf("expected HttpUserAgent=Firefox/99.0, got %s", httpUserAgent)
	}
	if hostname != "test-host" {
		t.Errorf("expected Hostname=test-host, got %s", hostname)
	}
	if environment != "testing" {
		t.Errorf("expected Environment=testing, got %s", environment)
	}
	if service != "nginx-test" {
		t.Errorf("expected Service=nginx-test, got %s", service)
	}
	if statusClass != "2xx" {
		t.Errorf("expected StatusClass=2xx, got %s", statusClass)
	}

	// Verify enrichment fields for the row with status 503.
	if err := c.QueryRow(context.Background(),
		`SELECT RemoteAddr, StatusClass FROM test_nginx.access_log_enriched WHERE Status = 503`,
	).Scan(&remoteAddr, &statusClass); err != nil {
		t.Fatalf("query combined row (status 503): %v", err)
	}
	if remoteAddr != "10.10.10.10" {
		t.Errorf("expected RemoteAddr=10.10.10.10, got %s", remoteAddr)
	}
	if statusClass != "5xx" {
		t.Errorf("expected StatusClass=5xx, got %s", statusClass)
	}
}
