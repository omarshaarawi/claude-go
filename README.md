# Claude Go Client

A feature-rich Go client library for the Anthropic Claude API. This library provides idiomatic Go bindings for Claude's Messages and Message Batches APIs, with built-in support for streaming, rate limiting, retries, and model management.

## Features

- ✨ Full support for the Claude 3 and 3.5 model families
- 🔄 Built-in retries with exponential backoff
- 🚦 Automatic rate limiting
- 📺 Streaming support with Server-Sent Events
- 🛠 Message batching for high-throughput use cases
- 💬 Conversation management utilities
- ⚙️ Configurable via environment variables or config file
- 📝 Comprehensive logging support

## Installation

```bash
go get github.com/omarshaarawi/claude-go
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/omarshaarawi/claude-go/claude"
)

func main() {
    // Create a client with an API key
    client, err := claude.NewClient("your-api-key")
    if err != nil {
        log.Fatal(err)
    }

    // Simple message
    response, err := client.SendMessage(context.Background(),
        "What is the meaning of life?",
        &claude.ClientOptions{
            Model: claude.ModelClaude35Sonnet,
        },
    )
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(response)
}
```

## Advanced Usage

### Streaming Responses

```go
textChan, errChan, err := client.SendMessageStream(ctx,
    "Write a story about a robot learning to love.",
    &claude.ClientOptions{
        Model:  claude.ModelClaude3Haiku,
        System: "You are a creative storyteller.",
    },
)
if err != nil {
    log.Fatal(err)
}

for {
    select {
    case text, ok := <-textChan:
        if !ok {
            return
        }
        fmt.Print(text)
    case err := <-errChan:
        if err != nil {
            log.Fatal(err)
        }
        return
    case <-ctx.Done():
        return
    }
}
```

### Managing Conversations

```go
conv := client.NewConversation(&claude.ClientOptions{
    Model:  claude.ModelClaude35Sonnet,
    System: "You are a helpful coding assistant.",
})

// First message
conv.AddMessage(claude.MessageRoleUser, "What is a closure in Go?")
response, err := conv.Send(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Assistant:", response)

// Follow-up
conv.AddMessage(claude.MessageRoleUser, "Can you show an example?")
response, err = conv.Send(ctx)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Assistant:", response)
```

### Batch Processing

```go
batch := &claude.CreateBatchRequest{
    Requests: []claude.BatchRequest{
        {
            CustomID: "question-1",
            Params: &claude.MessageRequest{
                Model: string(claude.ModelClaude35Sonnet),
                Messages: []claude.MessageContent{
                    {
                        Role:    claude.MessageRoleUser,
                        Content: "What is 2+2?",
                    },
                },
            },
        },
        // Add more requests...
    },
}

result, err := client.MessageBatches.Create(ctx, batch)
if err != nil {
    log.Fatal(err)
}
```

### Advanced Configuration

```go
config := &claude.Config{
    APIKey:  os.Getenv("CLAUDE_API_KEY"),
    BaseURL: "https://api.anthropic.com/v1/",

    // HTTP Client Configuration
    MaxRetries:   3,
    RetryWaitMin: 1 * time.Second,
    RetryWaitMax: 5 * time.Second,

    // Rate Limiting
    EnableRateLimiting: true,

    // Logging
    LogLevel:  "debug",
    LogFormat: "json",
}

client, err := claude.NewClientWithConfig(config)
if err != nil {
    log.Fatal(err)
}
```

## Available Models

The library supports all Claude 3 and 3.5 models:

- `claude.ModelClaude35Sonnet` - Best balance of intelligence and speed
- `claude.ModelClaude35Haiku` - Fastest response times
- `claude.ModelClaude3Opus` - Most capable for complex tasks
- `claude.ModelClaude3Sonnet` - Balanced performance
- `claude.ModelClaude3Haiku` - Fast and efficient

## Error Handling

The library provides typed errors for better error handling:

```go
response, err := client.SendMessage(ctx, "Hello", nil)
if err != nil {
    switch {
    case claude.IsRateLimitError(err):
        // Handle rate limit
    case claude.IsValidationError(err):
        // Handle validation error
    case claude.IsAPIError(err):
        // Handle API error
    default:
        // Handle other errors
    }
}
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
