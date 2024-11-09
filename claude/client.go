package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/omarshaarawi/claude-go/claude/internal/errors"
	"github.com/omarshaarawi/claude-go/claude/internal/ratelimit"
	"github.com/omarshaarawi/claude-go/claude/internal/transport"
)

const (
	defaultBaseURL = "https://api.anthropic.com"
	defaultTimeout = 30 * time.Second
	apiVersion     = "2023-06-01"
	userAgent      = "claude-go"
)

// Client is the main struct for interacting with the Claude API
type Client struct {
	// HTTP client for making requests
	httpClient *http.Client

	// Base URL for API requests
	baseURL *url.URL

	// API key used for authentication
	apiKey string

	// Logger instance for debug and error logging
	logger *slog.Logger

	// Custom headers to be sent with each request
	headers map[string]string

	// Message service for interacting with the Messages API
	Messages *MessagesService

	// Message Batches service for interacting with the Message Batches API
	MessageBatches *MessageBatchesService

	// Common options for all requests
	common service

	// Rate limiter
	rateLimiter *ratelimit.RateLimiter
}

// service is a base service with a reference to the client
type service struct {
	client *Client
}

// ClientOption is a function that modifies the client configuration
type ClientOption func(*Client) error

// NewClientWithConfig creates a new client using the provided configuration
func NewClientWithConfig(config *Config) (*Client, error) {
	if err := config.validate(); err != nil {
		return nil, err
	}

	baseURL, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	httpClient := &http.Client{
		Timeout: config.Timeout,
	}

	// Configure retries
	if config.MaxRetries > 0 {
		httpClient.Transport = transport.NewRetryTransport(
			http.DefaultTransport,
			&transport.RetryConfig{
				MaxRetries:         config.MaxRetries,
				RetryWaitMin:       config.RetryWaitMin,
				RetryWaitMax:       config.RetryWaitMax,
				RetryableHTTPCodes: config.RetryableHTTPCodes,
			},
		)
	}

	c := &Client{
		httpClient: httpClient,
		baseURL:    baseURL,
		apiKey:     config.APIKey,
		logger:     setupLogger(config),
		headers:    make(map[string]string),
	}

	if config.EnableRateLimiting {
		c.rateLimiter = ratelimit.NewRateLimiter()
		for model, limits := range config.CustomRateLimits {
			if err := c.rateLimiter.SetModelLimits(model, limits); err != nil {
				return nil, fmt.Errorf("error setting rate limits for model %s: %w", model, err)
			}
		}
	}

	c.common.client = c
	c.Messages = (*MessagesService)(&c.common)
	c.MessageBatches = (*MessageBatchesService)(&c.common)

	return c, nil
}

// NewClient creates a new Claude API client
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, errors.NewValidationError("apiKey", "API key cannot be empty")
	}

	baseURL, err := url.Parse(defaultBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	c := &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		baseURL: baseURL,
		apiKey:  apiKey,
		logger:  slog.Default(),
		headers: make(map[string]string),
	}

	for _, opt := range opts {
		if err := opt(c); err != nil {
			return nil, fmt.Errorf("error applying option: %w", err)
		}
	}

	c.common.client = c
	c.rateLimiter = ratelimit.NewRateLimiter()
	c.Messages = (*MessagesService)(&c.common)
	c.MessageBatches = (*MessageBatchesService)(&c.common)

	return c, nil
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *Client) error {
		if httpClient == nil {
			return errors.NewValidationError("httpClient", "HTTP client cannot be nil")
		}
		c.httpClient = httpClient
		return nil
	}
}

// WithBaseURL sets a custom base URL
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) error {
		if baseURL == "" {
			return errors.NewValidationError("baseURL", "Base URL cannot be empty")
		}
		parsedURL, err := url.Parse(baseURL)
		if err != nil {
			return fmt.Errorf("invalid base URL: %w", err)
		}
		c.baseURL = parsedURL
		return nil
	}
}

