// Package rule defines the core plugin interface for all analysis methodologies.
// Adding a new methodology = create a new file implementing AnalysisRule + add to rules.yaml.
// Existing code must NOT be modified (Open/Closed Principle).
package rule

import "github.com/Ju571nK/Chatter/pkg/models"

// AnalysisRule is the interface every methodology plugin must implement.
//
// Example usage:
//
//	type RSIRule struct{}
//	func (r *RSIRule) Name() string                             { return "rsi_overbought" }
//	func (r *RSIRule) RequiredIndicators() []string             { return []string{"RSI_14"} }
//	func (r *RSIRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) { ... }
type AnalysisRule interface {
	// Name returns the unique identifier for this rule (must match rules.yaml key).
	Name() string

	// RequiredIndicators returns the list of indicator keys this rule needs.
	// The engine will ensure these are computed before calling Analyze.
	RequiredIndicators() []string

	// Analyze evaluates the rule against the given context and returns a Signal.
	// Returns nil, nil when no signal condition is met (not an error).
	Analyze(ctx models.AnalysisContext) (*models.Signal, error)
}
