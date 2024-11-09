package ratelimit

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// RateLimitConfig represents the rate limits for a model
type RateLimitConfig struct {
	RequestsPerMinute int
	TokensPerMinute   int
	TokensPerDay      int
}

// defaultRateLimits defines the default rate limits for each model
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

// TokenBucket implements a token bucket rate limiter
type TokenBucket struct {
	tokens   float64
	capacity float64
	rate     float64
	lastTime time.Time
	mu       sync.Mutex
}

// NewTokenBucket creates a new token bucket rate limiter
func NewTokenBucket(capacity int, refillPerSecond float64) *TokenBucket {
	return &TokenBucket{
		tokens:   float64(capacity),
		capacity: float64(capacity),
		rate:     refillPerSecond,
		lastTime: time.Now(),
	}
}

// Take attempts to take n tokens from the bucket
func (tb *TokenBucket) Take(n int) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	duration := now.Sub(tb.lastTime).Seconds()
	tb.tokens = min(tb.capacity, tb.tokens+duration*tb.rate)
	tb.lastTime = now

	if float64(n) > tb.tokens {
		return false
	}

	tb.tokens -= float64(n)
	return true
}

// SlidingWindowRateLimiter implements a sliding window rate limiter
type SlidingWindowRateLimiter struct {
	window   time.Duration
	limit    int
	requests []time.Time
	mu       sync.Mutex
}

// NewSlidingWindowRateLimiter creates a new sliding window rate limiter
func NewSlidingWindowRateLimiter(window time.Duration, limit int) *SlidingWindowRateLimiter {
	return &SlidingWindowRateLimiter{
		window:   window,
		limit:    limit,
		requests: make([]time.Time, 0, limit),
	}
}

// Allow checks if a new request is allowed
func (sw *SlidingWindowRateLimiter) Allow() bool {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-sw.window)

	// Remove old requests
	i := 0
	for ; i < len(sw.requests); i++ {
		if sw.requests[i].After(windowStart) {
			break
		}
	}
	if i > 0 {
		sw.requests = sw.requests[i:]
	}

	// Check if we're at the limit
	if len(sw.requests) >= sw.limit {
		return false
	}

	// Add new request
	sw.requests = append(sw.requests, now)
	return true
}

// RateLimiter handles rate limiting for the Claude API
type RateLimiter struct {
	modelLimiters map[string]*ModelRateLimiter
	mu            sync.RWMutex
}

// ModelRateLimiter handles rate limiting for a specific model
type ModelRateLimiter struct {
	config            RateLimitConfig
	requestLimiter    *SlidingWindowRateLimiter
	tokenMinuteBucket *TokenBucket
	tokenDayBucket    *TokenBucket
	mu                sync.RWMutex
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

	limiter = &ModelRateLimiter{
		config:            config,
		requestLimiter:    NewSlidingWindowRateLimiter(time.Minute, config.RequestsPerMinute),
		tokenMinuteBucket: NewTokenBucket(config.TokensPerMinute, float64(config.TokensPerMinute)/60),
		tokenDayBucket:    NewTokenBucket(config.TokensPerDay, float64(config.TokensPerDay)/(24*60*60)),
	}

	rl.modelLimiters[model] = limiter
	return limiter, nil
}

// WaitForCapacity waits until rate limits allow the request
func (rl *RateLimiter) WaitForCapacity(ctx context.Context, model string, inputTokens, outputTokens int) error {
	limiter, err := rl.getModelLimiter(model)
	if err != nil {
		return err
	}

	totalTokens := inputTokens + outputTokens
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if limiter.checkCapacity(totalTokens) {
				return nil
			}
		}
	}
}

// checkCapacity checks if the request can be made within rate limits
func (ml *ModelRateLimiter) checkCapacity(totalTokens int) bool {
	ml.mu.Lock()
	defer ml.mu.Unlock()

	return ml.requestLimiter.Allow() &&
		ml.tokenMinuteBucket.Take(totalTokens) &&
		ml.tokenDayBucket.Take(totalTokens)
}

// UpdateLimits updates the rate limits based on API response headers
func (rl *RateLimiter) UpdateLimits(model string, headers http.Header) {
	limiter, err := rl.getModelLimiter(model)
	if err != nil {
		return
	}

	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	// Update request limits if provided
	if limit := headers.Get("anthropic-ratelimit-requests-limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			limiter.config.RequestsPerMinute = n
		}
	}

	// Update token limits if provided
	if limit := headers.Get("anthropic-ratelimit-tokens-limit"); limit != "" {
		if n, err := strconv.Atoi(limit); err == nil {
			limiter.config.TokensPerMinute = n
			limiter.tokenMinuteBucket = NewTokenBucket(n, float64(n)/60)
		}
	}
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

	limiter := &ModelRateLimiter{
		config:            config,
		requestLimiter:    NewSlidingWindowRateLimiter(time.Minute, config.RequestsPerMinute),
		tokenMinuteBucket: NewTokenBucket(config.TokensPerMinute, float64(config.TokensPerMinute)/60),
		tokenDayBucket:    NewTokenBucket(config.TokensPerDay, float64(config.TokensPerDay)/(24*60*60)),
	}

	rl.modelLimiters[model] = limiter
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

	// Return a copy of the config to prevent external modification
	config := limiter.config
	return &config, nil
}

// UpdateModelLimits updates the rate limits for a specific model
func (rl *RateLimiter) UpdateModelLimits(model string, config RateLimitConfig) error {
	// First remove existing limits if they exist
	rl.mu.Lock()
	if _, exists := rl.modelLimiters[model]; exists {
		delete(rl.modelLimiters, model)
	}
	rl.mu.Unlock()

	// Then set new limits
	return rl.SetModelLimits(model, config)
}

// RemoveModelLimits removes rate limits for a specific model
func (rl *RateLimiter) RemoveModelLimits(model string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.modelLimiters, model)
}

// ResetModelLimits resets all rate limiters for a specific model to their initial state
func (rl *RateLimiter) ResetModelLimits(model string) error {
	rl.mu.RLock()
	config, ok := rl.modelLimiters[model]
	rl.mu.RUnlock()

	if !ok {
		return fmt.Errorf("no rate limits found for model: %s", model)
	}

	return rl.SetModelLimits(model, config.config)
}
