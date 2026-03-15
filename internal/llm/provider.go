package llm

import "context"

// Provider is a minimal interface for calling a large language model.
type Provider interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}
