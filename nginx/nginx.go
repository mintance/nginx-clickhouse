package nginx

import (
	"github.com/mintance/nginx-clickhouse/config"
	"github.com/satyrius/gonx"
	"io"
	"strconv"
	"strings"
	"time"
	"github.com/Sirupsen/logrus"
	"fmt"
)

func GetParser(config *config.Config) (*gonx.Parser, error) {

	// Use nginx config file to extract format by the name
	nginxConfig := strings.NewReader(fmt.Sprintf("%s%s%s",`
		http {
			log_format   main  '`, config.Nginx.LogFormat, `';
		}
	`))

	return gonx.NewNginxParser(nginxConfig, config.Nginx.LogType)
}

func ParseField(key string, value string) interface{} {

	switch key {
	case "time_local":

		t, err := time.Parse(config.NginxTimeLayout, value)

		if err == nil {
			return t.Format(config.CHTimeLayout)
		}

		return value

	case "remote_addr", "remote_user", "request", "http_referer", "http_user_agent", "request_method", "https":
		return value
	case "bytes_sent", "connections_waiting", "connections_active", "status", "connection", "request_length", "body_bytes_sent":
		val, err := strconv.Atoi(value)

		if err != nil {
			logrus.Error("Error to convert string to int")
		}

		return val
	case "request_time", "upstream_connect_time", "upstream_header_time", "upstream_response_time", "msec":
		val, err := strconv.ParseFloat(value, 32)

		if err != nil {
			logrus.Error("Error to convert string to float32")
		}

		return val
	default:
		return value
	}

	return value
}

func ParseLogs(parser *gonx.Parser, logLines []string) []gonx.Entry {

	logReader := strings.NewReader(strings.Join(logLines, "\n"))
	reader := gonx.NewParserReader(logReader, parser)

	var logs []gonx.Entry

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
