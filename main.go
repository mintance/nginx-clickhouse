// Command nginx-clickhouse tails NGINX access logs and batch-inserts parsed
// entries into ClickHouse.
package main

import (
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
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

const defaultMaxBufferSize = 10000

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
	linesRead = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_read_total",
		Help: "Total lines read from the log file",
	})
	parseErrors = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_parse_errors_total",
		Help: "Total lines that failed to parse",
	})

	bufferSize = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nginx_clickhouse_buffer_size",
		Help: "Current number of lines in the buffer",
	})
	clickhouseUp = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nginx_clickhouse_clickhouse_up",
		Help: "Whether ClickHouse is reachable (1=up, 0=down)",
	})

	flushDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "nginx_clickhouse_flush_duration_seconds",
		Help:    "Time spent flushing a batch (parse + save)",
		Buckets: prometheus.DefBuckets,
	})
	batchSize = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "nginx_clickhouse_batch_size",
		Help:    "Number of log entries per flush",
		Buckets: []float64{10, 50, 100, 500, 1000, 5000, 10000},
	})
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	cfg := configParser.Read()
	cfg.SetEnvVariables()

	if cfg.Settings.MaxBufferSize == 0 {
		cfg.Settings.MaxBufferSize = defaultMaxBufferSize
	}

	parser, err := nginx.NewParser(cfg)
	if err != nil {
		logrus.Fatal("can't parse nginx log format: ", err)
	}

	client := clickhouse.NewClient(cfg)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			if client.Healthy() {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				w.Write([]byte("clickhouse unreachable"))
			}
		})
		if err := http.ListenAndServe(":2112", nil); err != nil {
			logrus.Fatal("metrics server failed: ", err)
		}
	}()

	logrus.WithField("path", cfg.Settings.LogPath).Info("opening log file")

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

	go flushLoop(cfg, parser, client)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logrus.WithField("signal", sig.String()).Info("received shutdown signal")

		// Flush remaining buffer.
		flush(parser, client)

		// Close ClickHouse connection.
		client.Close()

		logrus.Info("shutdown complete")
		os.Exit(0)
	}()

	for line := range t.Lines() {
		linesRead.Inc()
		mu.Lock()
		logs = append(logs, strings.TrimSpace(line.String()))
		bufferSize.Set(float64(len(logs)))
		shouldFlush := len(logs) >= cfg.Settings.MaxBufferSize
		mu.Unlock()

		if shouldFlush {
			flush(parser, client)
		}
	}
}

// flushLoop periodically flushes buffered log lines to ClickHouse.
func flushLoop(cfg *configParser.Config, parser *nginx.Parser, client *clickhouse.Client) {
	interval := time.Duration(cfg.Settings.Interval) * time.Second
	for {
		time.Sleep(interval)
		flush(parser, client)
	}
}

// flush drains the log buffer and saves entries to ClickHouse.
func flush(parser *nginx.Parser, client *clickhouse.Client) {
	mu.Lock()
	if len(logs) == 0 {
		mu.Unlock()
		return
	}

	batch := logs
	logs = nil
	bufferSize.Set(0)
	mu.Unlock()

	start := time.Now()

	logrus.WithField("entries", len(batch)).Info("preparing to save log entries")

	entries := nginx.ParseLogs(parser, batch)

	parseErrs := float64(len(batch) - len(entries))
	if parseErrs > 0 {
		parseErrors.Add(parseErrs)
	}
	batchSize.Observe(float64(len(entries)))

	if err := client.Save(entries); err != nil {
		logrus.WithError(err).Error("can't save logs")
		linesNotProcessed.Add(float64(len(batch)))
		clickhouseUp.Set(0)
	} else {
		logrus.WithField("entries", len(batch)).Info("saved log entries")
		linesProcessed.Add(float64(len(batch)))
		clickhouseUp.Set(1)
	}

	flushDuration.Observe(time.Since(start).Seconds())
}
