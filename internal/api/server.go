// Package api provides the HTTP REST API server for the Chart Analyzer settings UI.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/analyst"
	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/internal/history"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/pinescript"
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
	Phase          string   `json:"phase"`
	Symbols        int      `json:"symbols"`
	Rules          int      `json:"rules"`
	UptimeSec      int64    `json:"uptime_sec"`
	LastSignalUnix int64    `json:"last_signal_unix"` // 0 = no signal yet
	DataSources    []string `json:"data_sources"`
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
	Symbol           string  `json:"symbol"`
	Time             int64   `json:"time"`
	Direction        string  `json:"direction"`
	Rule             string  `json:"rule"`
	Score            float64 `json:"score"`
	Message          string  `json:"message"`
	AIInterpretation string  `json:"ai_interpretation"`
}

// AggregatedRuleStat aggregates per-rule backtest stats across multiple symbols.
type AggregatedRuleStat struct {
	Rule            string  `json:"rule"`
	SymbolsTested   int     `json:"symbols_tested"`
	TotalTrades     int     `json:"total_trades"`
	AvgWinRate      float64 `json:"avg_win_rate"`
	AvgRR           float64 `json:"avg_rr"`
	AvgProfitFactor float64 `json:"avg_profit_factor"`
	Exportable      bool    `json:"exportable"`
}

// ChartStore provides OHLCV and signal data for the chart dashboard.
// *storage.DB satisfies this interface.
type ChartStore interface {
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
	GetSignals(symbol string, limit int) ([]models.Signal, error)
	GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error)
}

// FullStore extends ChartStore with full OHLCV history and analysis persistence.
type FullStore interface {
	ChartStore
	GetOHLCVAll(symbol, timeframe string) ([]models.OHLCV, error)
	SaveAnalysis(result analyst.ScenarioResult) (int64, error)
	GetAnalysisHistory(symbol string, limit int) ([]storage.AnalysisRecord, error)
	GetAnalysisByID(id int64) (*storage.AnalysisRecord, error)
}

// AnalystDirector runs multi-analyst AI analysis.
type AnalystDirector interface {
	Analyze(ctx context.Context, input analyst.AnalystInput) analyst.ScenarioResult
}

// Announcer sends plain-text messages to all configured notification backends.
type Announcer interface {
	Announce(ctx context.Context, text string)
}

// BacktestRunner executes a backtest and returns the result.
// *backtest.Runner satisfies this interface.
type BacktestRunner interface {
	RunBacktest(symbol, timeframe, ruleFilter string, tpMult, slMult float64) (*backtest.BacktestResult, error)
	RunPerRule(symbol, timeframe string, tpMult, slMult float64) ([]backtest.RuleStats, error)
}

// PaperStore provides paper trading data for the API.
// *storage.DB satisfies this interface.
type PaperStore interface {
	GetAllOpenPositions() ([]paper.PaperPosition, error)
	GetClosedPositions(limit int) ([]paper.PaperPosition, error)
}

// ReportScheduler is implemented by *report.Scheduler.
type ReportScheduler interface {
	Reset(cfg appconfig.DailyReportConfig)
}

// ── Server ────────────────────────────────────────────────────────────────────

// Server is the HTTP API server for the settings UI.
// It serves a REST API for managing watchlist symbols and analysis rules,
// and optionally serves the compiled React frontend as static files.
type Server struct {
	configDir        string
	static           http.Handler                   // nil when webDist is absent or not built yet
	chartStore       ChartStore                     // optional; set via WithChartStore
	backtestRunner   BacktestRunner                 // optional; set via WithBacktestRunner
	paperStore       PaperStore                     // optional; set via WithPaperStore
	reportSched      ReportScheduler                // optional; set via WithReportScheduler
	alertHolder      *appconfig.AlertConfigHolder   // optional; set via WithAlertConfigHolder
	fullStore        FullStore                      // optional; set via WithFullStore
	analystDirector  AnalystDirector                // optional; set via WithAnalystDirector
	announcer        Announcer                      // optional; set via WithAnnouncer
	startTime        time.Time                      // server start timestamp for uptime
	dataSources      []string                       // active data sources (e.g. ["Binance","Tiingo"])
	mu               sync.RWMutex
}

