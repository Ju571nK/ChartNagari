package llm

import (
	"context"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const anthropicMaxTokens int64 = 1200

// AnthropicProvider wraps the Anthropic Claude API.
type AnthropicProvider struct {
	client    anthropic.Client
	maxTokens int64
}

// NewAnthropicProvider creates a Provider backed by Anthropic Claude (Opus 4.6).
func NewAnthropicProvider(apiKey string, opts ...option.RequestOption) *AnthropicProvider {
	allOpts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, opts...)
	return &AnthropicProvider{
		client:    anthropic.NewClient(allOpts...),
		maxTokens: anthropicMaxTokens,
	}
}

func (p *AnthropicProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	msg, err := p.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_6,
		MaxTokens: p.maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt))},
	})
	if err != nil {
		return "", err
	}
	for _, block := range msg.Content {
		if tb, ok := block.AsAny().(anthropic.TextBlock); ok {
			return tb.Text, nil
		}
	}
	return "", nil
}
