package models

import "github.com/omarshaarawi/claude-go/claude/internal/errors"

// Model represents a Claude model identifier
type Model string

const (
	// Claude 3.5 Family
	ModelClaude35Sonnet Model = "claude-3-5-sonnet-20241022" // Latest Sonnet 3.5
	ModelClaude35Haiku  Model = "claude-3-5-haiku-20241022"  // Latest Haiku 3.5

	// Claude 3 Family
	ModelClaude3Opus   Model = "claude-3-opus-20240229"   // Latest Opus
	ModelClaude3Sonnet Model = "claude-3-sonnet-20240229" // Latest Sonnet
	ModelClaude3Haiku  Model = "claude-3-haiku-20240307"  // Latest Haiku

	// Default model to use if none specified
	DefaultModel = ModelClaude35Sonnet
)

// ModelFamily represents the family of Claude models
type ModelFamily string

const (
	ModelFamilyClaude3  ModelFamily = "Claude 3"
	ModelFamilyClaude35 ModelFamily = "Claude 3.5"
)

// ModelConfig contains configuration for a specific model
type ModelConfig struct {
	Name           string
	Family         ModelFamily
	MaxInputTokens int
	DefaultTokens  int // Default value for max_tokens if not specified
	Capabilities   []string
	Description    string
	UseCases       []string
}

// Known capabilities
const (
	CapabilityVision      = "vision"
	CapabilityTools       = "tools"
	CapabilityCompletion  = "completion"
	CapabilityGeneration  = "generation"
	CapabilityAnalysis    = "analysis"
	CapabilityParsing     = "parsing"
	CapabilityCode        = "code"
	CapabilityTranslation = "translation"
)

// ModelConfigs maps Model IDs to their configurations
var ModelConfigs = map[Model]ModelConfig{
	// Claude 3.5 Family
	ModelClaude35Sonnet: {
		Name:           "Claude 3.5 Sonnet",
		Family:         ModelFamilyClaude35,
		MaxInputTokens: 150000,
		DefaultTokens:  4096,
		Capabilities: []string{
			CapabilityVision,
			CapabilityTools,
			CapabilityCompletion,
			CapabilityGeneration,
			CapabilityAnalysis,
			CapabilityParsing,
			CapabilityCode,
			CapabilityTranslation,
		},
		Description: "Most intelligent model, combining top-tier performance with improved speed.",
		UseCases: []string{
			"Advanced research and analysis",
			"Complex problem-solving",
			"Sophisticated language understanding and generation",
			"High-level strategic planning",
			"Code generation",
		},
	},
	ModelClaude35Haiku: {
		Name:           "Claude 3.5 Haiku",
		Family:         ModelFamilyClaude35,
		MaxInputTokens: 100000,
		DefaultTokens:  1024,
		Capabilities: []string{
			CapabilityVision,
			CapabilityTools,
			CapabilityCompletion,
			CapabilityGeneration,
			CapabilityCode,
		},
		Description: "Fastest and most-cost effective model.",
		UseCases: []string{
			"Real-time chatbots",
			"Data extraction and labeling",
			"Content classification",
		},
	},

	// Claude 3 Family
	ModelClaude3Opus: {
		Name:           "Claude 3 Opus",
		Family:         ModelFamilyClaude3,
		MaxInputTokens: 200000,
		DefaultTokens:  4096,
		Capabilities: []string{
			CapabilityVision,
			CapabilityTools,
			CapabilityCompletion,
			CapabilityGeneration,
			CapabilityAnalysis,
			CapabilityParsing,
			CapabilityCode,
			CapabilityTranslation,
		},
		Description: "Strong performance on highly complex tasks, such as math and coding.",
		UseCases: []string{
			"Task automation across APIs and databases",
			"Powerful coding tasks",
			"R&D, brainstorming and hypothesis generation",
			"Drug discovery",
			"Strategy and advanced analysis",
			"Data processing over vast amounts of knowledge",
		},
	},
	ModelClaude3Sonnet: {
		Name:           "Claude 3 Sonnet",
		Family:         ModelFamilyClaude3,
		MaxInputTokens: 150000,
		DefaultTokens:  4096,
		Capabilities: []string{
			CapabilityVision,
			CapabilityTools,
			CapabilityCompletion,
			CapabilityGeneration,
			CapabilityAnalysis,
			CapabilityParsing,
			CapabilityCode,
			CapabilityTranslation,
		},
		Description: "Balances intelligence and speed for high-throughput tasks.",
		UseCases: []string{
			"Sales forecasting",
			"Targeted marketing",
			"Code generation",
			"Quality control",
			"Live support chat",
			"Translations",
		},
	},
	ModelClaude3Haiku: {
		Name:           "Claude 3 Haiku",
		Family:         ModelFamilyClaude3,
		MaxInputTokens: 100000,
		DefaultTokens:  1024,
		Capabilities: []string{
			CapabilityVision,
			CapabilityTools,
			CapabilityCompletion,
			CapabilityGeneration,
			CapabilityCode,
		},
		Description: "Near-instant responsiveness that can mimic human interactions.",
		UseCases: []string{
			"Content moderation",
			"Extracting knowledge from unstructured data",
			"Live support chat",
		},
	},
}

// GetModelsByFamily returns all models belonging to a specific family
func GetModelsByFamily(family ModelFamily) []Model {
	var models []Model
	for model, config := range ModelConfigs {
		if config.Family == family {
			models = append(models, model)
		}
	}
	return models
}

// GetLatestModel returns the latest model of a specific type (opus, sonnet, or haiku)
func GetLatestModel(modelType string) (Model, error) {
	switch modelType {
	case "opus":
		if _, ok := ModelConfigs[ModelClaude3Opus]; ok {
			return ModelClaude3Opus, nil
		}
	case "sonnet":
		if _, ok := ModelConfigs[ModelClaude35Sonnet]; ok {
			return ModelClaude35Sonnet, nil
		}
		if _, ok := ModelConfigs[ModelClaude3Sonnet]; ok {
			return ModelClaude3Sonnet, nil
		}
	case "haiku":
		if _, ok := ModelConfigs[ModelClaude35Haiku]; ok {
			return ModelClaude35Haiku, nil
		}
		if _, ok := ModelConfigs[ModelClaude3Haiku]; ok {
			return ModelClaude3Haiku, nil
		}
	default:
		return "", errors.NewValidationError("modelType", "invalid model type: "+modelType)
	}
	return "", errors.NewValidationError("modelType", "no available model found for type: "+modelType)
}

// ClientConfig adds model-related configuration options
type ClientConfig struct {
	DefaultModel Model // Model to use if none specified
}

// ValidateModel checks if a model is valid and supported
func ValidateModel(model Model) error {
	if model == "" {
		return errors.NewValidationError("model", "model cannot be empty")
	}

	if _, ok := ModelConfigs[model]; !ok {
		return errors.NewValidationError("model", "unsupported model: "+string(model))
	}

	return nil
}

// GetModelConfig returns the configuration for a specific model
func GetModelConfig(model Model) (ModelConfig, error) {
	if config, ok := ModelConfigs[model]; ok {
		return config, nil
	}
	return ModelConfig{}, errors.NewValidationError("model", "unsupported model: "+string(model))
}

// HasCapability checks if a model has a specific capability
func HasCapability(model Model, capability string) bool {
	config, ok := ModelConfigs[model]
	if !ok {
		return false
	}

	for _, cap := range config.Capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}
