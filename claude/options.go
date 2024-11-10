package claude

import (
	"context"
	"fmt"

	"github.com/omarshaarawi/claude-go/claude/internal/models"
)

// ClientOptions holds configuration options for convenience functions
type ClientOptions struct {
	Model     models.Model
	MaxTokens int
	System    string
}

// defaultOptions returns the default client options
func defaultOptions() ClientOptions {
	return ClientOptions{
		Model:     models.DefaultModel,
		MaxTokens: models.ModelConfigs[models.DefaultModel].DefaultTokens,
	}
}

// SendMessage is a convenience function to send a simple text message and get a text response
func (c *Client) SendMessage(ctx context.Context, message string, opts *ClientOptions) (string, error) {
	options := defaultOptions()
	if opts != nil {
		if opts.Model != "" {
			model, ok := models.ModelConfigs[opts.Model]
			if !ok {
				return "", fmt.Errorf("invalid model: %s", opts.Model)
			}
			options.Model = opts.Model
			options.MaxTokens = model.DefaultTokens
		}
		if opts.MaxTokens > 0 {
			options.MaxTokens = opts.MaxTokens
		}
		options.System = opts.System
	}

	if err := models.ValidateModel(options.Model); err != nil {
		return "", err
	}

	req := &MessageRequest{
		Model:     string(options.Model),
		MaxTokens: options.MaxTokens,
		System:    options.System,
		Messages: []MessageContent{
			{
				Role:    MessageRoleUser,
				Content: message,
			},
		},
	}

	resp, err := c.Messages.Create(ctx, req, opts)
	if err != nil {
		return "", fmt.Errorf("failed to send message: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("received empty response")
	}

	return resp.Content[0].Text, nil
}

// SendMessageStream is a convenience function to stream a simple text message
func (c *Client) SendMessageStream(ctx context.Context, message string, opts *ClientOptions) (<-chan string, <-chan error, error) {
	options := defaultOptions()
	if opts != nil {
		if opts.Model != "" {
			model, ok := models.ModelConfigs[opts.Model]
			if !ok {
				return nil, nil, fmt.Errorf("invalid model: %s", opts.Model)
			}
			options.Model = opts.Model
			options.MaxTokens = model.DefaultTokens
		}
		if opts.MaxTokens > 0 {
			options.MaxTokens = opts.MaxTokens
		}
		options.System = opts.System
	}

	if err := models.ValidateModel(options.Model); err != nil {
		return nil, nil, err
	}

	req := &MessageRequest{
		Model:     string(options.Model),
		MaxTokens: options.MaxTokens,
		System:    options.System,
		Messages: []MessageContent{
			{
				Role:    MessageRoleUser,
				Content: message,
			},
		},
		Stream: true,
	}

	events, errChan, err := c.Messages.Stream(ctx, req)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to start stream: %w", err)
	}

	textChan := make(chan string)
	streamErrChan := make(chan error, 1)

	go func() {
		defer close(textChan)
		defer close(streamErrChan)

		for {
			select {
			case event, ok := <-events:
				if !ok {
					return
				}
				if event.Delta != nil && event.Delta.Text != "" {
					textChan <- event.Delta.Text
				}
			case err := <-errChan:
				if err != nil {
					streamErrChan <- err
				}
				return
			case <-ctx.Done():
				streamErrChan <- ctx.Err()
				return
			}
		}
	}()

	return textChan, streamErrChan, nil
}

// CountTokens is a convenience function to count the number of tokens in a message
func (c *Client) CountTokens(ctx context.Context, message string, opts *ClientOptions) (int, error) {
	options := defaultOptions()
	if opts != nil {
		if opts.Model != "" {
			options.Model = opts.Model
		}
		options.System = opts.System
	}

	if err := models.ValidateModel(options.Model); err != nil {
		return 0, err
	}

	req := &MessageRequest{
		Model:  string(options.Model),
		System: options.System,
		Messages: []MessageContent{
			{
				Role:    MessageRoleUser,
				Content: message,
			},
		},
	}

	resp, err := c.Messages.CountTokens(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("failed to count tokens: %w", err)
	}

	return resp, nil
}
