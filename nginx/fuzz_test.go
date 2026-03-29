package nginx

import (
	"testing"

	"github.com/mintance/nginx-clickhouse/config"
)

func FuzzParseField(f *testing.F) {
	// Seed corpus with representative field names and values.
	seeds := []struct {
		key, value string
	}{
		{"time_local", "04/Nov/2018:12:30:45 +0000"},
		{"status", "200"},
		{"status", "abc"},
		{"body_bytes_sent", "1024"},
		{"request_time", "0.123"},
		{"request_time", "not-a-float"},
		{"remote_addr", "192.168.1.1"},
		{"request", "GET /index.html HTTP/1.1"},
		{"http_user_agent", "Mozilla/5.0"},
		{"unknown_field", "some_value"},
		{"bytes_sent", ""},
		{"msec", "1541234567.890"},
	}
	for _, s := range seeds {
		f.Add(s.key, s.value)
	}

	f.Fuzz(func(t *testing.T, key, value string) {
		// ParseField must not panic on any input.
		_ = ParseField(key, value)
	})
}

func FuzzParseLogs(f *testing.F) {
	f.Add(`93.180.71.3 - - [17/May/2015:08:05:32 +0000] "GET /downloads/product_1 HTTP/1.1" 304 0 "-" "Debian APT-HTTP/1.3 (0.8.16~exp12ubuntu10.21)"`)
	f.Add(`127.0.0.1 - admin [04/Nov/2018:12:30:45 +0000] "POST /api HTTP/2.0" 201 512 "https://example.com" "curl/7.64.1"`)
	f.Add(`invalid log line that should not crash`)
	f.Add(``)

	cfg := &config.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`

	parser, err := NewParser(cfg)
	if err != nil {
		f.Fatalf("failed to create parser: %v", err)
	}

	f.Fuzz(func(t *testing.T, line string) {
		// ParseLogs must not panic on any input.
		_ = ParseLogs(parser, []string{line})
	})
}

func FuzzParseJSONLogs(f *testing.F) {
	f.Add(`{"remote_addr":"127.0.0.1","status":"200","request":"GET / HTTP/1.1"}`)
	f.Add(`{"status":404,"request_time":0.123,"body_bytes_sent":1024}`)
	f.Add(`{"nested":{"key":"value"},"array":[1,2,3]}`)
	f.Add(`not json at all`)
	f.Add(``)
	f.Add(`{}`)
	f.Add(`{"bool_field":true,"null_field":null}`)

	f.Fuzz(func(t *testing.T, line string) {
		// ParseJSONLogs must not panic on any input.
		_ = ParseJSONLogs([]string{line})
	})
}
