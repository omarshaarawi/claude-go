package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/omarshaarawi/claude-go/claude"
)

func main() {
	// Load configuration, falling back to environment if no config file
	config := &claude.Config{
		APIKey:             os.Getenv("CLAUDE_API_KEY"),
		BaseURL:            "https://api.anthropic.com/v1/",
		LogLevel:           "info",
		EnableRateLimiting: true,
		MaxRetries:         3,
		RetryWaitMin:       1 * time.Second,
		RetryWaitMax:       5 * time.Second,
		Timeout:            30 * time.Second,
	}

	if config.APIKey == "" {
		log.Fatal("CLAUDE_API_KEY environment variable is required")
	}

	// Create client
	client, err := claude.NewClientWithConfig(config)
	if err != nil {
		log.Fatalf("Error creating client: %v", err)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Example 1: Simple Text Message with different models
	fmt.Println("\n=== Example 1: Simple Text Messages with Different Models ===")
	if err := simpleMessages(ctx, client); err != nil {
		log.Printf("Error in simple messages example: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Example 2: Multi-turn Conversation using the Conversation utility
	fmt.Println("\n=== Example 2: Multi-turn Conversation ===")
	if err := improvedConversation(ctx, client); err != nil {
		log.Printf("Error in conversation example: %v", err)
	}

	time.Sleep(1 * time.Second)

	// Example 3: Streaming Response with system prompt
	fmt.Println("\n=== Example 3: Streaming Response with System Prompt ===")
	if err := improvedStreamingMessage(ctx, client); err != nil {
		log.Printf("Error in streaming example: %v", err)
	}

	// Example 4: Model Information
	fmt.Println("\n=== Example 4: Model Information ===")
	printModelInfo()
}

func simpleMessages(ctx context.Context, client *claude.Client) error {
	// Try different models with the same prompt
	models := []claude.Model{
		claude.ModelClaude3Haiku,   // Fastest
		claude.ModelClaude35Sonnet, // Default
		claude.ModelClaude3Opus,    // Most capable
	}

	prompt := "What is the capital of France? Please answer in one sentence."

	for _, model := range models {
		fmt.Printf("\nTrying model: %s\n", model)
		config, _ := claude.GetModelConfig(model)
		fmt.Printf("Model description: %s\n", config.Description)

		response, err := client.SendMessage(ctx, prompt, &claude.ClientOptions{
			Model:     model,
			MaxTokens: config.DefaultTokens,
		})

		if err != nil {
			fmt.Printf("Error with %s: %v\n", model, err)
			continue
		}

		fmt.Printf("Response: %s\n", response)
	}

	return nil
}

func improvedConversation(ctx context.Context, client *claude.Client) error {
	// Create a new conversation with specific options
	conv := client.NewConversation(&claude.ClientOptions{
		Model:  claude.ModelClaude35Sonnet,
		System: "You are a friendly art teacher explaining color theory.",
	})

	// First question
	conv.AddMessage(claude.MessageRoleUser, "What are the three primary colors?")
	response, err := conv.Send(ctx)
	if err != nil {
		return fmt.Errorf("failed to get first response: %w", err)
	}
	fmt.Printf("Assistant: %s\n\n", response)

	// Follow-up question
	conv.AddMessage(claude.MessageRoleUser, "What colors do you get when you mix them?")
	response, err = conv.Send(ctx)
	if err != nil {
		return fmt.Errorf("failed to get second response: %w", err)
	}
	fmt.Printf("Assistant: %s\n", response)

	return nil
}

func improvedStreamingMessage(ctx context.Context, client *claude.Client) error {
	textChan, errChan, err := client.SendMessageStream(ctx,
		"Write a short poem about coding.",
		&claude.ClientOptions{
			Model:  claude.ModelClaude3Haiku, // Using Haiku for faster responses
			System: "You are a poet who writes clever, technical poetry.",
		})

	if err != nil {
		return fmt.Errorf("failed to start stream: %w", err)
	}

	fmt.Print("Assistant: ")
	for {
		select {
		case text, ok := <-textChan:
			if !ok {
				fmt.Println("\nStream completed")
				return nil
			}
			fmt.Print(text)
		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("stream error: %w", err)
			}
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func printModelInfo() {
	fmt.Println("\nClaude 3.5 Family Models:")
	fmt.Println("------------------------")
	for _, model := range claude.GetModelsByFamily(claude.ModelFamilyClaude35) {
		config, _ := claude.GetModelConfig(model)
		printModelDetails(model, config)
	}

	fmt.Println("\nClaude 3 Family Models:")
	fmt.Println("---------------------")
	for _, model := range claude.GetModelsByFamily(claude.ModelFamilyClaude3) {
		config, _ := claude.GetModelConfig(model)
		printModelDetails(model, config)
	}
}

func printModelDetails(model claude.Model, config claude.ModelConfig) {
	fmt.Printf("\nModel: %s (%s)\n", config.Name, model)
	fmt.Printf("Family: %s\n", config.Family)
	fmt.Printf("Description: %s\n", config.Description)
	fmt.Printf("Max Input Tokens: %d\n", config.MaxInputTokens)
	fmt.Printf("Default Response Tokens: %d\n", config.DefaultTokens)
	fmt.Printf("Capabilities: %v\n", config.Capabilities)
	fmt.Printf("Use Cases:\n")
	for _, useCase := range config.UseCases {
		fmt.Printf("- %s\n", useCase)
	}
}
