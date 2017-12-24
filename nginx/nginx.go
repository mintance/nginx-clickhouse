package main

import (
	"github.com/satyrius/gonx"
	"io"
	"strconv"
	"strings"
	"time"
)

func getParser(config *Config) (*gonx.Parser, error) {

	// Use nginx config file to extract format by the name
	nginxConfig := strings.NewReader(`
		http {
			log_format   main  '` + config.Nginx.LogFormat + `';
		}
	`)

	return gonx.NewNginxParser(nginxConfig, config.Nginx.LogType)
}

func parseField(key string, value string) interface{} {

	switch key {
	case "time_local":

		t, err := time.Parse(nginxTimeLayout, value)

		if err == nil {
			return t.Format(chTimeLayout)
		} else {
			return value
		}

	case "remote_addr", "remote_user", "request", "http_referer", "http_user_agent", "request_method", "https":
		return value
	case "bytes_sent", "connections_waiting", "connections_active", "status":
		val, _ := strconv.Atoi(value)

		return val
	case "request_time", "upstream_connect_time", "upstream_header_time", "upstream_response_time":
		val, _ := strconv.ParseFloat(value, 32)

		return val
	default:
		return value
	}

	return value
}

func parseLogs(parser *gonx.Parser, log_lines []string) []gonx.Entry {

	logReader := strings.NewReader(strings.Join(log_lines, "\n"))

	reader := gonx.NewParserReader(logReader, parser)

	logs := []gonx.Entry{}

	for {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			panic(err)
		}
		// Process the record... e.g.
		logs = append(logs, *rec)
	}

	return logs
}
