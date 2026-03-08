// Package api provides the HTTP REST API server for the Chart Analyzer settings UI.
package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"sync"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
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

// OHLCVBar is the chart-compatible OHLCV response.
// Time is Unix seconds (TradingView Lightweight Charts convention).
type OHLCVBar struct {
	Time   int64   `json:"time"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Volume float64 `json:"volume"`
}

// SignalBar is the chart signal marker response.
type SignalBar struct {
	Time      int64   `json:"time"`
	Direction string  `json:"direction"`
	Rule      string  `json:"rule"`
	Score     float64 `json:"score"`
	Message   string  `json:"message"`
}

// ChartStore provides OHLCV and signal data for the chart dashboard.
// *storage.DB satisfies this interface.
type ChartStore interface {
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
	GetSignals(symbol string, limit int) ([]models.Signal, error)
}

// ── Server ────────────────────────────────────────────────────────────────────

// Server is the HTTP API server for the settings UI.
// It serves a REST API for managing watchlist symbols and analysis rules,
// and optionally serves the compiled React frontend as static files.
type Server struct {
	configDir  string
	static     http.Handler // nil when webDist is absent or not built yet
	chartStore ChartStore   // optional; set via WithChartStore
	mu         sync.RWMutex
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

// WithChartStore wires the chart data store (OHLCV + signals) to the server.
func (s *Server) WithChartStore(cs ChartStore) {
	s.chartStore = cs
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

	// Chart dashboard data
	mux.HandleFunc("GET /api/ohlcv/{symbol}/{timeframe}", s.getChartOHLCV)
	mux.HandleFunc("GET /api/signals", s.getChartSignals)

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

// getChartOHLCV returns OHLCV bars for a symbol+timeframe in ascending time order.
// Query param: limit (default 200).
func (s *Server) getChartOHLCV(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		jsonOK(w, []OHLCVBar{})
		return
	}
	symbol := r.PathValue("symbol")
	timeframe := r.PathValue("timeframe")
	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	bars, err := s.chartStore.GetOHLCV(symbol, timeframe, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Convert to chart format; DB returns DESC, chart expects ASC.
	result := make([]OHLCVBar, len(bars))
	for i, b := range bars {
		result[len(bars)-1-i] = OHLCVBar{
			Time:   b.OpenTime.Unix(),
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: b.Volume,
		}
	}
	jsonOK(w, result)
}

// getChartSignals returns recent signals for a symbol as chart markers.
// Query params: symbol (required), limit (default 50).
func (s *Server) getChartSignals(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		jsonOK(w, []SignalBar{})
		return
	}
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		http.Error(w, "symbol parameter required", http.StatusBadRequest)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	sigs, err := s.chartStore.GetSignals(symbol, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	result := make([]SignalBar, len(sigs))
	for i, sig := range sigs {
		result[i] = SignalBar{
			Time:      sig.CreatedAt.Unix(),
			Direction: sig.Direction,
			Rule:      sig.Rule,
			Score:     sig.Score,
			Message:   sig.Message,
		}
	}
	jsonOK(w, result)
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
