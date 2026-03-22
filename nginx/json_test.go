package nginx

import (
	"testing"
)

func TestParseJSONLogs(t *testing.T) {
	lines := []string{
		`{"remote_addr":"192.168.1.1","status":200,"request_time":0.123,"request":"GET /index.html HTTP/1.1"}`,
		`{"remote_addr":"10.0.0.1","status":404,"request_time":0.456,"request":"POST /api HTTP/1.1"}`,
	}

	entries := ParseJSONLogs(lines)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	addr, err := entries[0].Field("remote_addr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "192.168.1.1" {
		t.Errorf("expected remote_addr=192.168.1.1, got %s", addr)
	}

	addr, err = entries[1].Field("remote_addr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "10.0.0.1" {
		t.Errorf("expected remote_addr=10.0.0.1, got %s", addr)
	}

	status, err := entries[0].Field("status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if status != "200" {
		t.Errorf("expected status=200, got %s", status)
	}
}

func TestParseJSONLogsInvalidLine(t *testing.T) {
	lines := []string{
		`{"remote_addr":"192.168.1.1","status":200}`,
		`not valid json`,
		`{"remote_addr":"10.0.0.1","status":404}`,
	}

	entries := ParseJSONLogs(lines)

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries (invalid line skipped), got %d", len(entries))
	}

	addr, err := entries[0].Field("remote_addr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "192.168.1.1" {
		t.Errorf("expected remote_addr=192.168.1.1, got %s", addr)
	}

	addr, err = entries[1].Field("remote_addr")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if addr != "10.0.0.1" {
		t.Errorf("expected remote_addr=10.0.0.1, got %s", addr)
	}
}

func TestParseJSONLogsEmpty(t *testing.T) {
	entries := ParseJSONLogs([]string{})
	if entries != nil {
		t.Errorf("expected nil for empty input, got %v", entries)
	}
}

func TestJSONEntryFieldNotFound(t *testing.T) {
	entry := &JSONEntry{fields: map[string]string{"key": "value"}}

	_, err := entry.Field("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent field")
	}
}

func TestParseJSONLogsNumericValues(t *testing.T) {
	lines := []string{
		`{"int_val":42,"float_val":3.14,"bool_val":true,"null_val":null,"str_val":"hello"}`,
	}

	entries := ParseJSONLogs(lines)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	tests := []struct {
		field    string
		expected string
	}{
		{"int_val", "42"},
		{"float_val", "3.14"},
		{"bool_val", "true"},
		{"null_val", ""},
		{"str_val", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			val, err := entries[0].Field(tt.field)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if val != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, val)
			}
		})
	}
}
