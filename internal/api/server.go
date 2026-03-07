// Package api provides the HTTP REST API server for the Chart Analyzer settings UI.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"gopkg.in/yaml.v3"
)

// ── API response types ────────────────────────────────────────────────────────

// SymbolItem is the JSON representation of a single watchlist entry.
type SymbolItem struct {
	Symbol   string `json:"symbol"`
	Type     string `json:"type"`     // "crypto" | "stock"
	Exchange string `json:"exchange"`
	Enabled  bool   `json:"enabled"`
}

// RuleItem is the JSON representation of a single analysis rule.
type RuleItem struct {
	Name        string                 `json:"name"`
	Enabled     bool                   `json:"enabled"`
	Methodology string                 `json:"methodology"`
	Params      map[string]interface{} `json:"params"`
}

// StatusItem is the JSON representation of the system status.
type StatusItem struct {
	Phase   string `json:"phase"`
	Symbols int    `json:"symbols"`
	Rules   int    `json:"rules"`
	Tests   int    `json:"tests"`
}

// ── Server ────────────────────────────────────────────────────────────────────

// Server is the HTTP API server for the settings UI.
// It serves a REST API for managing watchlist symbols and analysis rules,
// and optionally serves the compiled React frontend as static files.
type Server struct {
	configDir string
	static    http.Handler // nil when webDist is absent or not built yet
	mu        sync.RWMutex
}

// New creates a Server.
//   - configDir: directory containing watchlist.yaml and rules.yaml.
//   - webDist:   path to the compiled React frontend (web/dist); empty or
//     non-existent path → static serving is disabled.
func New(configDir, webDist string) *Server {
	s := &Server{configDir: configDir}
	if webDist != "" {
		if _, err := os.Stat(webDist); err == nil {
			s.static = http.FileServer(http.Dir(webDist))
		}
	}
	return s
}

// Handler returns the fully configured http.Handler for the server.
// All /api/* routes are registered; other paths fall through to the static
// file server when available.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Status
	mux.HandleFunc("GET /api/status", s.getStatus)

	// Watchlist symbols
	mux.HandleFunc("GET /api/symbols", s.getSymbols)
	mux.HandleFunc("PUT /api/symbols/{symbol}", s.updateSymbol)

	// Analysis rules
	mux.HandleFunc("GET /api/rules", s.getRules)
	mux.HandleFunc("PUT /api/rules/{name}", s.updateRule)

	// Static frontend (SPA)
	if s.static != nil {
		mux.Handle("/", s.static)
	}

	return corsMiddleware(mux)
}

// ── handlers ─────────────────────────────────────────────────────────────────

func (s *Server) getStatus(w http.ResponseWriter, _ *http.Request) {
	wl, _ := s.readWatchlist()
	rc, _ := s.readRules()

	total := len(wl.Symbols.Crypto) + len(wl.Symbols.Stocks)

	jsonOK(w, StatusItem{
		Phase:   "Phase 1: Core MVP",
		Symbols: total,
		Rules:   len(rc.Rules),
		Tests:   100,
	})
}

func (s *Server) getSymbols(w http.ResponseWriter, _ *http.Request) {
	wl, err := s.readWatchlist()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]SymbolItem, 0, len(wl.Symbols.Crypto)+len(wl.Symbols.Stocks))
	for _, sym := range wl.Symbols.Crypto {
		items = append(items, SymbolItem{Symbol: sym.Symbol, Type: "crypto", Exchange: sym.Exchange, Enabled: sym.Enabled})
	}
	for _, sym := range wl.Symbols.Stocks {
		items = append(items, SymbolItem{Symbol: sym.Symbol, Type: "stock", Exchange: sym.Exchange, Enabled: sym.Enabled})
	}
	jsonOK(w, items)
}

func (s *Server) updateSymbol(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wl, err := s.readWatchlistLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	for i := range wl.Symbols.Crypto {
		if wl.Symbols.Crypto[i].Symbol == symbol {
			wl.Symbols.Crypto[i].Enabled = body.Enabled
			found = true
			break
		}
	}
	if !found {
		for i := range wl.Symbols.Stocks {
			if wl.Symbols.Stocks[i].Symbol == symbol {
				wl.Symbols.Stocks[i].Enabled = body.Enabled
				found = true
				break
			}
		}
	}

	if !found {
		http.Error(w, "symbol not found", http.StatusNotFound)
		return
	}

	if err := s.writeWatchlistLocked(wl); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getRules(w http.ResponseWriter, _ *http.Request) {
	rc, err := s.readRules()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	items := make([]RuleItem, len(rc.Rules))
	for i, r := range rc.Rules {
		items[i] = RuleItem{
			Name:        r.Name,
			Enabled:     r.Enabled,
			Methodology: r.Methodology,
			Params:      r.Params,
		}
	}
	jsonOK(w, items)
}

func (s *Server) updateRule(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rc, err := s.readRulesLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	for i := range rc.Rules {
		if rc.Rules[i].Name == name {
			rc.Rules[i].Enabled = body.Enabled
			found = true
			break
		}
	}

	if !found {
		http.Error(w, "rule not found", http.StatusNotFound)
		return
	}

	if err := s.writeRulesLocked(rc); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── YAML file helpers ─────────────────────────────────────────────────────────

// readWatchlist acquires a read lock and reads watchlist.yaml.
func (s *Server) readWatchlist() (appconfig.WatchlistConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readWatchlistLocked()
}

// readWatchlistLocked reads watchlist.yaml. Caller must hold s.mu (read or write).
func (s *Server) readWatchlistLocked() (appconfig.WatchlistConfig, error) {
	var wl appconfig.WatchlistConfig
	f, err := os.Open(s.configDir + "/watchlist.yaml")
	if err != nil {
		return wl, fmt.Errorf("watchlist 읽기 실패: %w", err)
	}
	defer f.Close()
	return wl, yaml.NewDecoder(f).Decode(&wl)
}

// writeWatchlistLocked writes watchlist.yaml. Caller must hold s.mu (write).
func (s *Server) writeWatchlistLocked(wl appconfig.WatchlistConfig) error {
	data, err := yaml.Marshal(wl)
	if err != nil {
		return fmt.Errorf("watchlist 직렬화 실패: %w", err)
	}
	return os.WriteFile(s.configDir+"/watchlist.yaml", data, 0o644)
}

// readRules acquires a read lock and reads rules.yaml.
func (s *Server) readRules() (appconfig.RulesConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.readRulesLocked()
}

// readRulesLocked reads rules.yaml. Caller must hold s.mu (read or write).
func (s *Server) readRulesLocked() (appconfig.RulesConfig, error) {
	var rc appconfig.RulesConfig
	f, err := os.Open(s.configDir + "/rules.yaml")
	if err != nil {
		return rc, fmt.Errorf("rules 읽기 실패: %w", err)
	}
	defer f.Close()
	return rc, yaml.NewDecoder(f).Decode(&rc)
}

// writeRulesLocked writes rules.yaml. Caller must hold s.mu (write).
func (s *Server) writeRulesLocked(rc appconfig.RulesConfig) error {
	data, err := yaml.Marshal(rc)
	if err != nil {
		return fmt.Errorf("rules 직렬화 실패: %w", err)
	}
	return os.WriteFile(s.configDir+"/rules.yaml", data, 0o644)
}

// ── utilities ─────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// corsMiddleware adds CORS headers and handles OPTIONS preflight requests.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
