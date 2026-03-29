package filter

import (
	"fmt"
	"testing"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

// testEntry is a minimal LogEntry for testing.
type testEntry struct {
	fields map[string]string
}

func (e *testEntry) Field(name string) (string, error) {
	v, ok := e.fields[name]
	if !ok {
		return "", fmt.Errorf("field %q not found", name)
	}
	return v, nil
}

func TestNewChainInvalidExpr(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "???invalid", Action: "drop"},
	}
	_, err := NewChain(rules, []string{"status"})
	if err == nil {
		t.Fatal("expected error for invalid expression")
	}
}

func TestNewChainInvalidAction(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "invalid"},
	}
	_, err := NewChain(rules, []string{"status"})
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestDropRule(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status >= 200 && status < 300", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
		&testEntry{fields: map[string]string{"status": "500"}},
		&testEntry{fields: map[string]string{"status": "201"}},
	}

	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("status")
	if v != "500" {
		t.Errorf("expected status 500, got %s", v)
	}
}

func TestKeepRule(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status >= 500", Action: "keep"},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
		&testEntry{fields: map[string]string{"status": "503"}},
	}

	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("status")
	if v != "503" {
		t.Errorf("expected status 503, got %s", v)
	}
}

func TestMultipleRules(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: `request == "/healthz"`, Action: "drop"},
		{Expr: "status >= 500", Action: "keep"},
	}
	chain, err := NewChain(rules, []string{"status", "request"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200", "request": "/healthz"}},
		&testEntry{fields: map[string]string{"status": "500", "request": "/api"}},
		&testEntry{fields: map[string]string{"status": "200", "request": "/api"}},
	}

	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("status")
	if v != "500" {
		t.Errorf("expected status 500, got %s", v)
	}
}

func TestEmptyChain(t *testing.T) {
	chain, err := NewChain(nil, nil)
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry (passthrough), got %d", len(result))
	}
}

func TestSampleRateZeroKeepsAll(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "keep", SampleRate: 0},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestSampleRateOneKeepsAll(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "keep", SampleRate: 1.0},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := make([]nginx.LogEntry, 100)
	for i := range entries {
		entries[i] = &testEntry{fields: map[string]string{"status": "200"}}
	}
	result := chain.Apply(entries)
	if len(result) != 100 {
		t.Errorf("expected 100 entries, got %d", len(result))
	}
}

func TestDropWithSamplingDropsFraction(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "drop", SampleRate: 1.0},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := make([]nginx.LogEntry, 100)
	for i := range entries {
		entries[i] = &testEntry{fields: map[string]string{"status": "200"}}
	}
	result := chain.Apply(entries)
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestFloatFieldComparison(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "request_time == 0", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"request_time"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"request_time": "0.000"}},
		&testEntry{fields: map[string]string{"request_time": "1.234"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestStringContains(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: `request contains "/health"`, Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"request"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"request": "GET /healthz HTTP/1.1"}},
		&testEntry{fields: map[string]string{"request": "GET /api/users HTTP/1.1"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestRegexMatch(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: `request matches "^GET /health"`, Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"request"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"request": "GET /healthz HTTP/1.1"}},
		&testEntry{fields: map[string]string{"request": "POST /api HTTP/1.1"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}

func TestSampleRateOutOfBounds(t *testing.T) {
	tests := []struct {
		name string
		rate float64
	}{
		{"negative", -0.5},
		{"over_one", 2.0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rules := []config.FilterRule{
				{Expr: "status == 200", Action: "drop", SampleRate: tt.rate},
			}
			_, err := NewChain(rules, []string{"status"})
			if err == nil {
				t.Fatalf("expected error for sample_rate %g", tt.rate)
			}
		})
	}
}

func TestDropWithFractionalSampling(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "drop", SampleRate: 0.5},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	const n = 10000
	entries := make([]nginx.LogEntry, n)
	for i := range entries {
		entries[i] = &testEntry{fields: map[string]string{"status": "200"}}
	}
	result := chain.Apply(entries)
	// With rate 0.5, roughly 50% should be dropped, 50% kept.
	// Allow wide margin for randomness.
	kept := len(result)
	if kept < 3000 || kept > 7000 {
		t.Errorf("expected ~5000 kept, got %d (outside 3000-7000 range)", kept)
	}
}

func TestKeepWithFractionalSampling(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "keep", SampleRate: 0.5},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	const n = 10000
	entries := make([]nginx.LogEntry, n)
	for i := range entries {
		entries[i] = &testEntry{fields: map[string]string{"status": "200"}}
	}
	result := chain.Apply(entries)
	kept := len(result)
	if kept < 3000 || kept > 7000 {
		t.Errorf("expected ~5000 kept, got %d (outside 3000-7000 range)", kept)
	}
}

func TestMissingFieldUsesZeroValue(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "request_time > 1", Action: "keep"},
	}
	// Compile with request_time as a known field
	chain, err := NewChain(rules, []string{"request_time"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	// Entry has NO request_time field — should default to 0.0, which fails > 1
	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
	}
	result := chain.Apply(entries)
	if len(result) != 0 {
		t.Errorf("expected 0 entries (missing field defaults to 0), got %d", len(result))
	}
}

func TestNonNumericValueInNumericField(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 0", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	// "abc" can't be parsed as int, falls back to 0, matches status == 0
	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "abc"}},
		&testEntry{fields: map[string]string{"status": "200"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("status")
	if v != "200" {
		t.Errorf("expected status 200, got %s", v)
	}
}

func TestApplyEmptyEntries(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: "status == 200", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	result := chain.Apply(nil)
	if len(result) != 0 {
		t.Errorf("expected 0 entries, got %d", len(result))
	}
}

func TestMultipleDropRules(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: `request contains "/health"`, Action: "drop"},
		{Expr: "status == 304", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"status", "request"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200", "request": "/healthz"}},
		&testEntry{fields: map[string]string{"status": "304", "request": "/api"}},
		&testEntry{fields: map[string]string{"status": "200", "request": "/api"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("request")
	if v != "/api" {
		t.Errorf("expected /api, got %s", v)
	}
}

func TestRuleOrderMatters(t *testing.T) {
	// keep-then-drop: keep 5xx first, then drop 503 specifically
	rules := []config.FilterRule{
		{Expr: "status >= 500", Action: "keep"},
		{Expr: "status == 503", Action: "drop"},
	}
	chain, err := NewChain(rules, []string{"status"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"status": "200"}},
		&testEntry{fields: map[string]string{"status": "500"}},
		&testEntry{fields: map[string]string{"status": "503"}},
	}
	result := chain.Apply(entries)
	// keep >= 500 leaves [500, 503], then drop 503 leaves [500]
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	v, _ := result[0].Field("status")
	if v != "500" {
		t.Errorf("expected status 500, got %s", v)
	}
}

func TestUnknownFieldAsString(t *testing.T) {
	rules := []config.FilterRule{
		{Expr: `gzip_ratio == "5.00"`, Action: "keep"},
	}
	chain, err := NewChain(rules, []string{"gzip_ratio"})
	if err != nil {
		t.Fatalf("NewChain: %v", err)
	}

	entries := []nginx.LogEntry{
		&testEntry{fields: map[string]string{"gzip_ratio": "5.00"}},
		&testEntry{fields: map[string]string{"gzip_ratio": "1.20"}},
	}
	result := chain.Apply(entries)
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
}
