// Package config loads and validates the watchdog YAML configuration.
package config

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	defaultAddress           = ":8080"
	defaultTitle             = "Endpoint Watchdog"
	defaultMaxConcurrent     = 5
	defaultDegradedThreshold = 750 * time.Millisecond
	defaultTimeout           = 5 * time.Second
	defaultInterval          = 30 * time.Second
	defaultRetryAttempts     = 3
	defaultInitialBackoff    = 200 * time.Millisecond
)

// Duration adds YAML text decoding to time.Duration.
type Duration struct {
	time.Duration
}

// UnmarshalText parses values such as "500ms", "5s", or "1m".
func (d *Duration) UnmarshalText(text []byte) error {
	value, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", text, err)
	}
	d.Duration = value
	return nil
}

// Server contains process-wide monitoring and HTTP settings.
type Server struct {
	Address           string   `yaml:"address"`
	Title             string   `yaml:"title"`
	MaxConcurrent     int      `yaml:"maxConcurrent"`
	DegradedThreshold Duration `yaml:"degradedThreshold"`
}

// Retry configures exponential-backoff attempts for failed checks.
type Retry struct {
	Attempts       int      `yaml:"attempts"`
	InitialBackoff Duration `yaml:"initialBackoff"`
}

// Endpoint defines one HTTP target.
type Endpoint struct {
	Name           string            `yaml:"name"`
	URL            string            `yaml:"url"`
	Method         string            `yaml:"method"`
	ExpectedStatus int               `yaml:"expectedStatus"`
	Timeout        Duration          `yaml:"timeout"`
	Interval       Duration          `yaml:"interval"`
	ExpectedText   string            `yaml:"expectedText,omitempty"`
	Headers        map[string]string `yaml:"headers,omitempty"`
}

// Config is the complete application configuration.
type Config struct {
	Server    Server     `yaml:"server"`
	Retry     Retry      `yaml:"retry"`
	Endpoints []Endpoint `yaml:"endpoints"`
}

// Load reads a YAML file, applies defaults, rejects unknown fields, and validates every target.
func Load(path string) (Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config: %w", err)
	}
	defer file.Close()

	cfg, err := Decode(file)
	if err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	return cfg, nil
}

// Decode parses configuration from a reader.
func Decode(reader io.Reader) (Config, error) {
	cfg := Config{
		Server: Server{
			Address:           defaultAddress,
			Title:             defaultTitle,
			MaxConcurrent:     defaultMaxConcurrent,
			DegradedThreshold: Duration{Duration: defaultDegradedThreshold},
		},
		Retry: Retry{
			Attempts:       defaultRetryAttempts,
			InitialBackoff: Duration{Duration: defaultInitialBackoff},
		},
	}
	decoder := yaml.NewDecoder(reader)
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, err
	}
	if err := ensureSingleDocument(decoder); err != nil {
		return Config{}, err
	}
	applyEndpointDefaults(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func ensureSingleDocument(decoder *yaml.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("multiple YAML documents are not supported")
}

func applyEndpointDefaults(cfg *Config) {
	for index := range cfg.Endpoints {
		endpoint := &cfg.Endpoints[index]
		endpoint.Name = strings.TrimSpace(endpoint.Name)
		endpoint.URL = strings.TrimSpace(endpoint.URL)
		endpoint.Method = strings.ToUpper(strings.TrimSpace(endpoint.Method))
		if endpoint.Method == "" {
			endpoint.Method = http.MethodGet
		}
		if endpoint.ExpectedStatus == 0 {
			endpoint.ExpectedStatus = http.StatusOK
		}
		if endpoint.Timeout.Duration == 0 {
			endpoint.Timeout.Duration = defaultTimeout
		}
		if endpoint.Interval.Duration == 0 {
			endpoint.Interval.Duration = defaultInterval
		}
	}
}

// Validate checks safe bounds and URL/method constraints.
func (cfg Config) Validate() error {
	if strings.TrimSpace(cfg.Server.Address) == "" {
		return errors.New("server.address must not be empty")
	}
	if strings.TrimSpace(cfg.Server.Title) == "" {
		return errors.New("server.title must not be empty")
	}
	if cfg.Server.MaxConcurrent < 1 || cfg.Server.MaxConcurrent > 1000 {
		return errors.New("server.maxConcurrent must be between 1 and 1000")
	}
	if cfg.Server.DegradedThreshold.Duration <= 0 {
		return errors.New("server.degradedThreshold must be positive")
	}
	if cfg.Retry.Attempts < 1 || cfg.Retry.Attempts > 10 {
		return errors.New("retry.attempts must be between 1 and 10")
	}
	if cfg.Retry.InitialBackoff.Duration <= 0 {
		return errors.New("retry.initialBackoff must be positive")
	}
	if len(cfg.Endpoints) == 0 {
		return errors.New("at least one endpoint is required")
	}

	names := make(map[string]struct{}, len(cfg.Endpoints))
	for index, endpoint := range cfg.Endpoints {
		if err := validateEndpoint(endpoint); err != nil {
			return fmt.Errorf("endpoints[%d]: %w", index, err)
		}
		key := strings.ToLower(endpoint.Name)
		if _, exists := names[key]; exists {
			return fmt.Errorf("endpoints[%d]: duplicate endpoint name %q", index, endpoint.Name)
		}
		names[key] = struct{}{}
	}
	return nil
}

func validateEndpoint(endpoint Endpoint) error {
	if endpoint.Name == "" {
		return errors.New("name must not be empty")
	}
	parsed, err := url.ParseRequestURI(endpoint.URL)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("url must be an absolute HTTP URL: %q", endpoint.URL)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsupported URL scheme %q; only http and https are allowed", parsed.Scheme)
	}
	if parsed.User != nil {
		return errors.New("url must not contain embedded credentials")
	}
	if endpoint.Method != http.MethodGet && endpoint.Method != http.MethodHead {
		return fmt.Errorf("method %q is not supported; use GET or HEAD", endpoint.Method)
	}
	if endpoint.ExpectedStatus < 100 || endpoint.ExpectedStatus > 599 {
		return errors.New("expectedStatus must be between 100 and 599")
	}
	if endpoint.Timeout.Duration <= 0 {
		return errors.New("timeout must be positive")
	}
	if endpoint.Interval.Duration <= 0 {
		return errors.New("interval must be positive")
	}
	if endpoint.Method == http.MethodHead && endpoint.ExpectedText != "" {
		return errors.New("expectedText cannot be used with HEAD")
	}
	for name, value := range endpoint.Headers {
		if !validHeader(name, value) {
			return fmt.Errorf("invalid header %q", name)
		}
	}
	return nil
}

func validHeader(name, value string) bool {
	if strings.EqualFold(name, "Host") || strings.EqualFold(name, "Content-Length") {
		return false
	}
	if name == "" || strings.ContainsAny(value, "\r\n") {
		return false
	}
	for index := range len(name) {
		character := name[index]
		if !isHeaderTokenCharacter(character) {
			return false
		}
	}
	return true
}

func isHeaderTokenCharacter(character byte) bool {
	if character >= 'a' && character <= 'z' ||
		character >= 'A' && character <= 'Z' ||
		character >= '0' && character <= '9' {
		return true
	}
	return strings.ContainsRune("!#$%&'*+-.^_`|~", rune(character))
}
