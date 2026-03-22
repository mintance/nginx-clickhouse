// Package config handles reading and parsing application configuration from
// YAML files and environment variables.
package config

import (
	"flag"
	"os"
	"strconv"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
)

// NginxTimeLayout is the time format used in NGINX access logs.
var NginxTimeLayout = "02/Jan/2006:15:04:05 -0700"

// CHTimeLayout is the DateTime format expected by ClickHouse.
var CHTimeLayout = "2006-01-02 15:04:05"

// Config holds the full application configuration parsed from YAML.
type Config struct {
	Settings   SettingsConfig   `yaml:"settings"`
	ClickHouse ClickHouseConfig `yaml:"clickhouse"`
	Nginx      NginxConfig      `yaml:"nginx"`
}

// RetryConfig holds retry behavior settings for ClickHouse writes.
type RetryConfig struct {
	MaxRetries         int `yaml:"max_retries"`
	BackoffInitialSecs int `yaml:"backoff_initial_secs"`
	BackoffMaxSecs     int `yaml:"backoff_max_secs"`
}

// SettingsConfig holds general application settings.
type SettingsConfig struct {
	Interval      int         `yaml:"interval"`
	LogPath       string      `yaml:"log_path"`
	SeekFromEnd   bool        `yaml:"seek_from_end"`
	MaxBufferSize int         `yaml:"max_buffer_size"`
	Retry         RetryConfig `yaml:"retry"`
}

// ClickHouseConfig holds ClickHouse connection and schema settings.
type ClickHouseConfig struct {
	DB          string            `yaml:"db"`
	Table       string            `yaml:"table"`
	Host        string            `yaml:"host"`
	Port        string            `yaml:"port"`
	Columns     map[string]string `yaml:"columns"`
	Credentials CredentialsConfig `yaml:"credentials"`
}

// CredentialsConfig holds authentication credentials for ClickHouse.
type CredentialsConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// NginxConfig holds NGINX log format settings.
type NginxConfig struct {
	LogType   string `yaml:"log_type"`
	LogFormat string `yaml:"log_format"`
}

var configPath string

func init() {
	flag.StringVar(&configPath, "config_path", "config/config.yml", "Config path.")
}

// Read loads the configuration from the YAML file specified by the -config_path flag.
func Read() *Config {
	if !flag.Parsed() {
		flag.Parse()
	}

	logrus.Info("reading config file: ", configPath)

	data, err := os.ReadFile(configPath)
	if err != nil {
		logrus.Fatal("config open error: ", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		logrus.Fatal("config unmarshal error: ", err)
	}

	return &cfg
}

// SetEnvVariables overrides configuration values with environment variables
// when they are set. Each supported environment variable maps to a specific
// configuration field.
func (c *Config) SetEnvVariables() {
	if v := os.Getenv("LOG_PATH"); v != "" {
		c.Settings.LogPath = v
	}

	if v := os.Getenv("FLUSH_INTERVAL"); v != "" {
		interval, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid FLUSH_INTERVAL %q: %v", v, err)
		}
		c.Settings.Interval = interval
	}

	if v := os.Getenv("MAX_BUFFER_SIZE"); v != "" {
		size, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid MAX_BUFFER_SIZE %q: %v", v, err)
		}
		c.Settings.MaxBufferSize = size
	}

	if v := os.Getenv("RETRY_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_MAX %q: %v", v, err)
		}
		c.Settings.Retry.MaxRetries = n
	}

	if v := os.Getenv("RETRY_BACKOFF_INITIAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_BACKOFF_INITIAL %q: %v", v, err)
		}
		c.Settings.Retry.BackoffInitialSecs = n
	}

	if v := os.Getenv("RETRY_BACKOFF_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_BACKOFF_MAX %q: %v", v, err)
		}
		c.Settings.Retry.BackoffMaxSecs = n
	}

	if v := os.Getenv("CLICKHOUSE_HOST"); v != "" {
		c.ClickHouse.Host = v
	}
	if v := os.Getenv("CLICKHOUSE_PORT"); v != "" {
		c.ClickHouse.Port = v
	}
	if v := os.Getenv("CLICKHOUSE_DB"); v != "" {
		c.ClickHouse.DB = v
	}
	if v := os.Getenv("CLICKHOUSE_TABLE"); v != "" {
		c.ClickHouse.Table = v
	}
	if v := os.Getenv("CLICKHOUSE_USER"); v != "" {
		c.ClickHouse.Credentials.User = v
	}
	if v := os.Getenv("CLICKHOUSE_PASSWORD"); v != "" {
		c.ClickHouse.Credentials.Password = v
	}

	if v := os.Getenv("NGINX_LOG_TYPE"); v != "" {
		c.Nginx.LogType = v
	}
	if v := os.Getenv("NGINX_LOG_FORMAT"); v != "" {
		c.Nginx.LogFormat = v
	}
}
