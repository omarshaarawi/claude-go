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

const (
	BetaPromptCaching  = "prompt-caching-2024-07-31"
	BetaMessageBatches = "message-batches-2024-09-24"
	BetaTokenCounting  = "token-counting-2024-11-01"
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

	// Rate limit request queue
	requestQueue *ratelimit.RequestQueue
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
		c.requestQueue = ratelimit.NewRequestQueue(c.rateLimiter, 5)

		for model, limits := range config.CustomRateLimits {
			if err := c.rateLimiter.SetModelLimits(model, limits); err != nil {
				return nil, fmt.Errorf("error setting rate limits for model %s: %w", model, err)
			}
		}
	}

	if c.headers == nil {
		c.headers = make(map[string]string)
	}

	for _, v := range config.BetaFeatures {
		c.headers["anthropic-beta"] = v
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
	c.requestQueue = ratelimit.NewRequestQueue(c.rateLimiter, 5)

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

// Add type for beta features
type BetaFeature string

// Add helper to enable beta features
func WithBeta(feature string) ClientOption {
	return func(c *Client) error {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers["anthropic-beta"] = feature
		return nil
	}
}

// Add helper to enable multiple beta features
func WithBetas(features ...string) ClientOption {
	return func(c *Client) error {
		if c.headers == nil {
			c.headers = make(map[string]string)
		}
		c.headers["anthropic-beta"] = strings.Join(features, ",")
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

// Close cleans up any resources used by the client
func (c *Client) Close() {
	if c.requestQueue != nil {
		c.requestQueue.Shutdown()
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

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s (%s/%s)", userAgent, Version, runtime.GOOS, runtime.GOARCH))

	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	if model, ok := ctx.Value("model").(string); ok {
		req.Header.Set("x-model", model)
	}

	return req, nil
}

// do performs the HTTP request and handles the response
func (c *Client) do(req *http.Request, v interface{}) error {
	if _, ok := c.headers["anthropic-beta"]; ok {
		req.Header.Set("anthropic-beta", c.headers["anthropic-beta"])
	}

	c.logger.Debug("making request",
		"method", req.Method,
		"url", req.URL.String(),
		"path", req.URL.Path,
		"headers", req.Header,
		"request_id", req.Context().Value("request_id"),
	)

	if c.rateLimiter != nil && c.requestQueue != nil {
		model := req.Header.Get("x-model")
		if model == "" {
			if req.Body != nil {
				bodyBytes, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

				var msgReq struct {
					Model string `json:"model"`
				}
				if err := json.Unmarshal(bodyBytes, &msgReq); err == nil && msgReq.Model != "" {
					model = msgReq.Model
				}
			}
		}

		if model != "" {
			inputTokens := estimateTokenCount(req)

			resultChan := make(chan error, 1)

			queueReq := &ratelimit.QueueRequest{
				Model:      model,
				TokenCount: inputTokens,
				Context:    req.Context(),
				ResultChan: resultChan,
			}

			if err := c.requestQueue.Submit(queueReq); err != nil {
				return fmt.Errorf("rate limit queue error: %w", err)
			}

			if err := <-resultChan; err != nil {
				return fmt.Errorf("rate limit error: %w", err)
			}
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if c.rateLimiter != nil {
		model := req.Header.Get("x-model")
		if model != "" {
			c.rateLimiter.UpdateLimits(model, resp.Header)
		}
	}

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
			return fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
		}
		if resp.StatusCode == http.StatusTooManyRequests {
			return errors.NewRateLimitError(resp, &apiErr)
		}
		return &apiErr
	}

	if v != nil && len(body) > 0 {
		if msg, ok := v.(*Message); ok {
			var fullResp struct {
				Usage *MessageUsage `json:"usage"`
			}
			if err := json.Unmarshal(body, &fullResp); err == nil && fullResp.Usage != nil {
				msg.Usage = fullResp.Usage
			}
		}

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

// Add helper function to estimate token count
func estimateTokenCount(req *http.Request) int {
	if req.Body == nil {
		return 100
	}

	body, err := io.ReadAll(req.Body)
	if err != nil {
		return 100
	}

	req.Body = io.NopCloser(bytes.NewBuffer(body))

	return len(body) / 4
}
