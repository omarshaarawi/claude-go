package errors

// ErrorType represents the type of error returned by the Claude API
type ErrorType string

const (
	// ErrorTypeInvalidRequest indicates there was an issue with the format or content of the request
	ErrorTypeInvalidRequest ErrorType = "invalid_request_error"

	// ErrorTypeAuthentication indicates there's an issue with the API key
	ErrorTypeAuthentication ErrorType = "authentication_error"

	// ErrorTypePermission indicates the API key doesn't have permission
	ErrorTypePermission ErrorType = "permission_error"

	// ErrorTypeNotFound indicates the requested resource was not found
	ErrorTypeNotFound ErrorType = "not_found_error"

	// ErrorTypeRequestTooLarge indicates the request exceeds the maximum allowed size
	ErrorTypeRequestTooLarge ErrorType = "request_too_large"

	// ErrorTypeRateLimit indicates the account has hit a rate limit
	ErrorTypeRateLimit ErrorType = "rate_limit_error"

	// ErrorTypeAPI indicates an unexpected error has occurred in Anthropic's systems
	ErrorTypeAPI ErrorType = "api_error"

	// ErrorTypeOverloaded indicates the API is temporarily overloaded
	ErrorTypeOverloaded ErrorType = "overloaded_error"
)

// APIError represents an error returned by the Claude API
type APIError struct {
	Type         string `json:"type"`
	ErrorDetails struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
	HTTPStatusCode int    `json:"-"`
	RequestID      string `json:"-"`
}

// Error implements the error interface
func (e *APIError) Error() string {
	return e.ErrorDetails.Message
}

// ValidationError represents an error in validating request parameters
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

// RateLimitError represents a rate limit error with additional information
type RateLimitError struct {
	*APIError
	RetryAfter                 int
	RateLimitRequestsLimit     int
	RateLimitRequestsRemaining int
	RateLimitRequestsReset     string
	RateLimitTokensLimit       int
	RateLimitTokensRemaining   int
	RateLimitTokensReset       string
}
