package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	configParser "github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/filter"
	"github.com/mintance/nginx-clickhouse/nginx"
)

func TestScanLinesFromReader(t *testing.T) {
	input := "line1\nline2\nline3\n"
	ch := scanLines(strings.NewReader(input))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(got))
	}
	if got[0] != "line1" || got[1] != "line2" || got[2] != "line3" {
		t.Errorf("unexpected lines: %v", got)
	}
}

func TestScanLinesEmpty(t *testing.T) {
	ch := scanLines(strings.NewReader(""))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 0 {
		t.Errorf("expected 0 lines, got %d", len(got))
	}
}

func TestScanLinesClosesFile(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")
	if err := os.WriteFile(tmpFile, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(tmpFile)
	if err != nil {
		t.Fatal(err)
	}

	ch := scanLines(f)
	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}

	// File should be closed after channel is drained.
	_, err = f.Read(make([]byte, 1))
	if err == nil {
		t.Error("expected error reading from closed file, got nil")
	}
}

func TestScanLinesFromPipe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	ch := scanLines(r)

	go func() {
		io.WriteString(w, "from-pipe-1\nfrom-pipe-2\n")
		w.Close()
	}()

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[0] != "from-pipe-1" || got[1] != "from-pipe-2" {
		t.Errorf("unexpected lines: %v", got)
	}
}

func TestScanLinesNoTrailingNewline(t *testing.T) {
	input := "line1\nline2"
	ch := scanLines(strings.NewReader(input))

	var got []string
	for line := range ch {
		got = append(got, line)
	}

	if len(got) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(got))
	}
	if got[1] != "line2" {
		t.Errorf("expected last line 'line2', got %q", got[1])
	}
}

func TestRunParse(t *testing.T) {
	cfg := &configParser.Config{}
	cfg.Nginx.LogType = "main"
	cfg.Nginx.LogFormat = `$remote_addr "$request" $status`
	cfg.ClickHouse.Columns = map[string]string{
		"RemoteAddr":  "remote_addr",
		"Status":      "status",
		"StatusClass": "_status_class",
	}
	parser, err := nginx.NewParser(cfg)
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	fc, err := filter.NewChain([]configParser.FilterRule{
		{Expr: "status == 200", Action: "drop"},
	}, []string{"remote_addr", "status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	input := "10.0.0.1 \"GET / HTTP/1.1\" 200\n\n10.0.0.2 \"GET /x HTTP/1.1\" 503\n"
	var out bytes.Buffer
	if ok := runParse(cfg, parser, fc, strings.NewReader(input), &out); !ok {
		t.Fatalf("runParse reported failure; output:\n%s", out.String())
	}

	var results []parseResult
	dec := json.NewDecoder(&out)
	for dec.More() {
		var r parseResult
		if err := dec.Decode(&r); err != nil {
			t.Fatalf("decode: %v", err)
		}
		results = append(results, r)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results (blank line skipped), got %d", len(results))
	}
	if results[1].Line != 3 {
		t.Errorf("expected second result on line 3, got %d", results[1].Line)
	}
	if results[0].Columns["StatusClass"] != "2xx" || results[0].Filter.Kept {
		t.Errorf("line 1: got columns %v filter %+v", results[0].Columns, results[0].Filter)
	}
	if results[1].Columns["Status"] != float64(503) || !results[1].Filter.Kept {
		t.Errorf("line 3: got columns %v filter %+v", results[1].Columns, results[1].Filter)
	}
}

func TestRunParseFailures(t *testing.T) {
	cfg := &configParser.Config{}
	cfg.ClickHouse.Columns = map[string]string{
		"Status":   "status",
		"Upstream": "upstream_response_time",
	}

	tests := []struct {
		name  string
		input string
		check func(parseResult) bool
	}{
		{"invalid line", "{bad\n", func(r parseResult) bool { return r.Error != "" && r.Raw == "{bad" }},
		{"missing field", `{"status":200}` + "\n", func(r parseResult) bool {
			return len(r.Missing) == 1 && r.Missing[0] == "upstream_response_time"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if ok := runParse(cfg, nil, nil, strings.NewReader(tt.input), &out); ok {
				t.Error("expected runParse to report failure")
			}
			var r parseResult
			if err := json.Unmarshal(out.Bytes(), &r); err != nil {
				t.Fatalf("unmarshal %q: %v", out.String(), err)
			}
			if !tt.check(r) {
				t.Errorf("unexpected result: %+v", r)
			}
			if r.Filter != nil {
				t.Errorf("expected no filter decision without a chain, got %+v", r.Filter)
			}
		})
	}
}