// New creates a Server.
//   - configDir: directory containing watchlist.yaml and rules.yaml.
//   - webDist:   path to the compiled React frontend (web/dist); empty or
//     non-existent path → static serving is disabled.
func New(configDir, webDist string) *Server {
	s := &Server{configDir: configDir, startTime: time.Now()}
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

// WithBacktestRunner wires the backtest runner to the server.
func (s *Server) WithBacktestRunner(br BacktestRunner) {
	s.backtestRunner = br
}

// WithDataSources records which data sources are active for the status display.
func (s *Server) WithDataSources(sources []string) {
	s.dataSources = sources
}

func (s *Server) WithPaperStore(ps PaperStore) {
	s.paperStore = ps
}

// WithReportScheduler wires the daily report scheduler to the server.
func (s *Server) WithReportScheduler(rs ReportScheduler) {
	s.reportSched = rs
}

// WithAlertConfigHolder wires an optional live-updated alert configuration holder.
func (s *Server) WithAlertConfigHolder(h *appconfig.AlertConfigHolder) {
	s.alertHolder = h
}

// WithFullStore wires the full OHLCV store for the analysis endpoint.
func (s *Server) WithFullStore(fs FullStore) {
	s.fullStore = fs
}

// WithAnalystDirector wires the multi-analyst AI director for the analysis endpoint.
func (s *Server) WithAnalystDirector(d AnalystDirector) {
	s.analystDirector = d
}

// WithAnnouncer wires a notification backend for Telegram export.
func (s *Server) WithAnnouncer(a Announcer) {
	s.announcer = a
}

// Handler returns the fully configured http.Handler for the server.
// All /api/* routes are registered; other paths fall through to the static
// file server when available.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// Liveness/health for Docker and load balancers
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Status
	mux.HandleFunc("GET /api/status", s.getStatus)

	// Watchlist symbols
	mux.HandleFunc("GET /api/symbols", s.getSymbols)
	mux.HandleFunc("POST /api/symbols", s.addSymbol)
	mux.HandleFunc("PUT /api/symbols/{symbol}", s.updateSymbol)
	mux.HandleFunc("DELETE /api/symbols/{symbol}", s.removeSymbol)

	// Analysis rules
	mux.HandleFunc("GET /api/rules", s.getRules)
	mux.HandleFunc("PUT /api/rules/{name}", s.updateRule)

	// Chart dashboard data
	mux.HandleFunc("GET /api/ohlcv/{symbol}/{timeframe}", s.getChartOHLCV)
	mux.HandleFunc("GET /api/signals", s.getChartSignals)
	mux.HandleFunc("GET /api/history", s.getHistory)

	// Backtest engine
	mux.HandleFunc("POST /api/backtest", s.runBacktest)
	mux.HandleFunc("GET /api/backtest/rules", s.runPerRuleBacktest)
	mux.HandleFunc("GET /api/performance/rules", s.getPerformanceRules)
	mux.HandleFunc("GET /api/export/pinescript", s.exportPineScript)

	// Paper trading
	mux.HandleFunc("GET /api/paper/positions", s.getPaperPositions)
	mux.HandleFunc("GET /api/paper/history", s.getPaperHistory)
	mux.HandleFunc("GET /api/paper/summary", s.getPaperSummary)

	// Daily report config
	mux.HandleFunc("GET /api/report/config", s.getReportConfig)
	mux.HandleFunc("PUT /api/report/config", s.updateReportConfig)

	// Alert config
	mux.HandleFunc("GET /api/alert/config", s.getAlertConfig)
	mux.HandleFunc("PUT /api/alert/config", s.updateAlertConfig)

	// Multi-analyst full analysis
	mux.HandleFunc("POST /api/analysis/full", s.runFullAnalysis)
	mux.HandleFunc("POST /api/analysis/export", s.runAnalysisExport)
	mux.HandleFunc("GET /api/analysis/history", s.getAnalysisHistory)
	mux.HandleFunc("GET /api/analysis/history/{id}", s.getAnalysisDetail)

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

	// Last signal time via optional type assertion (avoids interface change).
	var lastSignal int64
	if sts, ok := s.chartStore.(interface{ GetLatestSignalTime() (int64, error) }); ok {
		if ts, err := sts.GetLatestSignalTime(); err == nil {
			lastSignal = ts
		}
	}

	sources := s.dataSources
	if len(sources) == 0 {
		sources = []string{}
	}

	jsonOK(w, StatusItem{
		Phase:          "Phase 2: Enhancement",
		Symbols:        total,
		Rules:          len(rc.Rules),
		UptimeSec:      int64(time.Since(s.startTime).Seconds()),
		LastSignalUnix: lastSignal,
		DataSources:    sources,
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
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
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
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
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
			Symbol:           sig.Symbol,
			Time:             sig.CreatedAt.Unix(),
			Direction:        sig.Direction,
			Rule:             sig.Rule,
			Score:            sig.Score,
			Message:          sig.Message,
			AIInterpretation: sig.AIInterpretation,
		}
	}
	jsonOK(w, result)
}

