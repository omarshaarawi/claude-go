package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/omarshaarawi/claude-go/claude/internal/errors"
)

// MessageBatchStatus represents the processing status of a message batch
type MessageBatchStatus string

const (
	MessageBatchStatusInProgress MessageBatchStatus = "in_progress"
	MessageBatchStatusCanceling  MessageBatchStatus = "canceling"
	MessageBatchStatusEnded      MessageBatchStatus = "ended"
)

// MessageBatch represents a batch of message requests
type MessageBatch struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"`
	ProcessingStatus  MessageBatchStatus `json:"processing_status"`
	RequestCounts     RequestCounts      `json:"request_counts"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
	CreatedAt         time.Time          `json:"created_at"`
	ExpiresAt         time.Time          `json:"expires_at"`
	ArchivedAt        *time.Time         `json:"archived_at,omitempty"`
	CancelInitiatedAt *time.Time         `json:"cancel_initiated_at,omitempty"`
	ResultsURL        string             `json:"results_url"`
}

// RequestCounts represents the counts of requests in different states
type RequestCounts struct {
	Processing int `json:"processing"`
	Succeeded  int `json:"succeeded"`
	Errored    int `json:"errored"`
	Canceled   int `json:"canceled"`
	Expired    int `json:"expired"`
}

// BatchRequest represents a single request in a batch
type BatchRequest struct {
	CustomID string          `json:"custom_id"`
	Params   *MessageRequest `json:"params"`
}

// BatchRequestResult represents the result of a batch request
type BatchRequestResult struct {
	CustomID string      `json:"custom_id"`
	Result   BatchResult `json:"result"`
}

// BatchResult represents the result type and associated data
type BatchResult struct {
	Type    string    `json:"type"` // succeeded, errored, canceled, expired
	Message *Message  `json:"message,omitempty"`
	Error   *APIError `json:"error,omitempty"`
}

// CreateBatchRequest represents a request to create a message batch
type CreateBatchRequest struct {
	Requests []BatchRequest `json:"requests"`
}

// ListBatchesOptions represents options for listing message batches
type ListBatchesOptions struct {
	BeforeID string `url:"before_id,omitempty"`
	AfterID  string `url:"after_id,omitempty"`
	Limit    int    `url:"limit,omitempty"`
}

// ListBatchesResponse represents the response from listing message batches
type ListBatchesResponse struct {
	Data    []MessageBatch `json:"data"`
	HasMore bool           `json:"has_more"`
	FirstID string         `json:"first_id"`
	LastID  string         `json:"last_id"`
}

// MessageBatchesService handles communication with the Message Batches API
type MessageBatchesService service

// Create creates a new message batch
func (s *MessageBatchesService) Create(ctx context.Context, req *CreateBatchRequest) (*MessageBatch, error) {
	if err := validateCreateBatchRequest(req); err != nil {
		return nil, err
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodPost, "/messages/batches", req)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("anthropic-beta", "message-batches-2024-09-24")

	var batch MessageBatch
	if err := s.client.do(httpReq, &batch); err != nil {
		return nil, err
	}

	return &batch, nil
}

// Get retrieves a message batch by ID
func (s *MessageBatchesService) Get(ctx context.Context, batchID string) (*MessageBatch, error) {
	if batchID == "" {
		return nil, errors.NewValidationError("batchID", "cannot be empty")
	}

	path := fmt.Sprintf("/messages/batches/%s", batchID)
	httpReq, err := s.client.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("anthropic-beta", "message-batches-2024-09-24")

	var batch MessageBatch
	if err := s.client.do(httpReq, &batch); err != nil {
		return nil, err
	}

	return &batch, nil
}

// List lists all message batches
func (s *MessageBatchesService) List(ctx context.Context, opts *ListBatchesOptions) (*ListBatchesResponse, error) {
	query := make(map[string]string)
	if opts != nil {
		if opts.BeforeID != "" {
			query["before_id"] = opts.BeforeID
		}
		if opts.AfterID != "" {
			query["after_id"] = opts.AfterID
		}
		if opts.Limit > 0 {
			query["limit"] = fmt.Sprintf("%d", opts.Limit)
		}
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodGet, "/messages/batches", nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	q := httpReq.URL.Query()
	for k, v := range query {
		q.Add(k, v)
	}
	httpReq.URL.RawQuery = q.Encode()

	httpReq.Header.Set("anthropic-beta", "message-batches-2024-09-24")

	var response ListBatchesResponse
	if err := s.client.do(httpReq, &response); err != nil {
		return nil, err
	}

	return &response, nil
}

// Cancel cancels a message batch
func (s *MessageBatchesService) Cancel(ctx context.Context, batchID string) (*MessageBatch, error) {
	if batchID == "" {
		return nil, errors.NewValidationError("batchID", "cannot be empty")
	}

	path := fmt.Sprintf("/messages/batches/%s/cancel", batchID)
	httpReq, err := s.client.newRequest(ctx, http.MethodPost, path, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("anthropic-beta", "message-batches-2024-09-24")

	var batch MessageBatch
	if err := s.client.do(httpReq, &batch); err != nil {
		return nil, err
	}

	return &batch, nil
}

// GetResults retrieves the results of a message batch
func (s *MessageBatchesService) GetResults(ctx context.Context, batchID string) (<-chan BatchRequestResult, <-chan error, error) {
	if batchID == "" {
		return nil, nil, errors.NewValidationError("batchID", "cannot be empty")
	}

	path := fmt.Sprintf("/messages/batches/%s/results", batchID)
	httpReq, err := s.client.newRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating request: %w", err)
	}

	httpReq.Header.Set("anthropic-beta", "message-batches-2024-09-24")

	resp, err := s.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("error sending request: %w", err)
	}

	// Check for errors
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		apiErr := errors.ErrorFromResponse(resp)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, nil, errors.NewRateLimitError(resp, apiErr)
		}
		return nil, nil, apiErr
	}

	resultChan := make(chan BatchRequestResult)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(resultChan)
		defer close(errChan)

		reader := bufio.NewReader(resp.Body)

		for {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
				// Read line
				line, err := reader.ReadBytes('\n')
				if err != nil {
					if err == io.EOF {
						return
					}
					errChan <- fmt.Errorf("error reading results: %w", err)
					return
				}

				// Parse JSON
				var result BatchRequestResult
				if err := json.Unmarshal(line, &result); err != nil {
					errChan <- fmt.Errorf("error parsing result: %w", err)
					return
				}

				resultChan <- result
			}
		}
	}()

	return resultChan, errChan, nil
}

func validateCreateBatchRequest(req *CreateBatchRequest) error {
	if req == nil {
		return errors.NewValidationError("request", "cannot be nil")
	}

	if len(req.Requests) == 0 {
		return errors.NewValidationError("requests", "cannot be empty")
	}

	if len(req.Requests) > 10000 {
		return errors.NewValidationError("requests", "cannot contain more than 10,000 requests")
	}

	for i, batchReq := range req.Requests {
		if batchReq.CustomID == "" {
			return errors.NewValidationError(fmt.Sprintf("requests[%d].custom_id", i), "cannot be empty")
		}

		if err := validateMessageRequest(batchReq.Params); err != nil {
			return fmt.Errorf("invalid request at index %d: %w", i, err)
		}
	}

	return nil
}
