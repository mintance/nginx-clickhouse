// Package config handles reading and parsing application configuration from
// YAML files and environment variables.
package config

import (
	"flag"
	"os"
	"strconv"
	"strings"

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
	Interval       int                  `yaml:"interval"`
	LogPath        string               `yaml:"log_path"`
	SeekFromEnd    bool                 `yaml:"seek_from_end"`
	MaxBufferSize  int                  `yaml:"max_buffer_size"`
	Retry          RetryConfig          `yaml:"retry"`
	Buffer         BufferConfig         `yaml:"buffer"`
	CircuitBreaker CircuitBreakerConfig `yaml:"circuit_breaker"`
	Enrichments    EnrichmentConfig     `yaml:"enrichments"`
	Filters        []FilterRule         `yaml:"filters"`
}

// BufferConfig holds buffering settings.
type BufferConfig struct {
	Type         string `yaml:"type"`           // "memory" (default) or "disk"
	DiskPath     string `yaml:"disk_path"`      // directory for disk buffer segments
	MaxDiskBytes int64  `yaml:"max_disk_bytes"` // max disk usage in bytes
}

// EnrichmentConfig holds static fields added to every log entry.
// Fields are resolved at startup and injected into each ClickHouse row
// when a column maps to a special "_" prefixed source name (e.g. _hostname).
type EnrichmentConfig struct {
	Hostname    string            `yaml:"hostname"`    // "auto" = os.Hostname(), or literal value
	Environment string            `yaml:"environment"` // e.g. "production", "staging"
	Service     string            `yaml:"service"`     // service name tag
	Extra       map[string]string `yaml:"extra"`       // arbitrary key-value pairs
}

// FilterRule defines a single filter/sampling rule evaluated against parsed log entries.
type FilterRule struct {
	Expr       string  `yaml:"expr"`        // expr-lang expression, must return bool
	Action     string  `yaml:"action"`      // "drop" (discard matches) or "keep" (retain only matches)
	SampleRate float64 `yaml:"sample_rate"` // 0 = no sampling; 0.1 = keep 10% of matches
}

// CircuitBreakerConfig holds circuit breaker settings.
type CircuitBreakerConfig struct {
	Enabled      bool `yaml:"enabled"`
	Threshold    int  `yaml:"threshold"`     // consecutive failures to open
	CooldownSecs int  `yaml:"cooldown_secs"` // seconds before half-open probe
}

// ClickHouseConfig holds ClickHouse connection and schema settings.
type ClickHouseConfig struct {
	DB                    string            `yaml:"db"`
	Table                 string            `yaml:"table"`
	Host                  string            `yaml:"host"`
	Port                  string            `yaml:"port"`
	TLS                   bool              `yaml:"tls"`
	TLSInsecureSkipVerify bool              `yaml:"tls_insecure_skip_verify"`
	CACert                string            `yaml:"ca_cert"`
	TLSCertPath           string            `yaml:"tls_cert_path"`
	TLSKeyPath            string            `yaml:"tls_key_path"`
	UseServerSideBatching bool              `yaml:"use_server_side_batching"`
	Columns               map[string]string `yaml:"columns"`
	Credentials           CredentialsConfig `yaml:"credentials"`
}

