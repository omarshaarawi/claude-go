package ratelimit

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

var defaultRateLimits = map[string]RateLimitConfig{
	// Claude 3.5 Family
	"claude-3-5-sonnet-20241022": {
		RequestsPerMinute: 50,
		TokensPerMinute:   40000,
		TokensPerDay:      1000000,
	},
	"claude-3-5-haiku-20241022": {
		RequestsPerMinute: 50,
		TokensPerMinute:   50000,
		TokensPerDay:      5000000,
	},
	// Claude 3 Family
	"claude-3-opus-20240229": {
		RequestsPerMinute: 50,
		TokensPerMinute:   20000,
		TokensPerDay:      1000000,
	},
	"claude-3-sonnet-20240229": {
		RequestsPerMinute: 50,
		TokensPerMinute:   40000,
		TokensPerDay:      1000000,
	},
	"claude-3-haiku-20240307": {
		RequestsPerMinute: 50,
		TokensPerMinute:   50000,
		TokensPerDay:      5000000,
	},
}

// RateLimitConfig represents the rate limits for a model
type RateLimitConfig struct {
	RequestsPerMinute int
	TokensPerMinute   int
	TokensPerDay      int
}

// TokenBucket implements a thread-safe token bucket algorithm
type TokenBucket struct {
	tokens         float64
	capacity       float64
	rate           float64
	lastRefillTime time.Time
	retryAfter     time.Time
	mu             sync.Mutex
}

// NewTokenBucket creates a new token bucket with initial capacity and refill rate
func NewTokenBucket(capacity int, refillPerSecond float64) *TokenBucket {
	return &TokenBucket{
		tokens:         float64(capacity),
		capacity:       float64(capacity),
		rate:           refillPerSecond,
		lastRefillTime: time.Now(),
	}
}

// Take attempts to take tokens from the bucket, returning wait time if not enough tokens
func (tb *TokenBucket) Take(n int) (time.Duration, bool) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()

	if !tb.retryAfter.IsZero() && now.Before(tb.retryAfter) {
		return tb.retryAfter.Sub(now), false
	}

	elapsed := now.Sub(tb.lastRefillTime).Seconds()
	tb.tokens = math.Min(tb.capacity, tb.tokens+elapsed*tb.rate)
	tb.lastRefillTime = now

	if float64(n) > tb.tokens {
		requiredTokens := float64(n) - tb.tokens
		waitTime := time.Duration(requiredTokens / tb.rate * float64(time.Second))
		return waitTime, false
	}

	tb.tokens -= float64(n)
	return 0, true
}

// SetRetryAfter sets a retry-after time based on API response
func (tb *TokenBucket) SetRetryAfter(d time.Duration) {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	tb.retryAfter = time.Now().Add(d)
}

// UpdateCapacity updates the bucket's capacity and current tokens
func (tb *TokenBucket) UpdateCapacity(newCapacity int) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	ratio := float64(newCapacity) / tb.capacity
	tb.tokens = math.Min(float64(newCapacity), tb.tokens*ratio)
	tb.capacity = float64(newCapacity)
}

// ModelRateLimiter handles rate limiting for a specific model
type ModelRateLimiter struct {
	config            RateLimitConfig
	requestLimiter    *TokenBucket
	tokenMinuteBucket *TokenBucket
	tokenDayBucket    *TokenBucket
	mu                sync.RWMutex
}

// NewModelRateLimiter creates a new model-specific rate limiter
func NewModelRateLimiter(config RateLimitConfig) *ModelRateLimiter {
	return &ModelRateLimiter{
		config:            config,
		requestLimiter:    NewTokenBucket(config.RequestsPerMinute, float64(config.RequestsPerMinute)/60.0),
		tokenMinuteBucket: NewTokenBucket(config.TokensPerMinute, float64(config.TokensPerMinute)/60.0),
		tokenDayBucket:    NewTokenBucket(config.TokensPerDay, float64(config.TokensPerDay)/(24*60*60)),
	}
}

// checkCapacity checks if the request can be made within rate limits
func (ml *ModelRateLimiter) checkCapacity(tokens int) (time.Duration, bool) {
	ml.mu.RLock()
	defer ml.mu.RUnlock()

	if waitTime, ok := ml.requestLimiter.Take(1); !ok {
		return waitTime, false
	}

	if waitTime, ok := ml.tokenMinuteBucket.Take(tokens); !ok {
		return waitTime, false
	}

	if waitTime, ok := ml.tokenDayBucket.Take(tokens); !ok {
		return waitTime, false
	}

	return 0, true
}

