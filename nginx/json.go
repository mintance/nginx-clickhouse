// Package nginx provides NGINX access log parsing using configurable log formats.

package nginx

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/sirupsen/logrus"
)

// JSONEntry represents a parsed JSON log line.
type JSONEntry struct {
	fields map[string]string
}

// Field returns the value of the named field.
func (e *JSONEntry) Field(name string) (string, error) {
	v, ok := e.fields[name]
	if !ok {
		return "", fmt.Errorf("field %q not found", name)
	}
	return v, nil
}

// ParseJSONLogs parses JSON-formatted NGINX log lines into LogEntry values.
// Each line must be a valid JSON object. Fields are flattened to string values.
func ParseJSONLogs(logLines []string) []LogEntry {
	if len(logLines) == 0 {
		return nil
	}

	var entries []LogEntry
	for _, line := range logLines {
		var raw map[string]any
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			logrus.WithError(err).WithField("line", line).Error("failed to parse JSON log line")
			continue
		}

		fields := make(map[string]string, len(raw))
		for k, v := range raw {
			fields[k] = anyToString(v)
		}
		entries = append(entries, &JSONEntry{fields: fields})
	}

	return entries
}

// anyToString converts an arbitrary JSON value to its string representation.
func anyToString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case float64:
		// Use integer formatting when the value has no fractional part.
		if val == float64(int64(val)) {
			return strconv.FormatInt(int64(val), 10)
		}
		return strconv.FormatFloat(val, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(val)
	case nil:
		return ""
	default:
		b, err := json.Marshal(val)
		if err != nil {
			return fmt.Sprintf("%v", val)
		}
		return string(b)
	}
}
