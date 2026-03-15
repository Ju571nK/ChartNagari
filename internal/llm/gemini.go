package llm

import (
	"context"
	"strings"

	genai "github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

const geminiModel = "gemini-2.5-flash"

// GeminiProvider wraps the Google Gemini API.
type GeminiProvider struct {
	apiKey string
}

// NewGeminiProvider creates a Provider backed by Google Gemini 1.5 Flash.
func NewGeminiProvider(apiKey string) *GeminiProvider {
	return &GeminiProvider{apiKey: apiKey}
}

func (p *GeminiProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(p.apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel(geminiModel)
	model.SystemInstruction = &genai.Content{
		Parts: []genai.Part{genai.Text(systemPrompt)},
	}

	resp, err := model.GenerateContent(ctx, genai.Text(userPrompt))
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, cand := range resp.Candidates {
		if cand.Content == nil {
			continue
		}
		for _, part := range cand.Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				sb.WriteString(string(txt))
			}
		}
	}
	return sb.String(), nil
}
