package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/omarshaarawi/claude-go/claude/internal/errors"
	"github.com/omarshaarawi/claude-go/claude/internal/transport"
)

// Message represents a message in the Claude API
type Message struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Role         string         `json:"role"`
	Content      []ContentBlock `json:"content"`
	Model        string         `json:"model"`
	StopReason   *string        `json:"stop_reason,omitempty"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        *MessageUsage  `json:"usage,omitempty"`
}

// MessageUsage represents the token usage for a message
type MessageUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ContentBlockType represents the type of content in a message
type ContentBlockType string

const (
	ContentBlockTypeText    ContentBlockType = "text"
	ContentBlockTypeImage   ContentBlockType = "image"
	ContentBlockTypeToolUse ContentBlockType = "tool_use"
)

// ContentBlock represents a block of content in a message
type ContentBlock struct {
	Type    ContentBlockType `json:"type"`
	Text    string           `json:"text,omitempty"`
	Image   *ImageContent    `json:"image,omitempty"`
	ToolUse *ToolUse         `json:"tool_use,omitempty"`
}

// ImageContent represents an image in a message
type ImageContent struct {
	Source ImageSource `json:"source"`
}

// ImageSource represents the source of an image
type ImageSource struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ToolUse represents a tool use in a message
type ToolUse struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

// MessageRole represents the role of a message sender
type MessageRole string

const (
	MessageRoleUser      MessageRole = "user"
	MessageRoleAssistant MessageRole = "assistant"
)

// MessageRequest represents a request to create a message
type MessageRequest struct {
	Model      string           `json:"model"`
	MaxTokens  int              `json:"max_tokens,omitempty"`
	Messages   []MessageContent `json:"messages"`
	System     string           `json:"system,omitempty"`
	Stream     bool             `json:"stream,omitempty"`
	Tools      []Tool           `json:"tools,omitempty"`
	ToolChoice *ToolChoice      `json:"tool_choice,omitempty"`
}

type MessageContent struct {
	Role    MessageRole `json:"role"`
	Content interface{} `json:"content"`
}

// Tool represents a tool that can be used by Claude
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema ToolInputSchema `json:"input_schema"`
}

// ToolInputSchema represents the schema for tool input
type ToolInputSchema struct {
	Type       string                             `json:"type"`
	Properties map[string]ToolInputSchemaProperty `json:"properties"`
	Required   []string                           `json:"required,omitempty"`
}

// ToolInputSchemaProperty represents a property in a tool input schema
type ToolInputSchemaProperty struct {
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// ToolChoice represents the choice of tools for Claude to use
type ToolChoice struct {
	Type string `json:"type"` // "none" or "any"
}

// StreamEvent represents a streaming event
type StreamEvent struct {
	Type    string   `json:"type"`
	Message *Message `json:"message,omitempty"`
	Delta   *Delta   `json:"delta,omitempty"`
	Index   *int     `json:"index,omitempty"`
}

// Delta represents a text delta in a stream
type Delta struct {
	Type        string `json:"type"`
	Text        string `json:"text,omitempty"`
	PartialJSON string `json:"partial_json,omitempty"`
}

// MessagesService handles communication with the Messages API
type MessagesService service

// Create creates a new message
func (s *MessagesService) Create(ctx context.Context, req *MessageRequest, opts *ClientOptions) (*Message, error) {
	for i, msg := range req.Messages {
		if content, ok := msg.Content.(string); ok {
			req.Messages[i].Content = []ContentBlock{
				{
					Type: ContentBlockTypeText,
					Text: content,
				},
			}
		} else if content, ok := msg.Content.([]ContentBlock); ok {
			req.Messages[i].Content = content
		} else {
			return nil, errors.NewValidationError("content", "must be either string or []ContentBlock")
		}
	}

	if err := validateMessageRequest(req); err != nil {
		return nil, err
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodPost, "/messages", req)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	var message Message
	if err := s.client.do(httpReq, &message); err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}

	return &message, nil
}

// Stream creates a new message and streams the response
func (s *MessagesService) Stream(ctx context.Context, req *MessageRequest) (<-chan StreamEvent, <-chan error, error) {
	req.Stream = true

	for i, msg := range req.Messages {
		if content, ok := msg.Content.(string); ok {
			req.Messages[i].Content = []ContentBlock{
				{
					Type: ContentBlockTypeText,
					Text: content,
				},
			}
		} else if content, ok := msg.Content.([]ContentBlock); ok {
			req.Messages[i].Content = content
		} else {
			return nil, nil, errors.NewValidationError("content", "must be either string or []ContentBlock")
		}
	}

	if err := validateMessageRequest(req); err != nil {
		return nil, nil, err
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodPost, "/messages", req)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating request: %w", err)
	}

	resp, err := s.client.httpClient.Do(httpReq)
	if err != nil {
		return nil, nil, fmt.Errorf("error sending request: %w", err)
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		apiErr := errors.ErrorFromResponse(resp)
		if resp.StatusCode == http.StatusTooManyRequests {
			return nil, nil, errors.NewRateLimitError(resp, apiErr)
		}
		return nil, nil, apiErr
	}

	eventChan := make(chan StreamEvent)
	errChan := make(chan error, 1)

	go func() {
		defer resp.Body.Close()
		defer close(eventChan)
		defer close(errChan)

		decoder := transport.NewSSEDecoder(resp.Body)
		for {
			select {
			case <-ctx.Done():
				errChan <- ctx.Err()
				return
			default:
				event, err := decoder.Decode()
				if err != nil {
					if err == io.EOF {
						return
					}
					errChan <- fmt.Errorf("error decoding stream: %w", err)
					return
				}

				var streamEvent StreamEvent
				if err := json.Unmarshal([]byte(event.Data), &streamEvent); err != nil {
					errChan <- fmt.Errorf("error unmarshaling stream event: %w", err)
					return
				}

				eventChan <- streamEvent
			}
		}
	}()

	return eventChan, errChan, nil
}

// CountTokens counts the number of tokens in a message
func (s *MessagesService) CountTokens(ctx context.Context, req *MessageRequest) (int, error) {
	for i, msg := range req.Messages {
		if content, ok := msg.Content.(string); ok {
			req.Messages[i].Content = []ContentBlock{
				{
					Type: ContentBlockTypeText,
					Text: content,
				},
			}
		} else if content, ok := msg.Content.([]ContentBlock); ok {
			req.Messages[i].Content = content
		} else {
			return 0, errors.NewValidationError("content", "must be either string or []ContentBlock")
		}
	}

	if err := validateMessageRequest(req); err != nil {
		return 0, err
	}

	httpReq, err := s.client.newRequest(ctx, http.MethodPost, "/messages/count_tokens", req)
	if err != nil {
		return 0, fmt.Errorf("error creating request: %w", err)
	}

	var count int
	if err := s.client.do(httpReq, &count); err != nil {
		return 0, fmt.Errorf("error sending request: %w", err)
	}

	return count, nil
}

// validateMessageRequest validates a message request
func validateMessageRequest(req *MessageRequest) error {
	if req == nil {
		return errors.NewValidationError("request", "cannot be nil")
	}

	if req.Model == "" {
		return errors.NewValidationError("model", "cannot be empty")
	}

	if len(req.Messages) == 0 {
		return errors.NewValidationError("messages", "cannot be empty")
	}

	for i, msg := range req.Messages {
		if msg.Role == "" {
			return errors.NewValidationError(fmt.Sprintf("messages[%d].role", i), "cannot be empty")
		}
		if msg.Content == nil {
			return errors.NewValidationError(fmt.Sprintf("messages[%d].content", i), "cannot be nil")
		}
	}

	return nil
}
