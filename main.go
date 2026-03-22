// Command nginx-clickhouse tails NGINX access logs and batch-inserts parsed
// entries into ClickHouse.
package main

import (
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/papertrail/go-tail/follower"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"

	"github.com/mintance/nginx-clickhouse/buffer"
	"github.com/mintance/nginx-clickhouse/circuitbreaker"
	"github.com/mintance/nginx-clickhouse/clickhouse"
	configParser "github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
)

const defaultMaxBufferSize = 10000

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

	cbState = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "nginx_clickhouse_circuit_breaker_state",
		Help: "Circuit breaker state (0=closed, 1=open, 2=half-open)",
	})
	cbRejections = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_circuit_breaker_rejections_total",
		Help: "Total flushes rejected by circuit breaker",
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

	// Create buffer based on config.
	var buf buffer.Buffer
	switch cfg.Settings.Buffer.Type {
	case "disk":
		diskBuf, err := buffer.NewDiskBuffer(cfg.Settings.Buffer.DiskPath, cfg.Settings.Buffer.MaxDiskBytes)
		if err != nil {
			logrus.WithError(err).Fatal("can't create disk buffer")
		}
		buf = diskBuf
		logrus.WithField("path", cfg.Settings.Buffer.DiskPath).Info("using disk buffer")
	default:
		buf = buffer.NewMemoryBuffer(cfg.Settings.MaxBufferSize)
		logrus.Info("using memory buffer")
	}

	// Replay any lines from a previous session (crash recovery).
	if recovered, err := buf.Replay(); err != nil {
		logrus.WithError(err).Error("buffer replay failed")
	} else if len(recovered) > 0 {
		logrus.WithField("lines", len(recovered)).Info("replaying recovered log lines")
		entries := nginx.ParseLogs(parser, recovered)
		if err := client.Save(entries); err != nil {
			logrus.WithError(err).Error("failed to save recovered lines")
		}
	}

	// Create circuit breaker if enabled.
	var cb *circuitbreaker.CircuitBreaker
	if cfg.Settings.CircuitBreaker.Enabled {
		threshold := cfg.Settings.CircuitBreaker.Threshold
		if threshold == 0 {
			threshold = 5
		}
		cooldown := time.Duration(cfg.Settings.CircuitBreaker.CooldownSecs) * time.Second
		if cooldown == 0 {
			cooldown = 60 * time.Second
		}
		cb = circuitbreaker.New(threshold, cooldown)
		logrus.WithFields(logrus.Fields{
			"threshold": threshold,
			"cooldown":  cooldown,
		}).Info("circuit breaker enabled")
	}

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

	go flushLoop(cfg, buf, parser, client, cb)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logrus.WithField("signal", sig.String()).Info("received shutdown signal")

		// Flush remaining buffer.
		flush(buf, parser, client, cb)

		// Close ClickHouse connection.
		client.Close()

		// Close disk buffer if applicable.
		if closer, ok := buf.(io.Closer); ok {
			closer.Close()
		}

		logrus.Info("shutdown complete")
		os.Exit(0)
	}()

	for line := range t.Lines() {
		linesRead.Inc()
		line := strings.TrimSpace(line.String())
		if err := buf.Write(line); err != nil {
			logrus.WithError(err).Warn("buffer write failed, line dropped")
			linesNotProcessed.Inc()
			continue
		}
		bufferSize.Set(float64(buf.Len()))
		if buf.Len() >= cfg.Settings.MaxBufferSize {
			flush(buf, parser, client, cb)
		}
	}
}

// flushLoop periodically flushes buffered log lines to ClickHouse.
func flushLoop(cfg *configParser.Config, buf buffer.Buffer, parser *nginx.Parser, client *clickhouse.Client, cb *circuitbreaker.CircuitBreaker) {
	interval := time.Duration(cfg.Settings.Interval) * time.Second
	for {
		time.Sleep(interval)
		flush(buf, parser, client, cb)
	}
}

// flush drains the log buffer and saves entries to ClickHouse.
func flush(buf buffer.Buffer, parser *nginx.Parser, client *clickhouse.Client, cb *circuitbreaker.CircuitBreaker) {
	lines, err := buf.ReadAll()
	if err != nil {
		logrus.WithError(err).Error("buffer read failed")
		return
	}
	if len(lines) == 0 {
		return
	}
	bufferSize.Set(0)

	// Circuit breaker check.
	if cb != nil && !cb.Allow() {
		logrus.Warn("circuit breaker open, skipping flush")
		cbRejections.Inc()
		linesNotProcessed.Add(float64(len(lines)))
		return
	}

	start := time.Now()

	logrus.WithField("entries", len(lines)).Info("preparing to save log entries")

	entries := nginx.ParseLogs(parser, lines)

	parseErrs := float64(len(lines) - len(entries))
	if parseErrs > 0 {
		parseErrors.Add(parseErrs)
	}
	batchSize.Observe(float64(len(entries)))

	if err := client.Save(entries); err != nil {
		logrus.WithError(err).Error("can't save logs")
		linesNotProcessed.Add(float64(len(lines)))
		clickhouseUp.Set(0)
		if cb != nil {
			cb.RecordFailure()
			updateCBState(cb)
		}
	} else {
		logrus.WithField("entries", len(lines)).Info("saved log entries")
		linesProcessed.Add(float64(len(lines)))
		clickhouseUp.Set(1)
		if cb != nil {
			cb.RecordSuccess()
			updateCBState(cb)
		}
	}

	flushDuration.Observe(time.Since(start).Seconds())
}

// updateCBState sets the circuit breaker Prometheus gauge based on the
// current state.
func updateCBState(cb *circuitbreaker.CircuitBreaker) {
	switch cb.State() {
	case circuitbreaker.StateClosed:
		cbState.Set(0)
	case circuitbreaker.StateOpen:
		cbState.Set(1)
	case circuitbreaker.StateHalfOpen:
		cbState.Set(2)
	}
}
