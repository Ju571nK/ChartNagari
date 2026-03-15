package llm

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

const groqBaseURL = "https://api.groq.com/openai/v1"
const groqModel = "llama-3.3-70b-versatile"

// GroqProvider wraps the Groq API (OpenAI-compatible).
type GroqProvider struct {
	client *openai.Client
}

// NewGroqProvider creates a Provider backed by Groq Llama 3.3 70B.
func NewGroqProvider(apiKey string) *GroqProvider {
	cfg := openai.DefaultConfig(apiKey)
	cfg.BaseURL = groqBaseURL
	return &GroqProvider{client: openai.NewClientWithConfig(cfg)}
}

func (p *GroqProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: groqModel,
		Messages: []openai.ChatCompletionMessage{
			{Role: openai.ChatMessageRoleSystem, Content: systemPrompt},
			{Role: openai.ChatMessageRoleUser, Content: userPrompt},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	return resp.Choices[0].Message.Content, nil
}
