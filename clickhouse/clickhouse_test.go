package clickhouse

import (
	"testing"

	"github.com/satyrius/gonx"

	"github.com/mintance/nginx-clickhouse/config"
)

func TestBuildRow(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr": "remote_addr",
		"Status":     "status",
	}
	keys := []string{"RemoteAddr", "Status"}

	parser := gonx.NewParser(`$remote_addr $status`)
	entry, err := parser.ParseString(`192.168.1.1 200`)
	if err != nil {
		t.Fatalf("unexpected error parsing log: %v", err)
	}

	row := buildRow(keys, columns, *entry)

	if len(row) != 2 {
		t.Fatalf("expected 2 fields in row, got %d", len(row))
	}

	foundAddr := false
	foundStatus := false
	for _, val := range row {
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

func TestBuildRowMissingField(t *testing.T) {
	columns := map[string]string{
		"RemoteAddr": "remote_addr",
		"Status":     "nonexistent_field",
	}
	keys := []string{"RemoteAddr", "Status"}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	row := buildRow(keys, columns, *entry)

	if len(row) != 2 {
		t.Fatalf("expected 2 fields (with fallback), got %d", len(row))
	}
}

func TestBuildRowEmpty(t *testing.T) {
	cfg := &config.Config{}
	_ = cfg // ensure config import is used for consistency

	columns := map[string]string{}
	keys := []string{}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	row := buildRow(keys, columns, *entry)

	if len(row) != 0 {
		t.Errorf("expected 0 fields for empty columns, got %d", len(row))
	}
}