// getHistory handles GET /api/history.
// Query params: symbol (default ALL), direction (default ALL), limit (default 100, max 200).
func (s *Server) getHistory(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		jsonOK(w, []SignalBar{})
		return
	}
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		symbol = "ALL"
	}
	direction := r.URL.Query().Get("direction")
	if direction == "" {
		direction = "ALL"
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	sigs, err := s.chartStore.GetSignalsFiltered(symbol, direction, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result := make([]SignalBar, len(sigs))
	for i, sig := range sigs {
		result[i] = SignalBar{
			Symbol:           sig.Symbol,
			Time:             sig.CreatedAt.Unix(),
			Direction:        sig.Direction,
			Rule:             sig.Rule,
			Score:            sig.Score,
			Message:          sig.Message,
			AIInterpretation: sig.AIInterpretation,
		}
	}
	jsonOK(w, result)
}

// runBacktest handles POST /api/backtest.
// Request body: {"symbol":"BTCUSDT","timeframe":"1H","rule":""}
// Returns a BacktestResult with trade outcomes and performance statistics.
func (s *Server) runBacktest(w http.ResponseWriter, r *http.Request) {
	if s.backtestRunner == nil {
		http.Error(w, "backtest runner not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Symbol    string  `json:"symbol"`
		Timeframe string  `json:"timeframe"`
		Rule      string  `json:"rule"`     // optional filter
		TPMult    float64 `json:"tp_mult"`  // 0 = use default
		SLMult    float64 `json:"sl_mult"`  // 0 = use default
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Symbol == "" || req.Timeframe == "" {
		http.Error(w, "symbol and timeframe are required", http.StatusBadRequest)
		return
	}

	result, err := s.backtestRunner.RunBacktest(req.Symbol, req.Timeframe, req.Rule, req.TPMult, req.SLMult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, result)
}

// runPerRuleBacktest handles GET /api/backtest/rules.
// Query params: symbol (required), timeframe (required), tp_mult (optional), sl_mult (optional).
func (s *Server) runPerRuleBacktest(w http.ResponseWriter, r *http.Request) {
	if s.backtestRunner == nil {
		http.Error(w, "backtest runner not configured", http.StatusServiceUnavailable)
		return
	}
	symbol := r.URL.Query().Get("symbol")
	timeframe := r.URL.Query().Get("timeframe")
	if symbol == "" || timeframe == "" {
		http.Error(w, "symbol and timeframe are required", http.StatusBadRequest)
		return
	}
	var tpMult, slMult float64
	if v := r.URL.Query().Get("tp_mult"); v != "" {
		tpMult, _ = strconv.ParseFloat(v, 64)
	}
	if v := r.URL.Query().Get("sl_mult"); v != "" {
		slMult, _ = strconv.ParseFloat(v, 64)
	}
	stats, err := s.backtestRunner.RunPerRule(symbol, timeframe, tpMult, slMult)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if stats == nil {
		stats = []backtest.RuleStats{}
	}
	jsonOK(w, stats)
}

// getPerformanceRules handles GET /api/performance/rules
// Query params: symbols (comma-separated, required), timeframe (required),
//               tp_mult (optional), sl_mult (optional)
func (s *Server) getPerformanceRules(w http.ResponseWriter, r *http.Request) {
	if s.backtestRunner == nil {
		http.Error(w, "backtest runner not configured", http.StatusServiceUnavailable)
		return
	}
	symbolsParam := r.URL.Query().Get("symbols")
	timeframe := r.URL.Query().Get("timeframe")
	if symbolsParam == "" || timeframe == "" {
		http.Error(w, "symbols and timeframe are required", http.StatusBadRequest)
		return
	}
	symbols := strings.Split(symbolsParam, ",")
	var tpMult, slMult float64
	if v := r.URL.Query().Get("tp_mult"); v != "" {
		tpMult, _ = strconv.ParseFloat(v, 64)
	}
	if v := r.URL.Query().Get("sl_mult"); v != "" {
		slMult, _ = strconv.ParseFloat(v, 64)
	}

	// Collect per-rule stats across all symbols
	type accumulator struct {
		winRateSum      float64
		rrSum           float64
		profitFactorSum float64
		totalTrades     int
		count           int
	}
	acc := make(map[string]*accumulator)

	for _, sym := range symbols {
		sym = strings.TrimSpace(sym)
		if sym == "" {
			continue
		}
		stats, err := s.backtestRunner.RunPerRule(sym, timeframe, tpMult, slMult)
		if err != nil || len(stats) == 0 {
			continue
		}
		for _, st := range stats {
			if _, ok := acc[st.Rule]; !ok {
				acc[st.Rule] = &accumulator{}
			}
			a := acc[st.Rule]
			a.winRateSum += st.WinRate
			a.rrSum += st.AvgRR
			a.profitFactorSum += st.ProfitFactor
			a.totalTrades += st.Trades
			a.count++
		}
	}

	supported := make(map[string]bool)
	for _, r := range pinescript.SupportedRules() {
		supported[r] = true
	}

	result := make([]AggregatedRuleStat, 0, len(acc))
	for rule, a := range acc {
		if a.count == 0 {
			continue
		}
		result = append(result, AggregatedRuleStat{
			Rule:            rule,
			SymbolsTested:   a.count,
			TotalTrades:     a.totalTrades,
			AvgWinRate:      a.winRateSum / float64(a.count),
			AvgRR:           a.rrSum / float64(a.count),
			AvgProfitFactor: a.profitFactorSum / float64(a.count),
			Exportable:      supported[rule],
		})
	}

	// Sort by avg win rate descending
	for i := 1; i < len(result); i++ {
		for j := i; j > 0 && result[j].AvgWinRate > result[j-1].AvgWinRate; j-- {
			result[j], result[j-1] = result[j-1], result[j]
		}
	}

	if result == nil {
		result = []AggregatedRuleStat{}
	}
	jsonOK(w, result)
}

func (s *Server) getPaperPositions(w http.ResponseWriter, _ *http.Request) {
	if s.paperStore == nil {
		jsonOK(w, []paper.PaperPosition{})
		return
	}
	positions, err := s.paperStore.GetAllOpenPositions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if positions == nil {
		positions = []paper.PaperPosition{}
	}
	jsonOK(w, positions)
}

func (s *Server) getPaperHistory(w http.ResponseWriter, r *http.Request) {
	if s.paperStore == nil {
		jsonOK(w, []paper.PaperPosition{})
		return
	}
	limit := 100
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	positions, err := s.paperStore.GetClosedPositions(limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if positions == nil {
		positions = []paper.PaperPosition{}
	}
	jsonOK(w, positions)
}

func (s *Server) getPaperSummary(w http.ResponseWriter, r *http.Request) {
	if s.paperStore == nil {
		jsonOK(w, paper.Summary(nil, 0))
		return
	}
	open, err := s.paperStore.GetAllOpenPositions()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	closed, err := s.paperStore.GetClosedPositions(1000)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, paper.Summary(closed, len(open)))
}

// addSymbol handles POST /api/symbols — adds a new symbol to watchlist.yaml.
// Body: {"symbol":"AAPL","type":"stock","exchange":"nasdaq"}
// Note: collectors restart is required for live data collection to begin.
func (s *Server) addSymbol(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Symbol   string `json:"symbol"`
		Type     string `json:"type"`     // "crypto" | "stock"
		Exchange string `json:"exchange"` // optional
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if body.Symbol == "" || (body.Type != "crypto" && body.Type != "stock") {
		http.Error(w, "symbol required; type must be 'crypto' or 'stock'", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	wl, err := s.readWatchlistLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	entry := appconfig.SymbolEntry{
		Symbol:   strings.ToUpper(body.Symbol),
		Exchange: body.Exchange,
		Enabled:  true,
	}
	switch body.Type {
	case "crypto":
		wl.Symbols.Crypto = append(wl.Symbols.Crypto, entry)
	case "stock":
		wl.Symbols.Stocks = append(wl.Symbols.Stocks, entry)
	}

	if err := s.writeWatchlistLocked(wl); err != nil {
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// removeSymbol handles DELETE /api/symbols/{symbol} — removes a symbol from watchlist.yaml.
func (s *Server) removeSymbol(w http.ResponseWriter, r *http.Request) {
	target := strings.ToUpper(r.PathValue("symbol"))

	s.mu.Lock()
	defer s.mu.Unlock()

	wl, err := s.readWatchlistLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	found := false
	filtered := wl.Symbols.Crypto[:0]
	for _, e := range wl.Symbols.Crypto {
		if strings.ToUpper(e.Symbol) == target {
			found = true
		} else {
			filtered = append(filtered, e)
		}
	}
	wl.Symbols.Crypto = filtered

	filtered = wl.Symbols.Stocks[:0]
	for _, e := range wl.Symbols.Stocks {
		if strings.ToUpper(e.Symbol) == target {
			found = true
		} else {
			filtered = append(filtered, e)
		}
	}
	wl.Symbols.Stocks = filtered

	if !found {
		http.Error(w, "symbol not found", http.StatusNotFound)
		return
	}
	if err := s.writeWatchlistLocked(wl); err != nil {
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getReportConfig handles GET /api/report/config.
func (s *Server) getReportConfig(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cfg, err := s.readReportConfigLocked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, cfg)
}

// updateReportConfig handles PUT /api/report/config.
func (s *Server) updateReportConfig(w http.ResponseWriter, r *http.Request) {
	var cfg appconfig.DailyReportConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.writeReportConfigLocked(cfg); err != nil {
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}

	if s.reportSched != nil {
		s.reportSched.Reset(cfg)
	}

	w.WriteHeader(http.StatusNoContent)
}

// reportCfgFile returns the path to report.yaml.
func (s *Server) reportCfgFile() string {
	return s.configDir + "/report.yaml"
}

// readReportConfigLocked reads report.yaml. Caller must hold s.mu (read or write).
func (s *Server) readReportConfigLocked() (appconfig.DailyReportConfig, error) {
	cfg := appconfig.DailyReportConfig{
		Enabled:    true,
		Time:       "09:00",
		Timezone:   "Asia/Seoul",
		AIMinScore: 8.0,
	}
	f, err := os.Open(s.reportCfgFile())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("failed to read report.yaml: %w", err)
	}
	defer f.Close()
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return cfg, fmt.Errorf("failed to parse report.yaml: %w", err)
	}
	return cfg, nil
}

// writeReportConfigLocked writes report.yaml. Caller must hold s.mu (write).
func (s *Server) writeReportConfigLocked(cfg appconfig.DailyReportConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal report config: %w", err)
	}
	return os.WriteFile(s.reportCfgFile(), data, 0o644)
}

// alertCfgFile returns the path to alert.yaml.
func (s *Server) alertCfgFile() string { return s.configDir + "/alert.yaml" }

// readAlertConfigLocked reads alert.yaml. Caller must hold s.mu (read or write).
func (s *Server) readAlertConfigLocked() (appconfig.AlertConfig, error) {
	var cfg appconfig.AlertConfig
	f, err := os.Open(s.alertCfgFile())
	if err != nil {
		return cfg, err
	}
	defer f.Close()
	return cfg, yaml.NewDecoder(f).Decode(&cfg)
}

// writeAlertConfigLocked writes alert.yaml. Caller must hold s.mu (write).
func (s *Server) writeAlertConfigLocked(cfg appconfig.AlertConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(s.alertCfgFile(), data, 0o644)
}

// getAlertConfig handles GET /api/alert/config.
func (s *Server) getAlertConfig(w http.ResponseWriter, _ *http.Request) {
	if s.alertHolder == nil {
		jsonOK(w, appconfig.AlertConfig{ScoreThreshold: 12.0, CooldownHours: 4, MTFConsensusMin: 2})
		return
	}
	jsonOK(w, s.alertHolder.Get())
}

// updateAlertConfig handles PUT /api/alert/config.
func (s *Server) updateAlertConfig(w http.ResponseWriter, r *http.Request) {
	var cfg appconfig.AlertConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if cfg.ScoreThreshold <= 0 || cfg.CooldownHours <= 0 || cfg.MTFConsensusMin < 1 {
		http.Error(w, "invalid values", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writeAlertConfigLocked(cfg); err != nil {
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}
	if s.alertHolder != nil {
		s.alertHolder.Set(cfg)
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
		return wl, fmt.Errorf("failed to read watchlist: %w", err)
	}
	defer f.Close()
	return wl, yaml.NewDecoder(f).Decode(&wl)
}

// writeWatchlistLocked writes watchlist.yaml. Caller must hold s.mu (write).
func (s *Server) writeWatchlistLocked(wl appconfig.WatchlistConfig) error {
	data, err := yaml.Marshal(wl)
	if err != nil {
		return fmt.Errorf("failed to marshal watchlist: %w", err)
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
		return rc, fmt.Errorf("failed to read rules: %w", err)
	}
	defer f.Close()
	return rc, yaml.NewDecoder(f).Decode(&rc)
}

// writeRulesLocked writes rules.yaml. Caller must hold s.mu (write).
func (s *Server) writeRulesLocked(rc appconfig.RulesConfig) error {
	data, err := yaml.Marshal(rc)
	if err != nil {
		return fmt.Errorf("failed to marshal rules: %w", err)
	}
	return os.WriteFile(s.configDir+"/rules.yaml", data, 0o644)
}

// ── utilities ─────────────────────────────────────────────────────────────────

// configWriteErrorMessage returns a user-facing message for config file write failures
// (e.g. read-only volume in Docker). Call when writeWatchlistLocked, writeRulesLocked,
// writeReportConfigLocked, or writeAlertConfigLocked returns an error.
func configWriteErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	if os.IsPermission(err) || errors.Is(err, fs.ErrPermission) {
		return "cannot write to config directory (permission denied). Check that the config volume is not read-only (:ro) in Docker."
	}
	var errno *syscall.Errno
	if errors.As(err, &errno) && *errno == syscall.EROFS {
		return "config directory is read-only. Remove :ro from the Docker config volume."
	}
	return err.Error()
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// exportPineScript handles GET /api/export/pinescript?rule=<name>&win_rate=<float>&avg_rr=<float>
// Returns a plain-text Pine Script v5 file for download.
func (s *Server) exportPineScript(w http.ResponseWriter, r *http.Request) {
	rule := r.URL.Query().Get("rule")
	if rule == "" {
		http.Error(w, "rule parameter required", http.StatusBadRequest)
		return
	}

	var winRate, avgRR float64
	fmt.Sscanf(r.URL.Query().Get("win_rate"), "%f", &winRate)
	fmt.Sscanf(r.URL.Query().Get("avg_rr"), "%f", &avgRR)

	script, err := pinescript.Generate(rule, winRate, avgRR)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	filename := rule + ".pine"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(script))
}

// runFullAnalysis handles POST /api/analysis/full.
// Request body: {"symbol":"SPY","timeframe":"1D"}
// Fetches full OHLCV history, builds a history summary, computes indicators across
// all timeframes, and runs the multi-analyst AI analysis, returning a ScenarioResult.
func (s *Server) runFullAnalysis(w http.ResponseWriter, r *http.Request) {
	if s.fullStore == nil || s.analystDirector == nil {
		http.Error(w, "full analysis not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Symbol    string `json:"symbol"`
		Timeframe string `json:"timeframe"`
		Language  string `json:"language"` // "en" | "ko" | "ja"
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Symbol == "" {
		http.Error(w, "symbol is required", http.StatusBadRequest)
		return
	}
	if req.Timeframe == "" {
		req.Timeframe = "1D"
	}

	// Fetch full daily OHLCV history for history summary
	bars, err := s.fullStore.GetOHLCVAll(req.Symbol, "1D")
	if err != nil {
		http.Error(w, fmt.Sprintf("OHLCV fetch failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Build history summary (~600 tokens)
	summarizer := history.New()
	historySummary := summarizer.Summarize(req.Symbol, bars)

	// Fetch recent bars for each timeframe and compute indicators
	tfList := []string{"1H", "4H", "1D", "1W"}
	tfBars := make(map[string][]models.OHLCV, len(tfList))
	for _, tf := range tfList {
		tfData, err := s.fullStore.GetOHLCV(req.Symbol, tf, 200)
		if err != nil {
			continue
		}
		// GetOHLCV returns DESC; reverse to ASC for indicator computation
		for i, j := 0, len(tfData)-1; i < j; i, j = i+1, j-1 {
			tfData[i], tfData[j] = tfData[j], tfData[i]
		}
		tfBars[tf] = tfData
	}
	recentIndicators := indicator.Compute(tfBars)

	// Fetch recent signals and format as text
	sigs, err := s.fullStore.GetSignals(req.Symbol, 20)
	var ruleSignalText string
	if err == nil && len(sigs) > 0 {
		var sb strings.Builder
		for _, sig := range sigs {
			sb.WriteString(fmt.Sprintf("[%s] %s %s (score:%.1f) %s\n",
				sig.CreatedAt.Format("2006-01-02"),
				sig.Timeframe, sig.Direction, sig.Score, sig.Rule))
		}
		ruleSignalText = sb.String()
	}

	// Fetch SPY as S&P 500 macro backdrop for all non-SPY symbols.
	// Provides market cycle context that helps all three analysts judge
	// whether the target symbol is trading with or against the broad market.
	var macroContext string
	if req.Symbol != "SPY" && req.Symbol != "^GSPC" {
		if spyBars, err := s.fullStore.GetOHLCVAll("SPY", "1D"); err == nil && len(spyBars) > 0 {
			macroContext = summarizer.Summarize("SPY", spyBars)
		}
	}

	input := analyst.AnalystInput{
		Symbol:           req.Symbol,
		HistorySummary:   historySummary,
		MacroContext:     macroContext,
		RecentIndicators: recentIndicators,
		RuleSignalText:   ruleSignalText,
		Language:         req.Language,
	}

	result := s.analystDirector.Analyze(r.Context(), input)

	// Save result to DB
	if id, err := s.fullStore.SaveAnalysis(result); err != nil {
		log.Warn().Err(err).Msg("failed to save analysis result")
	} else {
		result.ID = id
	}

	jsonOK(w, result)
}

// getAnalysisHistory handles GET /api/analysis/history?symbol=SPY&limit=20
func (s *Server) getAnalysisHistory(w http.ResponseWriter, r *http.Request) {
	if s.fullStore == nil {
		http.Error(w, "store not configured", http.StatusServiceUnavailable)
		return
	}
	symbol := r.URL.Query().Get("symbol")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	records, err := s.fullStore.GetAnalysisHistory(symbol, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []storage.AnalysisRecord{}
	}
	jsonOK(w, records)
}

// getAnalysisDetail handles GET /api/analysis/history/{id}
func (s *Server) getAnalysisDetail(w http.ResponseWriter, r *http.Request) {
	if s.fullStore == nil {
		http.Error(w, "store not configured", http.StatusServiceUnavailable)
		return
	}
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	record, err := s.fullStore.GetAnalysisByID(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonOK(w, record)
}

// runAnalysisExport handles POST /api/analysis/export.
// Formats the ScenarioResult as an HTML message and sends it via Telegram.
func (s *Server) runAnalysisExport(w http.ResponseWriter, r *http.Request) {
	if s.announcer == nil {
		http.Error(w, "telegram not configured", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Result analyst.ScenarioResult `json:"result"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	res := req.Result
	finalEmoji := map[string]string{"BULL": "🟢", "BEAR": "🔴", "SIDEWAYS": "🟡"}[res.Final]
	confColor := map[string]string{"HIGH": "🔵", "MEDIUM": "🟣", "LOW": "⚪"}[res.Confidence]

	msg := fmt.Sprintf(
		"📊 <b>%s Analysis Result</b>\n\n"+
			"%s <b>%s</b> | %s %s\n\n"+
			"📈 BULL: <b>%.1f%%</b>\n"+
			"📉 BEAR: <b>%.1f%%</b>\n"+
			"➡️ SIDEWAYS: <b>%.1f%%</b>\n\n"+
			"<i>%s</i>",
		res.Symbol,
		finalEmoji, res.Final, confColor, res.Confidence,
		res.BullPct, res.BearPct, res.SidewaysPct,
		res.AggregatorReason,
	)

	s.announcer.Announce(r.Context(), msg)
	jsonOK(w, map[string]string{"status": "sent"})
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