// UpdateFromHeaders updates rate limits based on API response headers
func (ml *ModelRateLimiter) UpdateFromHeaders(headers http.Header) {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	if retryAfter := headers.Get("retry-after"); retryAfter != "" {
		if seconds, err := strconv.Atoi(retryAfter); err == nil {
			duration := time.Duration(seconds) * time.Second
			ml.requestLimiter.SetRetryAfter(duration)
			ml.tokenMinuteBucket.SetRetryAfter(duration)
			ml.tokenDayBucket.SetRetryAfter(duration)
		}
	}

	if limit := headers.Get("anthropic-ratelimit-requests-limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			ml.config.RequestsPerMinute = n
			ml.requestLimiter.UpdateCapacity(n)
		}
	}

	if limit := headers.Get("anthropic-ratelimit-tokens-limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			ml.config.TokensPerMinute = n
			ml.tokenMinuteBucket.UpdateCapacity(n)
		}
	}
}

// RateLimiter handles rate limiting across all models
type RateLimiter struct {
	modelLimiters map[string]*ModelRateLimiter
	mu            sync.RWMutex
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		modelLimiters: make(map[string]*ModelRateLimiter),
	}
}

// getModelLimiter gets or creates a rate limiter for a specific model
func (rl *RateLimiter) getModelLimiter(model string) (*ModelRateLimiter, error) {
	rl.mu.RLock()
	limiter, ok := rl.modelLimiters[model]
	rl.mu.RUnlock()
	if ok {
		return limiter, nil
	}

	config, ok := defaultRateLimits[model]
	if !ok {
		return nil, fmt.Errorf("unknown model: %s", model)
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Double-check after acquiring write lock
	if limiter, ok = rl.modelLimiters[model]; ok {
		return limiter, nil
	}

	limiter = NewModelRateLimiter(config)
	rl.modelLimiters[model] = limiter
	return limiter, nil
}

// WaitForCapacity waits until rate limits allow the request
func (rl *RateLimiter) WaitForCapacity(ctx context.Context, model string, totalTokens int) error {
	limiter, err := rl.getModelLimiter(model)
	if err != nil {
		return err
	}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	attempt := 0
	maxAttempts := 50

	for {
		waitTime, allowed := limiter.checkCapacity(totalTokens)
		if allowed {
			return nil
		}

		attempt++
		if attempt >= maxAttempts {
			return fmt.Errorf("rate limit exceeded: max wait time reached")
		}

		backoffDuration := time.Duration(math.Min(
			float64(waitTime),
			float64(100*time.Millisecond)*math.Pow(1.5, float64(attempt)),
		))

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoffDuration):
			continue
		}
	}
}

// UpdateLimits updates rate limits based on API response
func (rl *RateLimiter) UpdateLimits(model string, headers http.Header) {
	limiter, err := rl.getModelLimiter(model)
	if err != nil {
		return
	}

	limiter.UpdateFromHeaders(headers)
}

// QueueRequest represents a rate-limited request
type QueueRequest struct {
	Model      string
	TokenCount int
	Context    context.Context
	ResultChan chan<- error
}

// RequestQueue handles queuing and processing of rate-limited requests
type RequestQueue struct {
	requests    chan *QueueRequest
	rateLimiter *RateLimiter
	workerCount int
	shutdown    chan struct{}
}

// NewRequestQueue creates a new request queue
func NewRequestQueue(rateLimiter *RateLimiter, workerCount int) *RequestQueue {
	q := &RequestQueue{
		requests:    make(chan *QueueRequest, 1000),
		rateLimiter: rateLimiter,
		workerCount: workerCount,
		shutdown:    make(chan struct{}),
	}

	for i := 0; i < workerCount; i++ {
		go q.worker()
	}

	return q
}

// SetModelLimits updates or sets the rate limits for a specific model
func (rl *RateLimiter) SetModelLimits(model string, config RateLimitConfig) error {
	if config.RequestsPerMinute <= 0 {
		return fmt.Errorf("requests per minute must be positive")
	}
	if config.TokensPerMinute <= 0 {
		return fmt.Errorf("tokens per minute must be positive")
	}
	if config.TokensPerDay <= 0 {
		return fmt.Errorf("tokens per day must be positive")
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.modelLimiters[model] = NewModelRateLimiter(config)
	return nil
}

// GetModelLimits returns the current rate limits for a specific model
func (rl *RateLimiter) GetModelLimits(model string) (*RateLimitConfig, error) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	limiter, ok := rl.modelLimiters[model]
	if !ok {
		return nil, fmt.Errorf("no rate limits found for model: %s", model)
	}

	config := limiter.config
	return &config, nil
}

// Submit adds a request to the queue
func (q *RequestQueue) Submit(req *QueueRequest) error {
	select {
	case q.requests <- req:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("queue full")
	}
}

// worker processes requests from the queue
func (q *RequestQueue) worker() {
	for {
		select {
		case <-q.shutdown:
			return
		case req := <-q.requests:
			err := q.rateLimiter.WaitForCapacity(req.Context, req.Model, req.TokenCount)
			req.ResultChan <- err
		}
	}
}

// Shutdown stops all workers
func (q *RequestQueue) Shutdown() {
	close(q.shutdown)
}
