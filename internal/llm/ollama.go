package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OllamaProvider implements the Provider interface against a local (or
// compose-networked) Ollama server via POST /api/generate. It is opt-in —
// cmd/server/main.go only selects it when LLMProvider == "ollama".
type OllamaProvider struct {
	host   string
	model  string
	client *http.Client
}

// NewOllamaProvider constructs a provider. host should include scheme
// (e.g., "http://localhost:11434"). timeout <= 0 falls back to 120s —
// local inference can be slow on modest hardware.
func NewOllamaProvider(host, model string, timeout time.Duration) *OllamaProvider {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OllamaProvider{
		host:  host,
		model: model,
		client: &http.Client{Timeout: timeout},
	}
}

// Complete satisfies the Provider interface.
func (p *OllamaProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	prompt := systemPrompt
	if prompt != "" && userPrompt != "" {
		prompt += "\n\n"
	}
	prompt += userPrompt

	body, err := json.Marshal(map[string]any{
		"model":  p.model,
		"prompt": prompt,
		"stream": false,
		"options": map[string]any{
			"temperature": 0.3,
			"num_predict": 512,
		},
	})
	if err != nil {
		return "", fmt.Errorf("ollama: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.host+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("ollama: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama: request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20)) // 4 MiB cap
	if err != nil {
		return "", fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Try to surface Ollama's own error message.
		var errBody struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(respBody, &errBody)
		if errBody.Error != "" {
			return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, errBody.Error)
		}
		return "", fmt.Errorf("ollama: %d %s", resp.StatusCode, string(respBody))
	}

	var out struct {
		Response string `json:"response"`
	}
	if err := json.Unmarshal(respBody, &out); err != nil {
		return "", fmt.Errorf("ollama: parse response: %w", err)
	}
	return out.Response, nil
}
