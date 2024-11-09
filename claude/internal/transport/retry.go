package transport

import (
	"bytes"
	"context"
	"io"
	"math"
	"math/rand"
	"net/http"
	"time"
)

// RetryConfig contains configuration for the retry transport
type RetryConfig struct {
	MaxRetries         int
	RetryWaitMin       time.Duration
	RetryWaitMax       time.Duration
	RetryableHTTPCodes []int
}

// RetryTransport implements automatic retries for failed requests
type RetryTransport struct {
	transport http.RoundTripper
	config    *RetryConfig
}

// NewRetryTransport creates a new retry transport
func NewRetryTransport(transport http.RoundTripper, config *RetryConfig) *RetryTransport {
	return &RetryTransport{
		transport: transport,
		config:    config,
	}
}

// RoundTrip implements the http.RoundTripper interface
func (t *RetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var (
		attempts  = 0
		lastError error
	)

	ctx := req.Context()
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}

	for {
		attempts++

		var body []byte
		if req.Body != nil {
			body, _ = io.ReadAll(req.Body)
			req.Body.Close()
		}

		newReq := req.Clone(ctx)
		if body != nil {
			newReq.Body = io.NopCloser(bytes.NewReader(body))
		}

		resp, err := t.transport.RoundTrip(newReq)
		if err != nil {
			lastError = err
		} else if !t.shouldRetry(resp.StatusCode) {
			return resp, nil
		}

		if attempts >= t.config.MaxRetries {
			if err != nil {
				return nil, lastError
			}
			return resp, nil
		}

		delay := t.calculateDelay(attempts)

		select {
		case <-ctx.Done():
			if err != nil {
				return nil, lastError
			}
			return resp, nil
		case <-time.After(delay):
			continue
		}
	}
}

// shouldRetry checks if a request should be retried based on the status code
func (t *RetryTransport) shouldRetry(statusCode int) bool {
	for _, code := range t.config.RetryableHTTPCodes {
		if statusCode == code {
			return true
		}
	}
	return false
}

// calculateDelay calculates the delay before the next retry attempt
func (t *RetryTransport) calculateDelay(attempt int) time.Duration {
	min := float64(t.config.RetryWaitMin)
	max := float64(t.config.RetryWaitMax)
	base := math.Min(max, min*math.Pow(2, float64(attempt-1)))

	jitter := rand.Float64() * (base * 0.25)
	delay := base + jitter

	if delay > max {
		delay = max
	}

	return time.Duration(delay)
}
