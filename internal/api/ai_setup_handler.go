package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/llm"
)

type aiCatalogItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Model       string `json:"model"`
	Kind        string `json:"kind"`
	Capability  string `json:"capability"`
	Description string `json:"description"`
	SourceURL   string `json:"source_url"`
	Supported   bool   `json:"supported"`
}

var aiCatalog = []aiCatalogItem{
	{"qwen3-4b", "Qwen3 4B", "qwen3:4b", "ollama", "text-generation", "Small local model for generated analysis.", "https://ollama.com/library/qwen3:4b", true},
	{"qwen3-8b", "Qwen3 8B", "qwen3:8b", "ollama", "text-generation", "Medium local model for generated analysis.", "https://ollama.com/library/qwen3:8b", true},
	{"qwen3-14b", "Qwen3 14B", "qwen3:14b", "ollama", "text-generation", "Larger local model for generated analysis.", "https://ollama.com/library/qwen3:14b", true},
	{"laya-typed-decisions", "Laya Typed Decisions", "typed-decisions", "laya", "typed-decision", "Typed choice, score and yes/no decisions; does not generate analyst prose. Requires a dedicated adapter.", "https://github.com/NandhaKishorM/laya", false},
}

func (s *Server) getAISetup(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	if s.aiProfiles == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI setup unavailable")
		return
	}
	profiles, active := s.aiProfiles.Snapshot()
	jsonOK(w, map[string]any{"profiles": profiles, "catalog": aiCatalog, "active_profile_id": active})
}

func (s *Server) saveAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.aiProfiles == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI setup unavailable")
		return
	}
	var input struct {
		appconfig.AIProfile
		ClearAPIKey bool `json:"clear_api_key"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid profile JSON")
		return
	}
	p := input.AIProfile
	if p.APIKey != "" && strings.TrimSpace(p.APIKey) == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid API key")
		return
	}
	public, err := s.aiProfiles.Save(p, input.ClearAPIKey)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	jsonOK(w, public)
}

func (s *Server) profileConnection(w http.ResponseWriter, r *http.Request) (*llm.Connection, bool) {
	if s.aiProfiles == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI setup unavailable")
		return nil, false
	}
	p, ok := s.aiProfiles.Get(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "profile not found")
		return nil, false
	}
	c, err := llm.NewConnection(p.Kind, p.BaseURL, p.Model, p.APIKey)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid saved profile")
		return nil, false
	}
	return c, true
}

func (s *Server) testAIProfile(w http.ResponseWriter, r *http.Request) {
	if s.aiProfiles == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI setup unavailable")
		return
	}
	p, exists := s.aiProfiles.Get(r.PathValue("id"))
	if !exists {
		writeJSONError(w, http.StatusNotFound, "profile not found")
		return
	}
	c, err := llm.NewConnection(p.Kind, p.BaseURL, p.Model, p.APIKey)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid saved profile")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	start := time.Now()
	output, err := c.Complete(ctx, "Reply briefly.", "Say ready.")
	if err == nil && strings.TrimSpace(output) == "" {
		err = context.DeadlineExceeded
	}
	if err == nil && !s.aiProfiles.MarkTested(p) {
		err = context.Canceled
	}
	result := map[string]any{"ok": err == nil, "message": "connection succeeded", "output": output, "duration_ms": time.Since(start).Milliseconds()}
	if err != nil {
		result["message"] = "connection or inference failed"
		result["output"] = ""
	}
	jsonOK(w, result)
}

func (s *Server) activateAIProfile(w http.ResponseWriter, r *http.Request) {
	s.aiSetupMu.Lock()
	defer s.aiSetupMu.Unlock()
	if s.aiProfiles == nil || s.aiProvider == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "AI setup unavailable")
		return
	}
	// Construct before persisting so validation errors cannot change the active ID.
	p, ok := s.aiProfiles.Get(r.PathValue("id"))
	if !ok {
		writeJSONError(w, http.StatusNotFound, "profile not found")
		return
	}
	provider, err := llm.NewConnection(p.Kind, p.BaseURL, p.Model, p.APIKey)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid profile")
		return
	}
	if _, err := s.aiProfiles.ActivateSnapshot(p); err != nil {
		writeJSONError(w, http.StatusConflict, err.Error())
		return
	}
	s.aiProvider.Set(provider)
	if s.aiActivated != nil {
		s.aiActivated()
	}
	jsonOK(w, map[string]bool{"ok": true})
}

func (s *Server) listAIModels(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	c, ok := s.profileConnection(w, r)
	if !ok {
		return
	}
	models, err := c.Models(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "could not list models from AI host")
		return
	}
	jsonOK(w, map[string]any{"models": models})
}

func (s *Server) pullAIModel(w http.ResponseWriter, r *http.Request) {
	c, ok := s.profileConnection(w, r)
	if !ok {
		return
	}
	var input struct {
		Model string `json:"model"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&input); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid pull request")
		return
	}
	if strings.TrimSpace(input.Model) == "" {
		writeJSONError(w, http.StatusBadRequest, "model is required")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()
	if err := c.Pull(ctx, input.Model); err != nil {
		writeJSONError(w, http.StatusBadGateway, "model pull failed")
		return
	}
	jsonOK(w, map[string]bool{"ok": true})
}
