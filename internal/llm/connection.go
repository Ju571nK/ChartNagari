package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Switch delegates each request to the current provider. A nil selection is
// possible at cold start, before the first profile is activated.
type Switch struct {
	mu       sync.RWMutex
	provider Provider
}

func NewSwitch(initial Provider) *Switch { return &Switch{provider: initial} }
func (s *Switch) Set(p Provider) {
	s.mu.Lock()
	s.provider = p
	s.mu.Unlock()
}
func (s *Switch) Available() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.provider != nil
}
func (s *Switch) Complete(ctx context.Context, system, user string) (string, error) {
	s.mu.RLock()
	p := s.provider
	s.mu.RUnlock()
	if p == nil {
		return "", errors.New("AI provider is not configured")
	}
	return p.Complete(ctx, system, user)
}

// Connection uses the selected host for inference, model listing and pulls.
type Connection struct {
	kind    string
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

func NewConnection(kind, baseURL, model, apiKey string) (*Connection, error) {
	u, err := url.Parse(baseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return nil, errors.New("invalid AI host URL")
	}
	if kind != "ollama" && kind != "openai-compatible" {
		return nil, errors.New("invalid AI connection kind")
	}
	return &Connection{kind: kind, baseURL: baseURL, model: model, apiKey: apiKey, client: &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirects are not allowed") },
	}}, nil
}

func (c *Connection) request(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI host request failed: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		resp.Body.Close()
		return nil, fmt.Errorf("AI host returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func (c *Connection) Complete(ctx context.Context, system, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	if c.kind == "ollama" {
		resp, err := c.request(ctx, http.MethodPost, "/api/generate", map[string]any{
			"model": c.model, "system": system, "prompt": user, "stream": false, "think": false,
			"options": map[string]any{"temperature": 0.3, "num_predict": 512},
		})
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		var result struct {
			Response string `json:"response"`
			Error    string `json:"error"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
			return "", err
		}
		if result.Error != "" {
			return "", errors.New("AI host reported an inference error")
		}
		if strings.TrimSpace(result.Response) == "" {
			return "", errors.New("AI host returned no text")
		}
		return result.Response, nil
	}
	resp, err := c.request(ctx, http.MethodPost, "/chat/completions", map[string]any{
		"model":       c.model,
		"messages":    []map[string]string{{"role": "system", "content": system}, {"role": "user", "content": user}},
		"temperature": 0.3,
	})
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("AI host returned no choices")
	}
	if strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", errors.New("AI host returned no text")
	}
	return result.Choices[0].Message.Content, nil
}

func (c *Connection) Models(ctx context.Context) ([]string, error) {
	path := "/models"
	if c.kind == "ollama" {
		path = "/api/tags"
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := c.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return nil, err
	}
	models := make([]string, 0, len(result.Models)+len(result.Data))
	for _, m := range result.Models {
		if m.Name != "" {
			models = append(models, m.Name)
		}
	}
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

func (c *Connection) Pull(ctx context.Context, model string) error {
	if c.kind != "ollama" {
		return errors.New("model pull requires an Ollama profile")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return errors.New("model is required")
	}
	resp, err := c.request(ctx, http.MethodPost, "/api/pull", map[string]any{"model": model, "stream": false})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&result); err != nil {
		return err
	}
	if result.Error != "" || result.Status != "success" {
		return errors.New("Ollama model pull did not complete")
	}
	return nil
}
