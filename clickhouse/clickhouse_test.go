package clickhouse

import (
	"testing"

	"github.com/satyrius/gonx"

	"github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{}
	client := NewClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.conn != nil {
		t.Error("expected conn to be nil on new client")
	}
}

func TestSaveEmptyLogs(t *testing.T) {
	cfg := &config.Config{}
	cfg.ClickHouse.Columns = map[string]string{"RemoteAddr": "remote_addr"}
	client := NewClient(cfg)

	err := client.Save(nil)
	if err != nil {
		t.Errorf("Save with nil logs should return nil, got %v", err)
	}
}

func TestSaveEmptyColumns(t *testing.T) {
	cfg := &config.Config{}
	cfg.ClickHouse.Columns = map[string]string{}
	client := NewClient(cfg)

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	err := client.Save([]nginx.LogEntry{entry})
	if err != nil {
		t.Errorf("Save with empty columns should return nil, got %v", err)
	}
}

func TestHealthyNoConnection(t *testing.T) {
	cfg := &config.Config{}
	client := NewClient(cfg)

	if client.Healthy() {
		t.Error("expected Healthy to return false when conn is nil")
	}
}

func TestCloseNoConnection(t *testing.T) {
	cfg := &config.Config{}
	client := NewClient(cfg)

	err := client.Close()
	if err != nil {
		t.Errorf("Close with nil conn should return nil, got %v", err)
	}
}

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

	row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

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

	row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

	if len(row) != 2 {
		t.Fatalf("expected 2 fields (with fallback), got %d", len(row))
	}
}

func TestBuildRowEmpty(t *testing.T) {
	columns := map[string]string{}
	keys := []string{}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

	if len(row) != 0 {
		t.Errorf("expected 0 fields for empty columns, got %d", len(row))
	}
}

func TestBuildRowEnrichment(t *testing.T) {
	columns := map[string]string{
		"Hostname": "_hostname",
	}
	keys := []string{"Hostname"}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	enrichments := &config.EnrichmentConfig{Hostname: "web-01"}
	row := buildRow(keys, columns, entry, enrichments)

	if len(row) != 1 {
		t.Fatalf("expected 1 field, got %d", len(row))
	}
	if row[0] != "web-01" {
		t.Errorf("expected Hostname=web-01, got %v", row[0])
	}
}

func TestBuildRowStatusClass(t *testing.T) {
	columns := map[string]string{
		"StatusClass": "_status_class",
	}
	keys := []string{"StatusClass"}

	parser := gonx.NewParser(`$status`)
	entry, _ := parser.ParseString(`200`)

	row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

	if len(row) != 1 {
		t.Fatalf("expected 1 field, got %d", len(row))
	}
	if row[0] != "2xx" {
		t.Errorf("expected StatusClass=2xx, got %v", row[0])
	}
}

func TestBuildRowStatusClassVariants(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"success", "200", "2xx"},
		{"redirect", "301", "3xx"},
		{"client error", "404", "4xx"},
		{"server error", "500", "5xx"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := map[string]string{"StatusClass": "_status_class"}
			keys := []string{"StatusClass"}

			parser := gonx.NewParser(`$status`)
			entry, _ := parser.ParseString(tt.status)

			row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

			if len(row) != 1 {
				t.Fatalf("expected 1 field, got %d", len(row))
			}
			if row[0] != tt.expected {
				t.Errorf("expected StatusClass=%s, got %v", tt.expected, row[0])
			}
		})
	}
}

func TestBuildRowStatusClassInvalid(t *testing.T) {
	tests := []struct {
		name   string
		status string
	}{
		{"non-numeric", "OK"},
		{"zero prefix", "099"},
		{"six prefix", "600"},
		{"empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := map[string]string{"StatusClass": "_status_class"}
			keys := []string{"StatusClass"}

			parser := gonx.NewParser(`$status`)
			entry, _ := parser.ParseString(tt.status)

			row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})

			if len(row) != 1 {
				t.Fatalf("expected 1 field, got %d", len(row))
			}
			if row[0] != "" {
				t.Errorf("expected empty string for invalid status %q, got %v", tt.status, row[0])
			}
		})
	}
}

func TestBuildRowExtraEnrichment(t *testing.T) {
	columns := map[string]string{
		"MyTag": "_extra.my_tag",
	}
	keys := []string{"MyTag"}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	enrichments := &config.EnrichmentConfig{
		Extra: map[string]string{"my_tag": "us-east-1"},
	}
	row := buildRow(keys, columns, entry, enrichments)

	if len(row) != 1 {
		t.Fatalf("expected 1 field, got %d", len(row))
	}
	if row[0] != "us-east-1" {
		t.Errorf("expected MyTag=us-east-1, got %v", row[0])
	}
}

func TestBuildRowExtraEnrichmentMissingKey(t *testing.T) {
	columns := map[string]string{
		"MyTag": "_extra.nonexistent",
	}
	keys := []string{"MyTag"}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`192.168.1.1`)

	enrichments := &config.EnrichmentConfig{
		Extra: map[string]string{"my_tag": "value"},
	}
	row := buildRow(keys, columns, entry, enrichments)

	if len(row) != 1 {
		t.Fatalf("expected 1 field, got %d", len(row))
	}
	if row[0] != "" {
		t.Errorf("expected empty string for missing extra key, got %v", row[0])
	}
}

