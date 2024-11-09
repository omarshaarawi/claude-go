package errors

import (
	"fmt"
	"net/http"
)

// NewAPIError creates a new APIError from error details
func NewAPIError(errorType ErrorType, message string) *APIError {
	return &APIError{
		Type: string(errorType),
		ErrorDetails: struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}{
			Type:    string(errorType),
			Message: message,
		},
	}
}

// NewValidationError creates a new ValidationError
func NewValidationError(field, message string) *ValidationError {
	return &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("%s: %s", field, message),
	}
}

// NewRateLimitError creates a new RateLimitError from an HTTP response
func NewRateLimitError(resp *http.Response, apiErr *APIError) *RateLimitError {
	retryAfter := 0
	if retryStr := resp.Header.Get("retry-after"); retryStr != "" {
		fmt.Sscanf(retryStr, "%d", &retryAfter)
	}

	requestsLimit := 0
	if limitStr := resp.Header.Get("anthropic-ratelimit-requests-limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &requestsLimit)
	}

	requestsRemaining := 0
	if remainingStr := resp.Header.Get("anthropic-ratelimit-requests-remaining"); remainingStr != "" {
		fmt.Sscanf(remainingStr, "%d", &requestsRemaining)
	}

	tokensLimit := 0
	if limitStr := resp.Header.Get("anthropic-ratelimit-tokens-limit"); limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &tokensLimit)
	}

	tokensRemaining := 0
	if remainingStr := resp.Header.Get("anthropic-ratelimit-tokens-remaining"); remainingStr != "" {
		fmt.Sscanf(remainingStr, "%d", &tokensRemaining)
	}

	return &RateLimitError{
		APIError:                   apiErr,
		RetryAfter:                 retryAfter,
		RateLimitRequestsLimit:     requestsLimit,
		RateLimitRequestsRemaining: requestsRemaining,
		RateLimitRequestsReset:     resp.Header.Get("anthropic-ratelimit-requests-reset"),
		RateLimitTokensLimit:       tokensLimit,
		RateLimitTokensRemaining:   tokensRemaining,
		RateLimitTokensReset:       resp.Header.Get("anthropic-ratelimit-tokens-reset"),
	}
}
