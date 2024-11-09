package claude

import "github.com/omarshaarawi/claude-go/claude/internal/errors"

// Public error type assertions
type (
	APIError        = errors.APIError
	ValidationError = errors.ValidationError
	RateLimitError  = errors.RateLimitError
)

// Public error checking functions
func IsAPIError(err error) (*APIError, bool) {
	if err == nil {
		return nil, false
	}
	apiErr, ok := err.(*APIError)
	return apiErr, ok
}

// IsValidationError checks if an error is a ValidationError
func IsValidationError(err error) (*ValidationError, bool) {
	if err == nil {
		return nil, false
	}
	valErr, ok := err.(*ValidationError)
	return valErr, ok
}

// IsRateLimitError checks if an error is a RateLimitError
func IsRateLimitError(err error) (*RateLimitError, bool) {
	if err == nil {
		return nil, false
	}
	rateLimitErr, ok := err.(*RateLimitError)
	return rateLimitErr, ok
}
