// Package nginx provides NGINX access log parsing using configurable log formats.
package nginx

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/satyrius/gonx"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/config"
)

// Parser is a type alias for [gonx.Parser] to avoid leaking the dependency.
type Parser = gonx.Parser

// NewParser creates a NGINX log parser from the log format string in cfg.
func NewParser(cfg *config.Config) (*Parser, error) {
	nginxConfig := strings.NewReader(fmt.Sprintf(`
		http {
			log_format   main  '%s';
		}
	`, cfg.Nginx.LogFormat))

	return gonx.NewNginxParser(nginxConfig, cfg.Nginx.LogType)
}

// ParseField converts a raw NGINX log field value to the appropriate Go type
// based on the field name. Time fields are reformatted to the ClickHouse
// DateTime layout, integer fields are converted to int, and float fields to
// float64. Unknown fields are returned as strings.
func ParseField(key, value string) any {
	switch key {
	case "time_local":
		t, err := time.Parse(config.NginxTimeLayout, value)
		if err != nil {
			return value
		}
		return t.Format(config.CHTimeLayout)

	case "remote_addr", "remote_user", "request", "http_referer",
		"http_user_agent", "request_method", "https":
		return value

	case "bytes_sent", "connections_waiting", "connections_active",
		"status", "connection", "request_length", "body_bytes_sent":
		val, err := strconv.Atoi(value)
		if err != nil {
			logrus.WithFields(logrus.Fields{"field": key, "value": value}).WithError(err).Error("cannot convert to int")
		}
		return val

	case "request_time", "upstream_connect_time", "upstream_header_time",
		"upstream_response_time", "msec":
		val, err := strconv.ParseFloat(value, 32)
		if err != nil {
			logrus.WithFields(logrus.Fields{"field": key, "value": value}).WithError(err).Error("cannot convert to float")
		}
		return val

	default:
		return value
	}
}

// ParseLogs parses multiple raw NGINX log lines into structured entries using
// the provided parser. Lines that fail to parse are logged and skipped.
func ParseLogs(parser *Parser, logLines []string) []gonx.Entry {
	if len(logLines) == 0 {
		return nil
	}

	logReader := strings.NewReader(strings.Join(logLines, "\n"))
	reader := gonx.NewParserReader(logReader, parser)

	var entries []gonx.Entry
	for {
		rec, err := reader.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			logrus.Errorf("failed to parse log line: %v", err)
			continue
		}
		entries = append(entries, *rec)
	}

	return entries
}
