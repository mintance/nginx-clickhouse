package nginx

import (
	"fmt"
	"github.com/WinnerSoftLab/nginx-clickhouse/config"
	"github.com/satyrius/gonx"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"time"
)

func GetParser(config *config.Config) (*gonx.Parser, error) {
	nginxConfig := strings.NewReader(fmt.Sprintf("%s%s%s", `
		http {
			log_format   main  '`, config.Nginx.LogFormat, `';
		}
	`))
	return gonx.NewNginxParser(nginxConfig, config.Nginx.LogType)
}

func ParseField(key string, value string) interface{} {
	switch key {
	case "msec":
		val, err := strconv.ParseFloat(value, 64)
		if err != nil {
			logrus.Errorf("Failed to parse key: %s, value: %s", key, value)
			return time.Now()
		}
		t := time.Unix(int64(val), 0)
		return t

	case "time_local":
		t, err := time.Parse(config.NginxTimeLayout, value)
		if err == nil {
			return t.Format(config.CHTimeLayout)
		}
		return value
	case "bytes_sent", "connections_waiting", "connections_active", "status", "connection", "request_length", "body_bytes_sent":
		val, err := strconv.Atoi(value)
		if err != nil {
			logrus.Errorf("Failed to parse key: %s, value: %s", key, value)
		}
		return val
	case "request_time", "upstream_connect_time", "upstream_header_time", "upstream_response_time":
		val, err := strconv.ParseFloat(value, 32)
		if err != nil {
			logrus.Errorf("Failed to parse key: %s, value: %s", key, value)
		}
		dec := decimal.NewFromFloat32(float32(val))
		return dec
	default:
		return value
	}
	return value
}

func ParseLogs(parser *gonx.Parser, logLines []string) ([]*gonx.Entry, error) {
	result := make([]*gonx.Entry, 0, len(logLines))
	for _, line := range logLines {
		enty, err := parser.ParseString(line)
		if err != nil {
			return nil, err
		}
		result = append(result, enty)
	}
	return result, nil
}
