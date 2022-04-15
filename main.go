package main

import (
	"context"
	"github.com/WinnerSoftLab/nginx-clickhouse/clickhouse"
	configParser "github.com/WinnerSoftLab/nginx-clickhouse/config"
	"github.com/WinnerSoftLab/nginx-clickhouse/nginx"
	"github.com/papertrail/go-tail/follower"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/satyrius/gonx"
	"github.com/sirupsen/logrus"
	"io"
	"net/http"
	"strings"
	"time"
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
	linesReadFailed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "nginx_clickhouse_lines_read_failed_total",
		Help: "The total number of log lines which was not readed",
	})
)

const ChanSize = 10
const BuffSize = 200000
const BuffTimeout = time.Minute * 5

func main() {
	// Read config & incoming flags
	config := configParser.Read()

	nginxParser, err := nginx.GetParser(config)
	if err != nil {
		logrus.Fatal("Can`t parse nginx log format: ", err)
	}

	storage, err := clickhouse.NewStorage(config, context.Background())
	if err != nil {
		logrus.Fatal("Can`t connect to clickhouse: ", err)
	}

	go func() {
		http.Handle("/metrics", promhttp.Handler())
		http.ListenAndServe(":2112", nil)
	}()

	ch := make(chan []string, ChanSize)

	go Writer(storage, nginxParser, ch)
	go Reader(config, ch)

	// Не стал пока реализовывать обработку сигналов
	select {}
}

func Writer(storage *clickhouse.Storage, nginxParser *gonx.Parser, ch <-chan []string) {
	for pack := range ch {
		logrus.Debugf("Preparing to save %d new log entries.", len(pack))
		parsed, err := nginx.ParseLogs(nginxParser, pack)
		if err != nil {
			logrus.Errorf("Can't parse pack: %s", err)
			linesNotProcessed.Add(float64(len(pack)))
			continue
		}
		if err := storage.Save(parsed); err != nil {
			logrus.Error("Can't save pack: ", err)
			linesNotProcessed.Add(float64(len(pack)))
		} else {
			logrus.Info("Saved ", len(pack), " new logs.")
			linesProcessed.Add(float64(len(pack)))
		}
	}
}

func Reader(config *configParser.Config, ch chan<- []string) {
	whenceSeek := io.SeekStart
	if config.Settings.SeekFromEnd {
		whenceSeek = io.SeekEnd
	}
	tail, err := follower.New(config.Settings.LogPath, follower.Config{
		Whence: whenceSeek,
		Offset: 0,
		Reopen: true,
	})
	if err != nil {
		logrus.Fatalf("Can't tail logfile: %s", err)
	}

	buff := make([]string, 0, BuffSize)
	timer := time.NewTimer(BuffTimeout)
	for {
		select {
		case line := <-tail.Lines():
			if tail.Err() != nil {
				linesReadFailed.Add(1)
				logrus.Errorf("Tail failed, error: %s", tail.Err())
			}
			buff = append(buff, strings.TrimSpace(line.String()))
			if len(buff) >= BuffSize {
				ch <- buff
				buff = make([]string, 0, BuffSize)
				timer = time.NewTimer(BuffTimeout)
			}
		case <-timer.C:
			if len(buff) > 0 {
				ch <- buff
				buff = make([]string, 0, BuffSize)
				timer = time.NewTimer(BuffTimeout)
			}
		}
	}
}
