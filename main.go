package main

import (
	"github.com/Sirupsen/logrus"
	"sync"
	"github.com/hpcloud/tail"
	"time"
)

var (
	locker sync.Mutex
	logs []string
)

func main() {

	// Read config & incoming flags
	config := readConfig()

	// Update config with environment variables if exist
	config.setEnvVariables()

	nginxParser, err := getParser(config)

	if err != nil {
		logrus.Fatal("Can`t parse nginx log format: ", err)
	}

	logs = []string{}

	t, err := tail.TailFile(config.Settings.LogPath, tail.Config{ Follow: true, ReOpen: true, MustExist: true })

	if err != nil {
		logrus.Fatal("Can`t tail logfile: ", err)
	}

	logrus.Info("Opening logfile: " + config.Settings.LogPath)

	go func() {
		for {

			time.Sleep(time.Second * time.Duration(config.Settings.Interval))

			if len(logs) > 0 {

				logrus.Info("Preparing to save ", len(logs), " new log entries.")

				locker.Lock()

				err := save(config, parseLogs(nginxParser, logs))

				if err != nil {
					logrus.Error("Can`t save logs: ", err)
				} else {

					logrus.Info("Saved ", len(logs), " new logs.")

					logs = []string{}
				}

				locker.Unlock()
			}
		}
	}()

	// Push new log entries to array
	for line := range t.Lines {

		locker.Lock()

		logs = append(logs, line.Text)

		locker.Unlock()

	}
}
