package errors

import (
	"encoding/json"
	"io"
	"net/http"
)

// ErrorFromResponse creates an APIError from an HTTP response
func ErrorFromResponse(resp *http.Response) *APIError {
	apiErr := &APIError{
		HTTPStatusCode: resp.StatusCode,
		RequestID:      resp.Header.Get("request-id"),
	}

	// Attempt to read and parse the response body
	body, err := io.ReadAll(resp.Body)
	if err == nil && len(body) > 0 {
		if err := json.Unmarshal(body, apiErr); err == nil {
			return apiErr
		}
	}

	// If we couldn't parse the error response, create a generic error
	apiErr.Type = "api_error"
	apiErr.ErrorDetails.Type = http.StatusText(resp.StatusCode)
	apiErr.ErrorDetails.Message = "Request failed with status " + http.StatusText(resp.StatusCode)

	return apiErr
}
