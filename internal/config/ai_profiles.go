package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// AIProfile is stored locally. API handlers must use PublicAIProfile so secrets
// never leave the server in setup responses.
type AIProfile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
}

type PublicAIProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	BaseURL   string `json:"base_url"`
	Model     string `json:"model"`
	HasAPIKey bool   `json:"has_api_key"`
	Active    bool   `json:"active"`
	Tested    bool   `json:"tested"`
}

func (p AIProfile) Public(active bool) PublicAIProfile {
	return PublicAIProfile{p.ID, p.Name, p.Kind, p.BaseURL, p.Model, p.APIKey != "", active, false}
}

func ValidateAIProfile(p AIProfile) (AIProfile, error) {
	p.Name = strings.TrimSpace(p.Name)
	p.Model = strings.TrimSpace(p.Model)
	p.BaseURL = strings.TrimSpace(p.BaseURL)
	if p.Name == "" || p.Model == "" {
		return p, errors.New("name and model are required")
	}
	if p.Kind != "ollama" && p.Kind != "openai-compatible" {
		return p, errors.New("kind must be ollama or openai-compatible")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.RawFragment != "" {
		return p, errors.New("base_url must be an http(s) URL without credentials, query or fragment")
	}
	if strings.ContainsAny(p.BaseURL, "\r\n\t") {
		return p, errors.New("invalid base_url")
	}
	p.BaseURL = strings.TrimRight(u.String(), "/")
	return p, nil
}

type aiProfileFile struct {
	Profiles        []AIProfile `json:"profiles"`
	ActiveProfileID string      `json:"active_profile_id"`
	Activated       *AIProfile  `json:"activated,omitempty"`
}

// AIProfileStore persists profile changes atomically with mode 0600.
type AIProfileStore struct {
	mu     sync.RWMutex
	path   string
	data   aiProfileFile
	tested map[string]AIProfile
}

func NewAIProfileStore(path string) (*AIProfileStore, error) {
	s := &AIProfileStore{path: path, data: aiProfileFile{Profiles: []AIProfile{}}, tested: make(map[string]AIProfile)}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return nil, fmt.Errorf("read AI profiles: %w", err)
	}
	if err := os.Chmod(path, 0600); err != nil {
		return nil, fmt.Errorf("secure AI profile file: %w", err)
	}
	for i, p := range s.data.Profiles {
		valid, err := ValidateAIProfile(p)
		if err != nil {
			return nil, fmt.Errorf("invalid saved AI profile %d: %w", i, err)
		}
		s.data.Profiles[i] = valid
	}
	if s.data.Activated != nil {
		active, err := ValidateAIProfile(*s.data.Activated)
		if err != nil {
			return nil, fmt.Errorf("invalid activated AI profile: %w", err)
		}
		s.data.Activated = &active
	}
	if s.data.Profiles == nil {
		s.data.Profiles = []AIProfile{}
	}
	return s, nil
}

func (s *AIProfileStore) Snapshot() ([]PublicAIProfile, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]PublicAIProfile, 0, len(s.data.Profiles))
	for _, p := range s.data.Profiles {
		item := p.Public(p.ID == s.data.ActiveProfileID && (s.data.Activated == nil || p == *s.data.Activated))
		item.Tested = s.tested[p.ID] == p
		out = append(out, item)
	}
	return out, s.data.ActiveProfileID
}

func (s *AIProfileStore) Get(id string) (AIProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, p := range s.data.Profiles {
		if p.ID == id {
			return p, true
		}
	}
	return AIProfile{}, false
}

func (s *AIProfileStore) Active() (AIProfile, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.data.Activated != nil {
		return *s.data.Activated, true
	}
	for _, p := range s.data.Profiles {
		if p.ID == s.data.ActiveProfileID {
			return p, true
		}
	}
	return AIProfile{}, false
}

func (s *AIProfileStore) Save(p AIProfile, clearAPIKey bool) (PublicAIProfile, error) {
	p, err := ValidateAIProfile(p)
	if err != nil {
		return PublicAIProfile{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next := aiProfileFile{Profiles: append([]AIProfile(nil), s.data.Profiles...), ActiveProfileID: s.data.ActiveProfileID, Activated: s.data.Activated}
	index := -1
	for i, old := range next.Profiles {
		if old.ID == p.ID && p.ID != "" {
			index = i
			if p.APIKey == "" && !clearAPIKey && (p.BaseURL != old.BaseURL || p.Kind != old.Kind) && old.APIKey != "" {
				return PublicAIProfile{}, errors.New("changing host or kind requires a new API key or clear_api_key")
			}
			if p.APIKey == "" && !clearAPIKey {
				p.APIKey = old.APIKey
			}
			break
		}
	}
	if p.ID != "" && index < 0 {
		return PublicAIProfile{}, errors.New("profile not found")
	}
	if index < 0 {
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return PublicAIProfile{}, err
		}
		p.ID = hex.EncodeToString(id[:])
		next.Profiles = append(next.Profiles, p)
	} else {
		next.Profiles[index] = p
	}
	if err := s.write(next); err != nil {
		return PublicAIProfile{}, err
	}
	s.data = next
	delete(s.tested, p.ID)
	return p.Public(p.ID == next.ActiveProfileID && next.Activated != nil && p == *next.Activated), nil
}

// MarkTested records a successful sample inference for exactly this saved revision.
func (s *AIProfileStore) MarkTested(p AIProfile) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, saved := range s.data.Profiles {
		if saved.ID == p.ID && saved == p {
			s.tested[p.ID] = p
			return true
		}
	}
	return false
}

func (s *AIProfileStore) Activate(id string) (AIProfile, error) {
	p, ok := s.Get(id)
	if !ok {
		return AIProfile{}, errors.New("profile not found")
	}
	return s.ActivateSnapshot(p)
}

// ActivateSnapshot persists exactly the tested profile snapshot supplied by the
// caller. It fails if the saved profile changed after the caller read it.
func (s *AIProfileStore) ActivateSnapshot(expected AIProfile) (AIProfile, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.data.Profiles {
		if p.ID != expected.ID {
			continue
		}
		if p != expected {
			return AIProfile{}, errors.New("profile changed before activation")
		}
		if s.tested[expected.ID] != expected {
			return AIProfile{}, errors.New("profile must pass a sample inference test before activation")
		}
		next := s.data
		next.ActiveProfileID = expected.ID
		active := expected
		next.Activated = &active
		if err := s.write(next); err != nil {
			return AIProfile{}, err
		}
		s.data = next
		return expected, nil
	}
	return AIProfile{}, errors.New("profile not found")
}

func (s *AIProfileStore) write(data aiProfileFile) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".ai-profiles-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(0600); err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
