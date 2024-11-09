package claude

import (
	"context"
	"fmt"

	"github.com/omarshaarawi/claude-go/claude/internal/models"
)

// Conversation represents a conversation with Claude
type Conversation struct {
	client   *Client
	messages []MessageContent
	options  ClientOptions
}

// NewConversation creates a new conversation instance
func (c *Client) NewConversation(opts *ClientOptions) *Conversation {
	options := defaultOptions()
	if opts != nil {
		if opts.Model != "" {
			options.Model = opts.Model
		}
		if opts.MaxTokens > 0 {
			options.MaxTokens = opts.MaxTokens
		}
		options.System = opts.System
	}

	return &Conversation{
		client:   c,
		messages: make([]MessageContent, 0),
		options:  options,
	}
}

// WithModel sets the model for the conversation
func (conv *Conversation) WithModel(model models.Model) *Conversation {
	conv.options.Model = model
	return conv
}

// WithSystem sets the system prompt for the conversation
func (conv *Conversation) WithSystem(system string) *Conversation {
	conv.options.System = system
	return conv
}

// AddMessage adds a message to the conversation without sending it
func (conv *Conversation) AddMessage(role MessageRole, content string) *Conversation {
	conv.messages = append(conv.messages, MessageContent{
		Role:    role,
		Content: content,
	})
	return conv
}

// WithMaxTokens sets the maximum tokens for the conversation
func (conv *Conversation) WithMaxTokens(maxTokens int) *Conversation {
	conv.options.MaxTokens = maxTokens
	return conv
}

// Send sends the current conversation to Claude and gets a response
func (conv *Conversation) Send(ctx context.Context) (string, error) {
	if err := models.ValidateModel(conv.options.Model); err != nil {
		return "", err
	}

	req := &MessageRequest{
		Model:     string(conv.options.Model),
		MaxTokens: conv.options.MaxTokens,
		System:    conv.options.System,
		Messages:  conv.messages,
	}

	resp, err := conv.client.Messages.Create(ctx, req)
	if err != nil {
		return "", fmt.Errorf("failed to send conversation: %w", err)
	}

	if len(resp.Content) == 0 {
		return "", fmt.Errorf("received empty response")
	}

	conv.messages = append(conv.messages, MessageContent{
		Role: MessageRoleAssistant,
		Content: []ContentBlock{
			{
				Type: ContentBlockTypeText,
				Text: resp.Content[0].Text,
			},
		},
	})

	return resp.Content[0].Text, nil
}
