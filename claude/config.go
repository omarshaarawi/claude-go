package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/omarshaarawi/claude-go/claude/internal/ratelimit"
)

// ConfigSource represents where a configuration value came from
type ConfigSource string

const (
	// ConfigSourceDefault indicates the value is a default
	ConfigSourceDefault ConfigSource = "default"
	// ConfigSourceEnv indicates the value came from an environment variable
	ConfigSourceEnv ConfigSource = "env"
	// ConfigSourceFile indicates the value came from a config file
	ConfigSourceFile ConfigSource = "file"
)

// Config holds all configuration for the Claude client
type Config struct {
	// API Configuration
	APIKey     string `json:"api_key" env:"CLAUDE_API_KEY"`
	BaseURL    string `json:"base_url" env:"CLAUDE_BASE_URL"`
	APIVersion string `json:"api_version" env:"CLAUDE_API_VERSION"`

	// HTTP Client Configuration
	Timeout            time.Duration `json:"timeout" env:"CLAUDE_TIMEOUT"`
	MaxRetries         int           `json:"max_retries" env:"CLAUDE_MAX_RETRIES"`
	RetryWaitMin       time.Duration `json:"retry_wait_min" env:"CLAUDE_RETRY_WAIT_MIN"`
	RetryWaitMax       time.Duration `json:"retry_wait_max" env:"CLAUDE_RETRY_WAIT_MAX"`
	RetryableHTTPCodes []int         `json:"retryable_http_codes" env:"CLAUDE_RETRYABLE_HTTP_CODES"`

	// Rate Limiting Configuration
	EnableRateLimiting bool                                 `json:"enable_rate_limiting" env:"CLAUDE_ENABLE_RATE_LIMITING"`
	CustomRateLimits   map[string]ratelimit.RateLimitConfig `json:"custom_rate_limits"`

	// Logging Configuration
	LogLevel        string `json:"log_level" env:"CLAUDE_LOG_LEVEL"`
	LogFormat       string `json:"log_format" env:"CLAUDE_LOG_FORMAT"`
	LogOutput       string `json:"log_output" env:"CLAUDE_LOG_OUTPUT"`
	EnableDebugLogs bool   `json:"enable_debug_logs" env:"CLAUDE_ENABLE_DEBUG_LOGS"`

	// Sources tracks where each config value came from
	sources map[string]ConfigSource `json:"-"`
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		BaseURL:    defaultBaseURL,
		APIVersion: apiVersion,

		Timeout:            defaultTimeout,
		MaxRetries:         3,
		RetryWaitMin:       1 * time.Second,
		RetryWaitMax:       30 * time.Second,
		RetryableHTTPCodes: []int{408, 429, 500, 502, 503, 504},

		EnableRateLimiting: true,
		CustomRateLimits:   make(map[string]ratelimit.RateLimitConfig),

		LogLevel:  "info",
		LogFormat: "json",
		LogOutput: "stdout",

		sources: make(map[string]ConfigSource),
	}
}

// LoadConfig loads configuration from all available sources
func LoadConfig() (*Config, error) {
	config := DefaultConfig()

	config.markAllFieldsSource(ConfigSourceDefault)

	if err := loadDotEnv(); err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	if err := config.loadFromEnv(); err != nil {
		return nil, fmt.Errorf("error loading from environment: %w", err)
	}

	if err := config.loadFromFile(); err != nil {
		return nil, fmt.Errorf("error loading from config file: %w", err)
	}

	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// loadDotEnv attempts to load the .env file from common locations
func loadDotEnv() error {
	locations := []string{
		".env",
		"../.env",
		os.Getenv("HOME") + "/.config/claude/config.env",
	}

	for _, loc := range locations {
		if _, err := os.Stat(loc); err == nil {
			return godotenv.Load(loc)
		}
	}

	return nil
}

// ConfigFileLocation returns the default config file location for the current OS
func ConfigFileLocation() string {
	switch runtime.GOOS {
	case "windows":
		return filepath.Join(os.Getenv("APPDATA"), "claude", "config.json")
	case "darwin":
		return filepath.Join(os.Getenv("HOME"), "Library", "Application Support", "claude", "config.json")
	default: // linux and others
		if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
			return filepath.Join(xdgConfig, "claude", "config.json")
		}
		return filepath.Join(os.Getenv("HOME"), ".config", "claude", "config.json")
	}
}

