package config

import (
	"flag"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

type Config struct {
	Settings struct {
		Interval    int    `yaml:"interval"`
		LogPath     string `yaml:"log_path"`
		SeekFromEnd bool   `yaml:"seek_from_end"`
	} `yaml:"settings"`
	ClickHouse struct {
		Db          string            `yaml:"db"`
		Table       string            `yaml:"table"`
		Host        string            `yaml:"host"`
		Port        string            `yaml:"port"`
		Secure      bool              `yaml:"secure"`
		SkipVerify  bool              `yaml:"skip_verify"`
		Columns     map[string]string `yaml:"columns"`
		Credentials struct {
			User     string `yaml:"user"`
			Password string `yaml:"password"`
		} `yaml:"credentials"`
	} `yaml:"clickhouse"`
	Nginx struct {
		LogType   string `yaml:"log_type"`
		LogFormat string `yaml:"log_format"`
	}
}

var configPath string

var NginxTimeLayout = "02/Jan/2006:15:04:05 -0700"
var CHTimeLayout = "2006-01-02 15:04:05"

func init() {
	flag.StringVar(&configPath, "config_path", "config/config.yml", "Config path.")
	flag.Parse()
}

func Read() *Config {

	config := &Config{}

	logrus.Info("Reading config file: " + configPath)

	var data, err = ioutil.ReadFile(configPath)

	if err != nil {
		logrus.Fatal("Config open error: ", err)
	}

	if err = yaml.Unmarshal(data, &config); err != nil {
		logrus.Fatal("Config read & unmarshal error: ", err)
	}

	config.SetEnvVariables()

	return config
}

func (c *Config) SetEnvVariables() {

	// Settings

	if os.Getenv("LOG_PATH") != "" {
		c.Settings.LogPath = os.Getenv("LOG_PATH")
	}

	if os.Getenv("FLUSH_INTERVAL") != "" {

		var flushInterval, err = strconv.Atoi(os.Getenv("FLUSH_INTERVAL"))

		if err != nil {
			logrus.Errorf("error to convert FLUSH_INTERVAL string to int: %v", err)
		}

		c.Settings.Interval = flushInterval
	}

	// ClickHouse

	if os.Getenv("CLICKHOUSE_HOST") != "" {
		c.ClickHouse.Host = os.Getenv("CLICKHOUSE_HOST")
	}

	if os.Getenv("CLICKHOUSE_PORT") != "" {
		c.ClickHouse.Port = os.Getenv("CLICKHOUSE_PORT")
	}

	if os.Getenv("CLICKHOUSE_DB") != "" {
		c.ClickHouse.Db = os.Getenv("CLICKHOUSE_DB")
	}

	if os.Getenv("CLICKHOUSE_TABLE") != "" {
		c.ClickHouse.Table = os.Getenv("CLICKHOUSE_TABLE")
	}

	if os.Getenv("CLICKHOUSE_USER") != "" {
		c.ClickHouse.Credentials.User = os.Getenv("CLICKHOUSE_USER")
	}

	if os.Getenv("CLICKHOUSE_PASSWORD") != "" {
		c.ClickHouse.Credentials.Password = os.Getenv("CLICKHOUSE_PASSWORD")
	}

	if os.Getenv("CLICKHOUSE_SECURE") != "" {
		c.ClickHouse.Secure, _ = strconv.ParseBool(os.Getenv("CLICKHOUSE_SECURE"))
	}

	if os.Getenv("CLICKHOUSE_SKIP_VERIFY") != "" {
		c.ClickHouse.Secure, _ = strconv.ParseBool(os.Getenv("CLICKHOUSE_SKIP_VERIFY"))
	}

	// Nginx

	if os.Getenv("NGINX_LOG_TYPE") != "" {
		c.Nginx.LogType = os.Getenv("NGINX_LOG_TYPE")
	}

	if os.Getenv("NGINX_LOG_FORMAT") != "" {
		c.Nginx.LogFormat = os.Getenv("NGINX_LOG_FORMAT")
	}
}