// WithLogger sets a custom logger
func WithLogger(logger *slog.Logger) ClientOption {
	return func(c *Client) error {
		if logger == nil {
			return errors.NewValidationError("logger", "Logger cannot be nil")
		}
		c.logger = logger
		return nil
	}
}

// WithHeader adds a custom header to all requests
func WithHeader(key, value string) ClientOption {
	return func(c *Client) error {
		if key == "" {
			return errors.NewValidationError("key", "Header key cannot be empty")
		}
		c.headers[key] = value
		return nil
	}
}

// newRequest creates a new HTTP request
func (c *Client) newRequest(ctx context.Context, method, path string, body interface{}) (*http.Request, error) {
	u := *c.baseURL

	path = "/" + strings.Trim(path, "/")

	u.Path = strings.TrimSuffix(u.Path, "/") + path

	c.logger.Debug("building request",
		"base_url", c.baseURL.String(),
		"path", path,
		"final_url", u.String(),
	)

	var buf io.ReadWriter
	if body != nil {
		buf = new(bytes.Buffer)
		enc := json.NewEncoder(buf)
		enc.SetEscapeHTML(false)
		err := enc.Encode(body)
		if err != nil {
			return nil, fmt.Errorf("error encoding request body: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), buf)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s (%s/%s)", userAgent, Version, runtime.GOOS, runtime.GOARCH))

	// Set custom headers
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	return req, nil
}

// do performs the HTTP request and handles the response
func (c *Client) do(req *http.Request, v interface{}) error {
	c.logger.Debug("making request",
		"method", req.Method,
		"url", req.URL.String(),
		"path", req.URL.Path,
		"request_id", req.Context().Value("request_id"),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("received response",
		"status_code", resp.StatusCode,
		"body_length", len(body),
		"request_id", resp.Header.Get("request-id"),
	)

	if resp.StatusCode >= 400 {
		var apiErr APIError
		if err := json.Unmarshal(body, &apiErr); err != nil {
			// If we can't parse the error response, return the raw response
			return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return errors.NewRateLimitError(resp, &apiErr)
		}
		return &apiErr
	}

	if v != nil && len(body) > 0 {
		if err := json.Unmarshal(body, v); err != nil {
			c.logger.Error("failed to decode response",
				"error", err,
				"body", string(body),
			)
			return fmt.Errorf("failed to decode response: %w", err)
		}
	}

	return nil
}

// Helper function to estimate tokens
func estimateTokens(req *http.Request) (inputTokens, outputTokens int) {
	var reqBody struct {
		MaxTokens int `json:"max_tokens"`
		Messages  []struct {
			Content string `json:"content"`
		} `json:"messages"`
	}

	if err := json.NewDecoder(req.Body).Decode(&reqBody); err == nil {
		bodyBytes, _ := json.Marshal(reqBody)
		req.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		for _, msg := range reqBody.Messages {
			inputTokens += len(msg.Content) / 4
		}
		outputTokens = reqBody.MaxTokens
	}

	return inputTokens, outputTokens
}

func (c *Client) Version() string {
	return Version
}

func setupLogger(config *Config) *slog.Logger {
	var logHandler slog.Handler

	level := slog.LevelInfo
	switch strings.ToLower(config.LogLevel) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	var output io.Writer
	switch strings.ToLower(config.LogOutput) {
	case "stderr":
		output = os.Stderr
	case "file":
		if f, err := os.OpenFile("claude.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			output = f
		} else {
			output = os.Stdout
		}
	default:
		output = os.Stdout
	}

	opts := &slog.HandlerOptions{Level: level}
	switch strings.ToLower(config.LogFormat) {
	case "text":
		logHandler = slog.NewTextHandler(output, opts)
	default:
		logHandler = slog.NewJSONHandler(output, opts)
	}

	return slog.New(logHandler)
}
