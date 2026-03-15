package llm

import (
	"context"

	openai "github.com/sashabaranov/go-openai"
)

const openAIModel = openai.GPT4o

// OpenAIProvider wraps the OpenAI ChatGPT API.
type OpenAIProvider struct {
	client *openai.Client
}

// NewOpenAIProvider creates a Provider backed by OpenAI GPT-4o.
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{client: openai.NewClient(apiKey)}
}

func (p *OpenAIProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	resp, err := p.client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model: openAIModel,
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
