package nginx

import (
	"testing"

	"github.com/mintance/nginx-clickhouse/config"
)

func TestNewParser(t *testing.T) {
	cfg := &config.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$http_referer" "$http_user_agent"`

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
}

func TestNewParserInvalidLogType(t *testing.T) {
	cfg := &config.Config{}
	cfg.Nginx.LogType = "nonexistent"
	cfg.Nginx.LogFormat = `$remote_addr`

	_, err := NewParser(cfg)
	if err == nil {
		t.Fatal("expected error for nonexistent log type")
	}
}

func TestParseFieldTimeLocal(t *testing.T) {
	result := ParseField("time_local", "04/Nov/2018:12:30:45 +0000")
	expected := "2018-11-04 12:30:45"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestParseFieldTimeLocalInvalid(t *testing.T) {
	result := ParseField("time_local", "invalid-time")
	if result != "invalid-time" {
		t.Errorf("expected raw value for invalid time, got %q", result)
	}
}

func TestParseFieldStringTypes(t *testing.T) {
	tests := []struct {
		key   string
		value string
	}{
		{"remote_addr", "192.168.1.1"},
		{"remote_user", "admin"},
		{"request", "GET /index.html HTTP/1.1"},
		{"http_referer", "https://example.com"},
		{"http_user_agent", "Mozilla/5.0"},
		{"request_method", "POST"},
		{"https", "on"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ParseField(tt.key, tt.value)
			if result != tt.value {
				t.Errorf("expected %q, got %q", tt.value, result)
			}
		})
	}
}

func TestParseFieldIntTypes(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		expected int
	}{
		{"bytes_sent", "1024", 1024},
		{"status", "200", 200},
		{"connection", "42", 42},
		{"request_length", "512", 512},
		{"body_bytes_sent", "2048", 2048},
		{"connections_waiting", "10", 10},
		{"connections_active", "5", 5},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ParseField(tt.key, tt.value)
			intResult, ok := result.(int)
			if !ok {
				t.Fatalf("expected int, got %T", result)
			}
			if intResult != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, intResult)
			}
		})
	}
}

func TestParseFieldFloatTypes(t *testing.T) {
	tests := []struct {
		key      string
		value    string
		expected float64
	}{
		{"request_time", "0.123", 0.123},
		{"upstream_connect_time", "0.001", 0.001},
		{"upstream_header_time", "0.045", 0.045},
		{"upstream_response_time", "0.200", 0.200},
		{"msec", "12345.678", 12345.678},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := ParseField(tt.key, tt.value)
			floatResult, ok := result.(float64)
			if !ok {
				t.Fatalf("expected float64, got %T", result)
			}
			diff := floatResult - tt.expected
			if diff < -0.01 || diff > 0.01 {
				t.Errorf("expected ~%f, got %f", tt.expected, floatResult)
			}
		})
	}
}

func TestParseFieldUnknownKey(t *testing.T) {
	result := ParseField("unknown_field", "some_value")
	if result != "some_value" {
		t.Errorf("expected raw value for unknown field, got %q", result)
	}
}

func TestParseLogs(t *testing.T) {
	cfg := &config.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent`

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating parser: %v", err)
	}

	logLines := []string{
		`192.168.1.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /index.html HTTP/1.0" 200 2326`,
		`10.0.0.1 - - [10/Oct/2000:13:55:37 -0700] "POST /form HTTP/1.1" 301 512`,
	}

	entries := ParseLogs(parser, logLines)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	addrs := make(map[string]bool)
	for _, entry := range entries {
		addr, err := entry.Field("remote_addr")
		if err != nil {
			t.Fatalf("unexpected error getting field: %v", err)
		}
		addrs[addr] = true
	}

	if !addrs["192.168.1.1"] {
		t.Error("expected to find remote_addr=192.168.1.1")
	}
	if !addrs["10.0.0.1"] {
		t.Error("expected to find remote_addr=10.0.0.1")
	}
}

func TestParseLogsEmpty(t *testing.T) {
	cfg := &config.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent`

	parser, err := NewParser(cfg)
	if err != nil {
		t.Fatalf("unexpected error creating parser: %v", err)
	}

	entries := ParseLogs(parser, []string{})
	if len(entries) != 0 {
		t.Errorf("expected 0 entries for empty input, got %d", len(entries))
	}
}
