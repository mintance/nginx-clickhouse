// Command nginx-clickhouse tails NGINX access logs and batch-inserts parsed
// entries into ClickHouse.
package main

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/papertrail/go-tail/follower"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/clickhouse"
	configParser "github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

var (
	mu   sync.Mutex
	logs []string
)

var (
	linesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_processed_total",
		Help: "The total number of processed log lines",
	})
	linesNotProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_not_processed_total",
		Help: "The total number of log lines which were not processed",
	})
)

func main() {
	cfg := configParser.Read()
	cfg.SetEnvVariables()

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		logrus.Fatal("can't parse nginx log format: ", err)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		if err := http.ListenAndServe(":2112", nil); err != nil {
			logrus.Fatal("metrics server failed: ", err)
		}
	}()

	logrus.Info("opening log file: ", cfg.Settings.LogPath)

	whence := io.SeekStart
	if cfg.Settings.SeekFromEnd {
		whence = io.SeekEnd
	}

	t, err := follower.New(cfg.Settings.LogPath, follower.Config{
		Whence: whence,
		Offset: 0,
		Reopen: true,
	})
	if err != nil {
		logrus.Fatal("can't tail log file: ", err)
	}

	go flushLoop(cfg, parser)

	for line := range t.Lines() {
		mu.Lock()
		logs = append(logs, strings.TrimSpace(line.String()))
		mu.Unlock()
	}
}

// flushLoop periodically parses buffered log lines and saves them to ClickHouse.
func flushLoop(cfg *configParser.Config, parser *nginx.Parser) {
	interval := time.Duration(cfg.Settings.Interval) * time.Second
	for {
		time.Sleep(interval)

		mu.Lock()
		if len(logs) == 0 {
			mu.Unlock()
			continue
		}

		batch := logs
		logs = nil
		mu.Unlock()

		logrus.Info("preparing to save ", len(batch), " new log entries")

		entries := nginx.ParseLogs(parser, batch)
		if err := clickhouse.Save(cfg, entries); err != nil {
			logrus.Error("can't save logs: ", err)
			linesNotProcessed.Add(float64(len(batch)))
		} else {
			logrus.Info("saved ", len(batch), " new logs")
			linesProcessed.Add(float64(len(batch)))
		}
	}
}