// loadFromFile loads configuration from the config file
func (c *Config) loadFromFile() error {
	configFile := ConfigFileLocation()
	if _, err := os.Stat(configFile); os.IsNotExist(err) {
		return nil
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return fmt.Errorf("error reading config file: %w", err)
	}

	var fileConfig Config
	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return fmt.Errorf("error parsing config file: %w", err)
	}

	c.mergeFrom(&fileConfig, ConfigSourceFile)
	return nil
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() error {
	return loadEnvVars(c, ConfigSourceEnv)
}

// SaveToFile saves the current configuration to the config file
func (c *Config) SaveToFile() error {
	configFile := ConfigFileLocation()

	if err := os.MkdirAll(filepath.Dir(configFile), 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshaling config: %w", err)
	}

	if err := os.WriteFile(configFile, data, 0600); err != nil {
		return fmt.Errorf("error writing config file: %w", err)
	}

	return nil
}

// validate checks if the configuration is valid
func (c *Config) validate() error {
	if c.APIKey == "" {
		return fmt.Errorf("API key is required")
	}

	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be non-negative")
	}

	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries must be non-negative")
	}

	return nil
}

// mergeFrom merges another config into this one, tracking the source
func (c *Config) mergeFrom(other *Config, source ConfigSource) {
	// Use reflection to merge fields and track sources
	mergeConfigs(c, other, source)
}

// markAllFieldsSource marks all fields as coming from the given source
func (c *Config) markAllFieldsSource(source ConfigSource) {
	markFieldsSource(c, source)
}

// GetFieldSource returns the source of a configuration field
func (c *Config) GetFieldSource(field string) ConfigSource {
	if source, ok := c.sources[field]; ok {
		return source
	}
	return ConfigSourceDefault
}

// loadEnvVars loads environment variables into the config struct
func loadEnvVars(config *Config, source ConfigSource) error {
	v := reflect.ValueOf(config).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		envTag := field.Tag.Get("env")
		if envTag == "" {
			continue
		}

		envVal := os.Getenv(envTag)
		if envVal == "" {
			continue
		}

		fieldValue := v.Field(i)
		if err := setFieldFromString(fieldValue, envVal); err != nil {
			return fmt.Errorf("error setting %s: %w", field.Name, err)
		}

		config.sources[field.Name] = source
	}

	return nil
}

// setFieldFromString sets a reflect.Value from a string based on its type
func setFieldFromString(v reflect.Value, str string) error {
	switch v.Kind() {
	case reflect.String:
		v.SetString(str)
	case reflect.Bool:
		b, err := strconv.ParseBool(str)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			d, err := time.ParseDuration(str)
			if err != nil {
				return err
			}
			v.Set(reflect.ValueOf(d))
		} else {
			i, err := strconv.ParseInt(str, 10, 64)
			if err != nil {
				return err
			}
			v.SetInt(i)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		i, err := strconv.ParseUint(str, 10, 64)
		if err != nil {
			return err
		}
		v.SetUint(i)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(str, 64)
		if err != nil {
			return err
		}
		v.SetFloat(f)
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Int {
			parts := strings.Split(str, ",")
			slice := reflect.MakeSlice(v.Type(), len(parts), len(parts))
			for i, part := range parts {
				n, err := strconv.Atoi(strings.TrimSpace(part))
				if err != nil {
					return err
				}
				slice.Index(i).SetInt(int64(n))
			}
			v.Set(slice)
		}
	default:
		return fmt.Errorf("unsupported type: %s", v.Type())
	}
	return nil
}

// mergeConfigs merges two configs using reflection
func mergeConfigs(dest, src *Config, source ConfigSource) {
	vDest := reflect.ValueOf(dest).Elem()
	vSrc := reflect.ValueOf(src).Elem()
	t := vDest.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("json") == "-" {
			continue
		}

		destField := vDest.Field(i)
		srcField := vSrc.Field(i)

		if isZeroValue(srcField) {
			continue
		}

		if destField.Kind() == reflect.Map {
			if srcField.Len() > 0 {
				if destField.IsNil() {
					destField.Set(reflect.MakeMap(destField.Type()))
				}
				for _, key := range srcField.MapKeys() {
					destField.SetMapIndex(key, srcField.MapIndex(key))
				}
				dest.sources[field.Name] = source
			}
			continue
		}

		destField.Set(srcField)
		dest.sources[field.Name] = source
	}
}

// markFieldsSource marks all fields as coming from the given source
func markFieldsSource(config *Config, source ConfigSource) {
	v := reflect.ValueOf(config).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Tag.Get("json") == "-" {
			continue
		}
		config.sources[field.Name] = source
	}
}

// isZeroValue checks if a reflect.Value is the zero value for its type
func isZeroValue(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() == 0
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	}
	return false
}
