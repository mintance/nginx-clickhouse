// Command nginx-clickhouse tails NGINX access logs and batch-inserts parsed
// entries into ClickHouse.
package main

import (
	"bufio"
	"flag"
	"fmt"
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

	"github.com/mintance/nginx-clickhouse/buffer"
	"github.com/mintance/nginx-clickhouse/circuitbreaker"
	"github.com/mintance/nginx-clickhouse/clickhouse"
	configParser "github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/filter"
	"github.com/mintance/nginx-clickhouse/nginx"
)

const defaultMaxBufferSize = 10000

var (
	checkMode bool
	onceMode  bool
	stdinMode bool
)

func init() {
	flag.BoolVar(&checkMode, "check", false, "Validate config and ClickHouse connectivity, then exit.")
	flag.BoolVar(&onceMode, "once", false, "Read the log file from start to end, flush, and exit.")
	flag.BoolVar(&stdinMode, "stdin", false, "Read log lines from stdin instead of a file.")
}

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

	linesFiltered = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_filtered_total",
		Help: "Total log entries dropped by filter rules",
	})
)

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})

	cfg := configParser.Read()
	cfg.SetEnvVariables()

	// Resolve "auto" hostname enrichment.
	if cfg.Settings.Enrichments.Hostname == "auto" {
		h, err := os.Hostname()
		if err != nil {
			logrus.WithError(err).Warn("failed to resolve hostname for enrichment")
		} else {
			cfg.Settings.Enrichments.Hostname = h
		}
	}

	if cfg.Settings.MaxBufferSize == 0 {
		cfg.Settings.MaxBufferSize = defaultMaxBufferSize
	}

	var parser *nginx.Parser
	if cfg.Nginx.LogFormatType != "json" {
		var err error
		parser, err = nginx.NewParser(cfg)
		if err != nil {
			logrus.Fatal("can't parse nginx log format: ", err)
		}
	}

	// Compile filter rules.
	var filterChain *filter.Chain
	if len(cfg.Settings.Filters) > 0 {
		// Collect field names from column mappings (excluding enrichment fields).
		var fields []string
		for _, source := range cfg.ClickHouse.Columns {
			if !strings.HasPrefix(source, "_") {
				fields = append(fields, source)
			}
		}

		var err error
		filterChain, err = filter.NewChain(cfg.Settings.Filters, fields)
		if err != nil {
			logrus.Fatal("invalid filter rules: ", err)
		}
		logrus.WithField("rules", len(cfg.Settings.Filters)).Info("filter rules compiled")
	}

	if cfg.ClickHouse.UseServerSideBatching {
		logrus.Info("server-side batching enabled (async_insert=1, wait_for_async_insert=1)")
		if cfg.Settings.Buffer.Type == "disk" {
			logrus.Warn("disk buffer is redundant with server-side batching; ClickHouse handles durability via async inserts")
		}
	}

	client := clickhouse.NewClient(cfg)

	if checkMode {
		runCheck(cfg, parser, client)
		return
	}

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
		var entries []nginx.LogEntry
		if parser != nil {
			entries = nginx.ParseLogs(parser, recovered)
		} else {
			entries = nginx.ParseJSONLogs(recovered)
		}
		if filterChain != nil {
			entries = filterChain.Apply(entries)
		}
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

	// Determine the line source based on flags.
	// Priority: -stdin > -once > tail (continuous).
	lines := openLineSource(cfg)

	go flushLoop(cfg, buf, parser, client, cb, filterChain)

	// Guard against concurrent shutdown from signal + EOF.
	var shutdownOnce sync.Once
	doShutdown := func() {
		shutdownOnce.Do(func() {
			flush(buf, parser, client, cb, filterChain)
			client.Close()
			if closer, ok := buf.(io.Closer); ok {
				closer.Close()
			}
			logrus.Info("shutdown complete")
			os.Exit(0)
		})
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		sig := <-sigCh
		logrus.WithField("signal", sig.String()).Info("received shutdown signal")
		doShutdown()
	}()

	for line := range lines {
		linesRead.Inc()
		line = strings.TrimSpace(line)
		if err := buf.Write(line); err != nil {
			logrus.WithError(err).Warn("buffer write failed, line dropped")
			linesNotProcessed.Inc()
			continue
		}
		bufferSize.Set(float64(buf.Len()))
		if buf.Len() >= cfg.Settings.MaxBufferSize {
			flush(buf, parser, client, cb, filterChain)
		}
	}

	// Line source closed (EOF in -once or -stdin mode).
	// In tail mode this is unreachable since follower never closes.
	doShutdown()
}

// openLineSource returns a channel of log lines based on the active input mode.
func openLineSource(cfg *configParser.Config) <-chan string {
	if stdinMode {
		if onceMode {
			logrus.Warn("-stdin and -once both set; using -stdin")
		}
		logrus.Info("reading from stdin")
		return scanLines(os.Stdin)
	}

	if onceMode {
		logrus.WithField("path", cfg.Settings.LogPath).Info("reading file once")
		f, err := os.Open(cfg.Settings.LogPath)
		if err != nil {
			logrus.Fatal("can't open log file: ", err)
		}
		return scanLines(f)
	}

	// Default: tail mode (continuous).
	logrus.WithField("path", cfg.Settings.LogPath).Info("tailing log file")

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

	ch := make(chan string)
	go func() {
		defer close(ch)
		for line := range t.Lines() {
			ch <- line.String()
		}
	}()
	return ch
}