func TestBuildRowReferrerDomain(t *testing.T) {
	tests := []struct {
		name     string
		referer  string
		expected string
	}{
		{"full url", "https://example.com/page?q=1", "example.com"},
		{"with port", "https://example.com:8080/page", "example.com"},
		{"http", "http://sub.domain.org/path", "sub.domain.org"},
		{"dash", "-", ""},
		{"empty", "", ""},
		{"bare path", "/local/path", ""},
		{"invalid url", "://bad", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := map[string]string{"RefDomain": "_referrer_domain"}
			keys := []string{"RefDomain"}

			parser := gonx.NewParser(`$http_referer`)
			entry, _ := parser.ParseString(tt.referer)

			row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})
			if len(row) != 1 {
				t.Fatalf("expected 1 field, got %d", len(row))
			}
			if row[0] != tt.expected {
				t.Errorf("expected %q, got %v", tt.expected, row[0])
			}
		})
	}
}

func TestBuildRowURLExtension(t *testing.T) {
	tests := []struct {
		name     string
		request  string
		expected string
	}{
		{"html", "GET /index.html HTTP/1.1", "html"},
		{"js with query", "GET /app.js?v=123 HTTP/1.1", "js"},
		{"css with fragment", "GET /style.css#section HTTP/1.1", "css"},
		{"no extension", "GET /api/users HTTP/1.1", ""},
		{"root", "GET / HTTP/1.1", ""},
		{"image", "GET /logo.png HTTP/2.0", "png"},
		{"dotfile", "GET /.env HTTP/1.1", "env"},
		{"empty", "", ""},
		{"method only", "GET", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := map[string]string{"Ext": "_url_extension"}
			keys := []string{"Ext"}

			parser := gonx.NewParser(`"$request"`)
			input := `"` + tt.request + `"`
			if tt.request == "" {
				input = `""`
			}
			entry, _ := parser.ParseString(input)

			row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})
			if len(row) != 1 {
				t.Fatalf("expected 1 field, got %d", len(row))
			}
			if row[0] != tt.expected {
				t.Errorf("expected %q, got %v", tt.expected, row[0])
			}
		})
	}
}

func TestBuildRowIsBot(t *testing.T) {
	tests := []struct {
		name     string
		ua       string
		expected string
	}{
		{"googlebot", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", "1"},
		{"bingbot", "Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", "1"},
		{"curl", "curl/7.68.0", "1"},
		{"wget", "Wget/1.20.3", "1"},
		{"chrome", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", "0"},
		{"firefox", "Mozilla/5.0 (Windows NT 10.0; rv:121.0) Gecko/20100101 Firefox/121.0", "0"},
		{"empty", "", "0"},
		{"dash", "-", "0"},
		{"ahrefsbot", "Mozilla/5.0 (compatible; AhrefsBot/7.0; +http://ahrefs.com/robot/)", "1"},
		{"gptbot", "Mozilla/5.0 AppleWebKit/537.36 (KHTML, like Gecko; compatible; GPTBot/1.0; +https://openai.com/gptbot)", "1"},
		{"claudebot", "ClaudeBot/1.0", "1"},
		{"semrush", "Mozilla/5.0 (compatible; SemrushBot/7; +http://www.semrush.com/bot.html)", "1"},
		{"facebookbot", "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", "1"},
		{"whatsapp", "WhatsApp/2.23.20.0", "1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			columns := map[string]string{"Bot": "_is_bot"}
			keys := []string{"Bot"}

			parser := gonx.NewParser(`"$http_user_agent"`)
			input := `"` + tt.ua + `"`
			if tt.ua == "" {
				input = `""`
			}
			entry, _ := parser.ParseString(input)

			row := buildRow(keys, columns, entry, &config.EnrichmentConfig{})
			if len(row) != 1 {
				t.Fatalf("expected 1 field, got %d", len(row))
			}
			if row[0] != tt.expected {
				t.Errorf("expected %q, got %v", tt.expected, row[0])
			}
		})
	}
}

func TestBuildRowAllEnrichments(t *testing.T) {
	columns := map[string]string{
		"Hostname":    "_hostname",
		"Environment": "_environment",
		"Service":     "_service",
		"RemoteAddr":  "remote_addr",
	}
	keys := []string{"Environment", "Hostname", "RemoteAddr", "Service"}

	parser := gonx.NewParser(`$remote_addr`)
	entry, _ := parser.ParseString(`10.0.0.1`)

	enrichments := &config.EnrichmentConfig{
		Hostname:    "web-02",
		Environment: "production",
		Service:     "api-gateway",
	}
	row := buildRow(keys, columns, entry, enrichments)

	if len(row) != 4 {
		t.Fatalf("expected 4 fields, got %d", len(row))
	}

	expected := map[int]any{
		0: "production",
		1: "web-02",
		2: "10.0.0.1",
		3: "api-gateway",
	}
	for i, want := range expected {
		if row[i] != want {
			t.Errorf("row[%d]: expected %v, got %v", i, want, row[i])
		}
	}
}