// CredentialsConfig holds authentication credentials for ClickHouse.
type CredentialsConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// NginxConfig holds NGINX log format settings.
type NginxConfig struct {
	LogType       string `yaml:"log_type"`
	LogFormat     string `yaml:"log_format"`
	LogFormatType string `yaml:"log_format_type"` // "text" (default) or "json"
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

	logrus.WithField("path", configPath).Info("reading config file")

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
		} else {
			c.Settings.Interval = interval
		}
	}

	if v := os.Getenv("MAX_BUFFER_SIZE"); v != "" {
		size, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid MAX_BUFFER_SIZE %q: %v", v, err)
		} else {
			c.Settings.MaxBufferSize = size
		}
	}

	if v := os.Getenv("RETRY_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_MAX %q: %v", v, err)
		} else {
			c.Settings.Retry.MaxRetries = n
		}
	}

	if v := os.Getenv("RETRY_BACKOFF_INITIAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_BACKOFF_INITIAL %q: %v", v, err)
		} else {
			c.Settings.Retry.BackoffInitialSecs = n
		}
	}

	if v := os.Getenv("RETRY_BACKOFF_MAX"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid RETRY_BACKOFF_MAX %q: %v", v, err)
		} else {
			c.Settings.Retry.BackoffMaxSecs = n
		}
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
	if v := os.Getenv("CLICKHOUSE_TLS"); v != "" {
		c.ClickHouse.TLS = v == "true"
	}
	if v := os.Getenv("CLICKHOUSE_TLS_SKIP_VERIFY"); v != "" {
		c.ClickHouse.TLSInsecureSkipVerify = v == "true"
	}
	if v := os.Getenv("CLICKHOUSE_CA_CERT"); v != "" {
		c.ClickHouse.CACert = v
	}
	if v := os.Getenv("CLICKHOUSE_TLS_CERT_PATH"); v != "" {
		c.ClickHouse.TLSCertPath = v
	}
	if v := os.Getenv("CLICKHOUSE_TLS_KEY_PATH"); v != "" {
		c.ClickHouse.TLSKeyPath = v
	}
	if v := os.Getenv("CLICKHOUSE_USE_SERVER_SIDE_BATCHING"); v != "" {
		c.ClickHouse.UseServerSideBatching = v == "true"
	}

	if v := os.Getenv("NGINX_LOG_TYPE"); v != "" {
		c.Nginx.LogType = v
	}
	if v := os.Getenv("NGINX_LOG_FORMAT"); v != "" {
		c.Nginx.LogFormat = v
	}
	if v := os.Getenv("NGINX_LOG_FORMAT_TYPE"); v != "" {
		c.Nginx.LogFormatType = v
	}

	if v := os.Getenv("BUFFER_TYPE"); v != "" {
		c.Settings.Buffer.Type = v
	}
	if v := os.Getenv("BUFFER_DISK_PATH"); v != "" {
		c.Settings.Buffer.DiskPath = v
	}
	if v := os.Getenv("BUFFER_MAX_DISK_BYTES"); v != "" {
		maxBytes, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			logrus.Errorf("invalid BUFFER_MAX_DISK_BYTES %q: %v", v, err)
		} else {
			c.Settings.Buffer.MaxDiskBytes = maxBytes
		}
	}

	if v := os.Getenv("CIRCUIT_BREAKER_ENABLED"); v != "" {
		c.Settings.CircuitBreaker.Enabled = v == "true"
	}
	if v := os.Getenv("CIRCUIT_BREAKER_THRESHOLD"); v != "" {
		threshold, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid CIRCUIT_BREAKER_THRESHOLD %q: %v", v, err)
		} else {
			c.Settings.CircuitBreaker.Threshold = threshold
		}
	}
	if v := os.Getenv("CIRCUIT_BREAKER_COOLDOWN"); v != "" {
		cooldown, err := strconv.Atoi(v)
		if err != nil {
			logrus.Errorf("invalid CIRCUIT_BREAKER_COOLDOWN %q: %v", v, err)
		} else {
			c.Settings.CircuitBreaker.CooldownSecs = cooldown
		}
	}

	if v := os.Getenv("ENRICHMENT_HOSTNAME"); v != "" {
		c.Settings.Enrichments.Hostname = v
	}
	if v := os.Getenv("ENRICHMENT_ENVIRONMENT"); v != "" {
		c.Settings.Enrichments.Environment = v
	}
	if v := os.Getenv("ENRICHMENT_SERVICE"); v != "" {
		c.Settings.Enrichments.Service = v
	}

	// Any ENRICHMENT_ env var not matching a known field goes into the extra map.
	known := map[string]bool{"ENRICHMENT_HOSTNAME": true, "ENRICHMENT_ENVIRONMENT": true, "ENRICHMENT_SERVICE": true}
	if c.Settings.Enrichments.Extra == nil {
		c.Settings.Enrichments.Extra = make(map[string]string)
	}
	for _, env := range os.Environ() {
		key, val, _ := strings.Cut(env, "=")
		if after, ok := strings.CutPrefix(key, "ENRICHMENT_"); ok && !known[key] {
			c.Settings.Enrichments.Extra[strings.ToLower(after)] = val
		}
	}
}