// scanLines reads lines from r and sends them on the returned channel.
// The channel is closed when r reaches EOF or encounters an error.
func scanLines(r io.Reader) <-chan string {
	ch := make(chan string)
	go func() {
		defer close(ch)
		if closer, ok := r.(io.Closer); ok && r != os.Stdin {
			defer closer.Close()
		}
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // max 1MB per line
		for scanner.Scan() {
			ch <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			logrus.WithError(err).Error("error reading input")
		}
	}()
	return ch
}

// flushLoop periodically flushes buffered log lines to ClickHouse.
func flushLoop(cfg *configParser.Config, buf buffer.Buffer, parser *nginx.Parser, client *clickhouse.Client, cb *circuitbreaker.CircuitBreaker, fc *filter.Chain) {
	interval := time.Duration(cfg.Settings.Interval) * time.Second
	for {
		time.Sleep(interval)
		flush(buf, parser, client, cb, fc)
	}
}

// flush drains the log buffer and saves entries to ClickHouse.
func flush(buf buffer.Buffer, parser *nginx.Parser, client *clickhouse.Client, cb *circuitbreaker.CircuitBreaker, fc *filter.Chain) {
	// Circuit breaker check BEFORE draining the buffer to avoid data loss.
	if cb != nil && !cb.Allow() {
		logrus.Warn("circuit breaker open, skipping flush")
		cbRejections.Inc()
		return
	}

	lines, err := buf.ReadAll()
	if err != nil {
		logrus.WithError(err).Error("buffer read failed")
		return
	}
	if len(lines) == 0 {
		return
	}
	bufferSize.Set(0)

	start := time.Now()

	logrus.WithField("entries", len(lines)).Info("preparing to save log entries")

	var entries []nginx.LogEntry
	if parser != nil {
		entries = nginx.ParseLogs(parser, lines)
	} else {
		entries = nginx.ParseJSONLogs(lines)
	}

	parseErrs := float64(len(lines) - len(entries))
	if parseErrs > 0 {
		parseErrors.Add(parseErrs)
	}

	if fc != nil {
		before := len(entries)
		entries = fc.Apply(entries)
		if dropped := before - len(entries); dropped > 0 {
			linesFiltered.Add(float64(dropped))
			logrus.WithFields(logrus.Fields{
				"before": before,
				"after":  len(entries),
			}).Debug("filter applied")
		}
	}

	batchSize.Observe(float64(len(entries)))

	if len(entries) == 0 {
		logrus.Debug("all entries filtered, skipping save")
		return
	}

	if err := client.Save(entries); err != nil {
		logrus.WithError(err).Error("can't save logs")
		linesNotProcessed.Add(float64(len(lines)))
		clickhouseUp.Set(0)
		if cb != nil {
			cb.RecordFailure()
			updateCBState(cb)
		}

		// Write lines back into the buffer so they can be retried next flush.
		for _, line := range lines {
			if writeErr := buf.Write(line); writeErr != nil {
				logrus.WithError(writeErr).Warn("failed to re-buffer line after save failure")
			}
		}
		bufferSize.Set(float64(buf.Len()))
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

// runCheck validates the configuration and ClickHouse connectivity, then exits.
func runCheck(cfg *configParser.Config, parser *nginx.Parser, client *clickhouse.Client) {
	allOK := true

	// Config loaded
	fmt.Println("✓ Config loaded")

	// Log format
	if cfg.Nginx.LogFormatType == "json" {
		fmt.Println("✓ Log format: JSON")
	} else if parser != nil {
		fmt.Println("✓ Log format: text (parseable)")
	} else {
		fmt.Println("✗ Log format: text parser failed to initialize")
		allOK = false
	}

	// Filters
	if len(cfg.Settings.Filters) > 0 {
		var fields []string
		for _, source := range cfg.ClickHouse.Columns {
			if !strings.HasPrefix(source, "_") {
				fields = append(fields, source)
			}
		}
		_, err := filter.NewChain(cfg.Settings.Filters, fields)
		if err != nil {
			fmt.Printf("✗ Filters: %v\n", err)
			allOK = false
		} else {
			fmt.Printf("✓ Filters: %d rules compiled\n", len(cfg.Settings.Filters))
		}
	} else {
		fmt.Println("· Filters: none configured")
	}

	// Log file
	if _, err := os.Stat(cfg.Settings.LogPath); err != nil {
		fmt.Printf("✗ Log file: %s (%v)\n", cfg.Settings.LogPath, err)
		allOK = false
	} else {
		fmt.Printf("✓ Log file: %s\n", cfg.Settings.LogPath)
	}

	// ClickHouse checks
	results := client.Check()
	for _, r := range results {
		if r.OK {
			fmt.Printf("✓ %s: %s\n", r.Name, r.Message)
		} else {
			fmt.Printf("✗ %s: %s\n", r.Name, r.Message)
			allOK = false
		}
	}

	client.Close()

	// Config warnings (non-fatal).
	if cfg.ClickHouse.UseServerSideBatching && cfg.Settings.Buffer.Type == "disk" {
		fmt.Println("WARNING: disk buffer is redundant with server-side batching")
	}

	if allOK {
		fmt.Println("\nAll checks passed.")
	} else {
		fmt.Println("\nSome checks failed.")
		os.Exit(1)
	}
}
