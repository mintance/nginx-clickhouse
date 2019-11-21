package main

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/mintance/nginx-clickhouse/clickhouse"
	configParser "github.com/mintance/nginx-clickhouse/config"
	"github.com/mintance/nginx-clickhouse/nginx"
	"github.com/papertrail/go-tail/follower"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	locker sync.Mutex
	logs   []string
)

var (
	linesProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_processed_total",
		Help: "The total number of processed log lines",
	})
	linesNotProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_not_processed_total",
		Help: "The total number of log lines which was not processed",
	})
)

func main() {

	// Read config & incoming flags
	config := configParser.Read()

	// Update config with environment variables if exist
	config.SetEnvVariables()

	nginxParser, err := nginx.GetParser(config)

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2112", nil)
	}()

	if err != nil {
		logrus.Fatal("Can`t parse nginx log format: ", err)
	}

	logs = []string{}

	logrus.Info("Trying to open logfile: " + config.Settings.LogPath)

	whenceSeek := io.SeekStart
	if config.Settings.SeekFromEnd {
		whenceSeek = io.SeekEnd
	}

	t, err := follower.New(config.Settings.LogPath, follower.Config{
		Whence: whenceSeek,
		Offset: 0,
		Reopen: true,
	})

	if err != nil {
		logrus.Fatal("Can`t tail logfile: ", err)
	}

	go func() {
		for {
			time.Sleep(time.Second * time.Duration(config.Settings.Interval))

			if len(logs) > 0 {

				logrus.Info("Preparing to save ", len(logs), " new log entries.")
				locker.Lock()
				err := clickhouse.Save(config, nginx.ParseLogs(nginxParser, logs))

				if err != nil {
					logrus.Error("Can`t save logs: ", err)
					linesNotProcessed.Add(float64(len(logs)))
				} else {
					logrus.Info("Saved ", len(logs), " new logs.")
					linesProcessed.Add(float64(len(logs)))
				}

				logs = []string{}
				locker.Unlock()
			}
		}
	}()

	// Push new log entries to array
	for line := range t.Lines() {
		locker.Lock()
		logs = append(logs, strings.TrimSpace(line.String()))
		locker.Unlock()
	}
}
