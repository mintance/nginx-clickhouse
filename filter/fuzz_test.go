package filter

import (
	"fmt"
	"testing"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

// stubEntry is a minimal LogEntry for fuzz testing.
type stubEntry struct {
	fields map[string]string
}

func (e *stubEntry) Field(name string) (string, error) {
	v, ok := e.fields[name]
	if !ok {
		return "", fmt.Errorf("field %q not found", name)
	}
	return v, nil
}

func FuzzChainApply(f *testing.F) {
	f.Add(`status >= 500`, "drop", "200", "/index.html")
	f.Add(`request contains "/health"`, "drop", "200", "/healthz")
	f.Add(`status == 404`, "keep", "404", "/missing")
	f.Add(`status < 300`, "keep", "301", "/redirect")
	f.Add(`request == "GET / HTTP/1.1"`, "drop", "200", "GET / HTTP/1.1")

	fields := []string{"status", "request"}

	f.Fuzz(func(t *testing.T, expression, action, status, request string) {
		if action != "drop" && action != "keep" {
			t.Skip()
		}

		rules := []config.FilterRule{
			{Expr: expression, Action: action},
		}

		chain, err := NewChain(rules, fields)
		if err != nil {
			// Invalid expressions are expected — not a bug.
			t.Skip()
		}

		entry := &stubEntry{fields: map[string]string{
			"status":  status,
			"request": request,
		}}

		// Apply must not panic on any input.
		_ = chain.Apply([]nginx.LogEntry{entry})
	})
}
