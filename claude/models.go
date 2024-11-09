package claude

import "github.com/omarshaarawi/claude-go/claude/internal/models"

// Model represents a Claude model identifier
type Model = models.Model

// ModelFamily represents the family of Claude models
type ModelFamily = models.ModelFamily

// ModelConfig contains configuration for a specific model
type ModelConfig = models.ModelConfig

// Public model constants
const (
	// Claude 3.5 Family
	ModelClaude35Sonnet = models.ModelClaude35Sonnet
	ModelClaude35Haiku  = models.ModelClaude35Haiku

	// Claude 3 Family
	ModelClaude3Opus   = models.ModelClaude3Opus
	ModelClaude3Sonnet = models.ModelClaude3Sonnet
	ModelClaude3Haiku  = models.ModelClaude3Haiku

	// Model Families
	ModelFamilyClaude3  = models.ModelFamilyClaude3
	ModelFamilyClaude35 = models.ModelFamilyClaude35

	// Default model
	DefaultModel = models.DefaultModel
)

// GetModelsByFamily returns all models belonging to a specific family
func GetModelsByFamily(family ModelFamily) []Model {
	return models.GetModelsByFamily(family)
}

// GetModelConfig returns the configuration for a specific model
func GetModelConfig(model Model) (ModelConfig, error) {
	return models.GetModelConfig(model)
}

// GetLatestModel returns the latest model of a specific type
func GetLatestModel(modelType string) (Model, error) {
	return models.GetLatestModel(modelType)
}

// ValidateModel checks if a model is valid and supported
func ValidateModel(model Model) error {
	return models.ValidateModel(model)
}
