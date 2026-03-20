package clickhouse

import (
	"sort"
	"testing"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/satyrius/gonx"
)

func TestGetColumns(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr":    "remote_addr",
		"Status":        "status",
		"BodyBytesSent": "body_bytes_sent",
	}

	result := getColumns(columns)

	if len(result) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(result))
	}

	// Sort for deterministic comparison (map iteration order is random)
	sort.Strings(result)
	expected := []string{"BodyBytesSent", "RemoteAddr", "Status"}

	for i, col := range expected {
		if result[i] != col {
			t.Errorf("expected column %q at index %d, got %q", col, i, result[i])
		}
	}
}

func TestGetColumnsEmpty(t *testing.T) {
	columns := map[string]string{}
	result := getColumns(columns)
	if len(result) != 0 {
		t.Errorf("expected 0 columns, got %d", len(result))
	}
}

func TestBuildRows(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr": "remote_addr",
		"Status":     "status",
	}

	keys := []string{"RemoteAddr", "Status"}

	// Create gonx entries by parsing log lines
	cfg := &config.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr $status`

	parser := gonx.NewParser(cfg.Nginx.LogFormat)

	entry, err := parser.ParseString(`192.168.1.1 200`)
	if err != nil {
		t.Fatalf("unexpected error parsing log: %v", err)
	}

	entries := []gonx.Entry{*entry}
	rows := buildRows(keys, columns, entries)

	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if len(rows[0]) != 2 {
		t.Fatalf("expected 2 fields in row, got %d", len(rows[0]))
	}

	// Check that values are present (order matches keys)
	foundAddr := false
	foundStatus := false
	for _, val := range rows[0] {
		switch v := val.(type) {
		case string:
			if v == "192.168.1.1" {
				foundAddr = true
			}
		case int:
			if v == 200 {
				foundStatus = true
			}
		}
	}

	if !foundAddr {
		t.Error("expected to find remote_addr=192.168.1.1 in row")
	}
	if !foundStatus {
		t.Error("expected to find status=200 in row")
	}
}

func TestBuildRowsMultipleEntries(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr": "remote_addr",
	}
	keys := []string{"RemoteAddr"}

	parser := gonx.NewParser(`$remote_addr`)

	entry1, _ := parser.ParseString(`192.168.1.1`)
	entry2, _ := parser.ParseString(`10.0.0.1`)

	entries := []gonx.Entry{*entry1, *entry2}
	rows := buildRows(keys, columns, entries)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestBuildRowsEmpty(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr": "remote_addr",
	}
	keys := []string{"RemoteAddr"}

	rows := buildRows(keys, columns, []gonx.Entry{})

	if len(rows) != 0 {
		t.Errorf("expected 0 rows for empty entries, got %d", len(rows))
	}
}
