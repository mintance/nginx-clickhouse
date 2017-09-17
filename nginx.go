package main

import (
	"strings"
	"github.com/satyrius/gonx"
	"io"
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