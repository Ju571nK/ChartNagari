// Package api provides the HTTP REST API server for the Chart Nagari settings UI.
package api

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "modernc.org/sqlite" // pure-Go SQLite driver for integrity_check

	"github.com/rs/zerolog/log"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/analyst"
	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/internal/history"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/paper"
	"github.com/Ju571nK/Chatter/internal/pinescript"
	wyckoffanalyzer "github.com/Ju571nK/Chatter/internal/wyckoff"
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
	WSClients      int      `json:"ws_clients"`
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
	ZoneLow          float64 `json:"zone_low,omitempty"`
	ZoneHigh         float64 `json:"zone_high,omitempty"`
	ForwardReturn5d  float64 `json:"forward_return_5d,omitempty"`
	ForwardReturn10d float64 `json:"forward_return_10d,omitempty"`
	ForwardReturn20d float64 `json:"forward_return_20d,omitempty"`
	ForwardReturn40d float64 `json:"forward_return_40d,omitempty"`
	HTFTrend         string  `json:"htf_trend,omitempty"`
	ATRPercentile    float64 `json:"atr_percentile,omitempty"`
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

// CalendarStore provides economic event data for the API.
type CalendarStore interface {
	GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error)
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

// WSHub serves WebSocket connections.
type WSHub interface {
	ServeWS(w http.ResponseWriter, r *http.Request)
	ClientCount() int
}

// PriceAlertStore manages user-defined price target alerts.
// *storage.DB satisfies this interface.
type PriceAlertStore interface {
	ListPriceAlerts() ([]storage.PriceAlert, error)
	AddPriceAlert(symbol, condition string, target float64, note string) (int64, error)
	DeletePriceAlert(id int64) error
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
	priceAlertStore  PriceAlertStore                // optional; set via WithPriceAlertStore
	wsHub            WSHub                          // optional; set via WithHub
	calendarStore    CalendarStore                  // optional; set via WithCalendarStore
	settingsFile     string                         // path to settings.yaml; set via WithSettingsFile
	demoEngine       *engine.RuleEngine             // optional; set via WithDemoEngine for /api/demo/scan
	profileHolder       *appconfig.SymbolProfilesHolder    // optional; set via WithSymbolProfiles
	signalTuningHolder  *appconfig.SignalTuningHolder      // optional; set via WithSignalTuningHolder
	dbPath              string                             // path to SQLite DB file; set via WithDBPath
	startTime           time.Time                          // server start timestamp for uptime
	dataSources         []string                           // active data sources (e.g. ["Binance","Tiingo"])
	allowedOrigins      map[string]bool                    // CORS allowlist; set via WithAllowedOrigins
	apiToken            string                             // optional bearer token; set via WithAPIToken
	execHolder          *appconfig.ExecutionHolder         // optional; set via WithExecutionHolder
	execPath            string                             // path to execution.yaml; set via WithExecutionPath
	execDispatcher      ExecutionReleaser                  // optional; set via WithExecutionDispatcher
	execFeedback        FeedbackRecorder                   // optional; set via WithExecutionFeedback
	execDB              *sql.DB                            // optional; set via WithExecutionDB for feedback queries
	execState           *execution.StateStore              // optional; set via WithExecutionState for config versioning
	ollamaDetector      OllamaStatusProvider               // optional; set via WithOllamaDetector
	ollamaPullRunner    OllamaPullRunner                   // optional; set via WithOllamaPullRunner
	ollamaStarter       OllamaStarter                      // optional; set via WithOllamaStarter
	ollamaRepoRoot      string                             // optional; set via WithOllamaRepoRoot
	mu                  sync.RWMutex
	configUpdateOnce    sync.Once                          // guards the one-shot "execState nil" startup warning
}

// ExecutionReleaser is the minimal dispatcher surface the feedback handler
// needs (Release on terminal statuses). *execution.Dispatcher satisfies this.
type ExecutionReleaser interface {
	Release()
	ActiveCount() int64
}

// FeedbackRecorder records inbound plugin feedback idempotently.
// *execution.FeedbackIdempotency satisfies this.
type FeedbackRecorder interface {
	RecordOnce(ctx context.Context, pluginID, signalID, orderID, status, symbol, message string, at time.Time) (bool, error)
}

// New creates a Server.
//   - configDir: directory containing watchlist.yaml and rules.yaml.
//   - webDist:   path to the compiled React frontend (web/dist); empty or
//     non-existent path → static serving is disabled.
func New(configDir, webDist string) *Server {
	s := &Server{
		configDir: configDir,
		startTime: time.Now(),
		allowedOrigins: map[string]bool{
			"http://localhost:5173":   true,
			"http://localhost:8080":   true,
			"http://127.0.0.1:5173":  true,
			"http://127.0.0.1:8080":  true,
		},
	}
	if webDist != "" {
		if _, err := os.Stat(webDist); err == nil {
			s.static = http.FileServer(http.Dir(webDist))
		}
	}
	return s
}

// WithExecutionHolder wires the execution config holder (Phase 2 dispatcher).
func (s *Server) WithExecutionHolder(h *appconfig.ExecutionHolder, path string) {
	s.execHolder = h
	s.execPath = path
}

// WithExecutionDispatcher wires the dispatcher so feedback can call Release().
func (s *Server) WithExecutionDispatcher(d ExecutionReleaser) {
	s.execDispatcher = d
}

// WithExecutionFeedback wires the idempotency recorder.
func (s *Server) WithExecutionFeedback(f FeedbackRecorder) {
	s.execFeedback = f
}

// WithExecutionDB wires a shared *sql.DB for feedback queries (e.g. listExecutionFeedback).
// In production this is the same db.Conn() used by FeedbackIdempotency.
func (s *Server) WithExecutionDB(db *sql.DB) {
	s.execDB = db
}

// WithExecutionState wires the key-value state store used for config versioning
// (config_version) and kill-switch metadata (killed_at).
func (s *Server) WithExecutionState(store *execution.StateStore) {
	s.execState = store
}

// WithOllamaDetector wires the Ollama status detector. When unset, the
// status endpoint returns 503.
func (s *Server) WithOllamaDetector(d OllamaStatusProvider) {
	s.ollamaDetector = d
}

// WithOllamaPullRunner wires the pull runner. When unset, the pull endpoint
// returns 503.
func (s *Server) WithOllamaPullRunner(r OllamaPullRunner) {
	s.ollamaPullRunner = r
}

// WithOllamaStarter wires the subprocess launcher. When unset, the start
// endpoint returns 503.
func (s *Server) WithOllamaStarter(st OllamaStarter) {
	s.ollamaStarter = st
}

// WithOllamaRepoRoot sets the filesystem root used by the sidecar/enable
// handler to locate the compose template and write the override file.
// In production this is the working directory of the server process.
// Tests pass t.TempDir() to avoid writing to the real repo root.
func (s *Server) WithOllamaRepoRoot(root string) {
	s.ollamaRepoRoot = root
}

// WithAllowedOrigins replaces the CORS allowed-origin set.
func (s *Server) WithAllowedOrigins(origins []string) {
	m := make(map[string]bool, len(origins))
	for _, o := range origins {
		m[o] = true
	}
	s.allowedOrigins = m
}

// WithAPIToken sets the bearer token required for mutating (non-GET) endpoints.
// When token is empty, auth is skipped for backward compatibility.
func (s *Server) WithAPIToken(token string) {
	s.apiToken = token
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

// WithPriceAlertStore wires the price alert store to the server.
func (s *Server) WithPriceAlertStore(ps PriceAlertStore) {
	s.priceAlertStore = ps
}

// WithHub wires the WebSocket hub to the server.
func (s *Server) WithHub(h WSHub) {
	s.wsHub = h
}

func (s *Server) WithCalendarStore(cs CalendarStore) {
	s.calendarStore = cs
}

// WithSymbolProfiles wires the per-symbol profile holder to the server.
func (s *Server) WithSignalTuningHolder(h *appconfig.SignalTuningHolder) {
	s.signalTuningHolder = h
}

func (s *Server) WithSymbolProfiles(h *appconfig.SymbolProfilesHolder) {
	s.profileHolder = h
}

// WithSettingsFile enables the GET/PUT /api/settings/config endpoints by pointing them at settings.yaml.
// WithDemoEngine injects the rule engine for the demo scan endpoint.
func (s *Server) WithDemoEngine(e *engine.RuleEngine) {
	s.demoEngine = e
}

func (s *Server) WithSettingsFile(path string) {
	s.settingsFile = path
}

// WithDBPath stores the SQLite database file path for backup/restore endpoints.
func (s *Server) WithDBPath(path string) {
	s.dbPath = path
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

	// WebSocket real-time push
	if s.wsHub != nil {
		mux.HandleFunc("GET /ws", s.wsHub.ServeWS)
	}

	// Status
	mux.HandleFunc("GET /api/status", s.getStatus)

	// Watchlist symbols
	mux.HandleFunc("GET /api/symbols/validate", s.validateSymbol)
	mux.HandleFunc("GET /api/symbols", s.getSymbols)
	mux.HandleFunc("POST /api/symbols", s.addSymbol)
	mux.HandleFunc("PUT /api/symbols/{symbol}", s.updateSymbol)
	mux.HandleFunc("DELETE /api/symbols/{symbol}", s.removeSymbol)

	// Analysis rules
	mux.HandleFunc("GET /api/rules", s.getRules)
	mux.HandleFunc("PUT /api/rules/{name}", s.updateRule)

	// VIX market volatility
	mux.HandleFunc("GET /api/vix/current", s.getVIXCurrent)

	// Chart dashboard data
	mux.HandleFunc("GET /api/ohlcv/{symbol}/{timeframe}", s.getChartOHLCV)
	mux.HandleFunc("GET /api/signals", s.getChartSignals)
	mux.HandleFunc("GET /api/history", s.getHistory)

	// Wyckoff phase overlay
	mux.HandleFunc("GET /api/wyckoff/{symbol}/{timeframe}", s.getWyckoffAnalysis)

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

	// Signal tuning config
	mux.HandleFunc("GET /api/signal-tuning", s.getSignalTuning)
	mux.HandleFunc("PUT /api/signal-tuning", s.updateSignalTuning)

	// settings.yaml config (only when settingsFile is set)
	if s.settingsFile != "" {
		mux.HandleFunc("GET /api/settings/config", s.getEnvConfig)
		mux.HandleFunc("PUT /api/settings/config", s.updateEnvConfig)
		// backward-compat: old /api/env/config route
		mux.HandleFunc("GET /api/env/config", s.getEnvConfig)
		mux.HandleFunc("PUT /api/env/config", s.updateEnvConfig)
	}

	// Price target alerts
	if s.priceAlertStore != nil {
		mux.HandleFunc("GET /api/price-alerts", s.listPriceAlerts)
		mux.HandleFunc("POST /api/price-alerts", s.createPriceAlert)
		mux.HandleFunc("DELETE /api/price-alerts/{id}", s.deletePriceAlert)
	}

	// Symbol profiles
	if s.profileHolder != nil {
		mux.HandleFunc("GET /api/profiles", s.getProfiles)
		mux.HandleFunc("GET /api/profiles/{symbol}", s.getSymbolProfile)
		mux.HandleFunc("PUT /api/profiles/{symbol}", s.updateSymbolProfile)
	}

	// Economic calendar
	if s.calendarStore != nil {
		mux.HandleFunc("GET /api/calendar", s.getCalendarEvents)
	}

	// Demo scan (no DB required — runs rule engine on sample data)
	if s.demoEngine != nil {
		mux.HandleFunc("GET /api/demo/scan", s.demoScan)
	}

	// Signal CSV export
	mux.HandleFunc("GET /api/signals/export", s.exportSignalsCSV)

	// DB backup & restore
	mux.HandleFunc("GET /api/backup/db", s.backupDB)
	mux.HandleFunc("POST /api/restore/db", s.restoreDB)

	// Settings export & import
	mux.HandleFunc("GET /api/settings/export", s.exportSettings)
	mux.HandleFunc("POST /api/settings/import", s.importSettings)

	// Multi-analyst full analysis
	mux.HandleFunc("POST /api/analysis/full", s.runFullAnalysis)
	mux.HandleFunc("POST /api/analysis/export", s.runAnalysisExport)
	mux.HandleFunc("GET /api/analysis/history", s.getAnalysisHistory)
	mux.HandleFunc("GET /api/analysis/history/{id}", s.getAnalysisDetail)

	// Execution dispatcher (Phase 2).
	if s.execHolder != nil {
		mux.HandleFunc("GET /api/execution/config", s.getExecutionConfig)
		mux.HandleFunc("PUT /api/execution/config", s.updateExecutionConfig)
		mux.HandleFunc("POST /api/execution/kill", s.toggleExecutionKill)
		mux.HandleFunc("POST /api/execution/feedback", s.postExecutionFeedback)
		mux.HandleFunc("GET /api/execution/feedback", s.listExecutionFeedback)
		mux.HandleFunc("GET /api/execution/plugins/stats", s.getExecutionPluginStats)
	}

	// Ollama local-LLM status (detection state machine)
	mux.HandleFunc("GET /api/ai/ollama/status", s.getOllamaStatus)
	mux.HandleFunc("POST /api/ai/ollama/pull", s.pullOllamaModel)
	mux.HandleFunc("POST /api/ai/ollama/start", s.startOllama)
	mux.HandleFunc("POST /api/ai/ollama/sidecar/enable", s.enableOllamaSidecar)

	// Static frontend (SPA)
	if s.static != nil {
		mux.Handle("/", s.static)
	}

	return s.corsMiddleware(s.authMiddleware(mux))
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

	var wsClients int
	if s.wsHub != nil {
		wsClients = s.wsHub.ClientCount()
	}

	jsonOK(w, StatusItem{
		Phase:          "Phase 2: Enhancement",
		Symbols:        total,
		Rules:          len(rc.Rules),
		UptimeSec:      int64(time.Since(s.startTime).Seconds()),
		LastSignalUnix: lastSignal,
		DataSources:    sources,
		WSClients:      wsClients,
	})
}

// CoiledResponse is the nested coiled market state in VIXResponse.
type CoiledResponse struct {
	IsCoiled    bool    `json:"is_coiled"`
	RealizedVol float64 `json:"realized_vol"`
	ImpliedVol  float64 `json:"implied_vol"`
	Ratio       float64 `json:"ratio"`
}

// VIXResponse is the JSON structure for the /api/vix/current endpoint.
type VIXResponse struct {
	Current   float64         `json:"current"`
	Avg20d    float64         `json:"avg_20d"`
	Trend     string          `json:"trend"` // "rising" | "falling"
	Available bool            `json:"available"`
	Coiled    *CoiledResponse `json:"coiled,omitempty"`
}

func (s *Server) getVIXCurrent(w http.ResponseWriter, _ *http.Request) {
	if s.chartStore == nil {
		jsonOK(w, VIXResponse{Available: false})
		return
	}

	bars, err := s.chartStore.GetOHLCV("^VIX", "1D", 30)
	if err != nil || len(bars) == 0 {
		jsonOK(w, VIXResponse{Available: false})
		return
	}

	// bars are newest-first (DESC order from DB)
	current := bars[0].Close

	// Calculate 20-day simple moving average of close prices
	n := len(bars)
	if n > 20 {
		n = 20
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += bars[i].Close
	}
	avg20d := sum / float64(n)

	trend := "falling"
	if current > avg20d {
		trend = "rising"
	}

	resp := VIXResponse{
		Current:   math.Round(current*100) / 100,
		Avg20d:    math.Round(avg20d*100) / 100,
		Trend:     trend,
		Available: true,
	}

	// Compute coiled market state using realized volatility from SPY (or any broad market proxy).
	// We use SPY 1D closes to compute 20-period realized vol, then compare to VIX.
	spyBars, spyErr := s.chartStore.GetOHLCV("SPY", "1D", 30)
	if spyErr == nil && len(spyBars) >= 21 {
		// spyBars are in DESC order; reverse for indicator computation (needs ASC).
		closes := make([]float64, len(spyBars))
		for i, b := range spyBars {
			closes[len(spyBars)-1-i] = b.Close
		}
		rv := indicator.ComputeRealizedVol(closes, 20)
		if rv > 0 {
			ratio := 0.0
			if current > 0 {
				ratio = rv / current
			}
			threshold := 0.70 // default
			if s.signalTuningHolder != nil {
				tc := s.signalTuningHolder.Get()
				threshold = float64(tc.CoiledMarket.RatioThreshold) / 100.0
			}
			resp.Coiled = &CoiledResponse{
				IsCoiled:    ratio > 0 && ratio < threshold,
				RealizedVol: math.Round(rv*10) / 10,
				ImpliedVol:  resp.Current,
				Ratio:       math.Round(ratio*100) / 100,
			}
		}
	}

	jsonOK(w, resp)
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
			ZoneLow:          sig.ZoneLow,
			ZoneHigh:         sig.ZoneHigh,
			ForwardReturn5d:  sig.ForwardReturn5d,
			ForwardReturn10d: sig.ForwardReturn10d,
			ForwardReturn20d: sig.ForwardReturn20d,
			ForwardReturn40d: sig.ForwardReturn40d,
			HTFTrend:         sig.HTFTrend,
			ATRPercentile:    sig.ATRPercentile,
		}
	}
	jsonOK(w, result)
}

// getWyckoffAnalysis handles GET /api/wyckoff/{symbol}/{timeframe}.
// It loads OHLCV bars, runs the Wyckoff phase analyzer, and returns the
// full overlay payload (phase zones, spring/upthrust events, swing levels).
// Query param: limit (default 300).
func (s *Server) getWyckoffAnalysis(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		jsonOK(w, wyckoffanalyzer.Analysis{})
		return
	}
	symbol := r.PathValue("symbol")
	timeframe := r.PathValue("timeframe")
	limit := 300
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	bars, err := s.chartStore.GetOHLCV(symbol, timeframe, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// DB returns newest-first; analyzer expects oldest-first.
	for i, j := 0, len(bars)-1; i < j; i, j = i+1, j-1 {
		bars[i], bars[j] = bars[j], bars[i]
	}

	analysis := wyckoffanalyzer.Analyze(symbol, timeframe, bars)
	jsonOK(w, analysis)
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
			ZoneLow:          sig.ZoneLow,
			ZoneHigh:         sig.ZoneHigh,
			ForwardReturn5d:  sig.ForwardReturn5d,
			ForwardReturn10d: sig.ForwardReturn10d,
			ForwardReturn20d: sig.ForwardReturn20d,
			ForwardReturn40d: sig.ForwardReturn40d,
			HTFTrend:         sig.HTFTrend,
			ATRPercentile:    sig.ATRPercentile,
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
	w.WriteHeader(http.StatusNoContent)
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

// signalTuningFile returns the path to signal_tuning.yaml.
func (s *Server) signalTuningFile() string { return s.configDir + "/signal_tuning.yaml" }

// getSignalTuning handles GET /api/signal-tuning.
func (s *Server) getSignalTuning(w http.ResponseWriter, _ *http.Request) {
	if s.signalTuningHolder == nil {
		jsonOK(w, appconfig.DefaultSignalTuning())
		return
	}
	jsonOK(w, s.signalTuningHolder.Get())
}

// updateSignalTuning handles PUT /api/signal-tuning.
func (s *Server) updateSignalTuning(w http.ResponseWriter, r *http.Request) {
	var cfg appconfig.SignalTuningConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	// Validate ranges
	if cfg.HTFFilter.CounterTrendPenaltyPct < 0 || cfg.HTFFilter.CounterTrendPenaltyPct > 100 {
		http.Error(w, "counter_trend_penalty_pct must be 0-100", http.StatusBadRequest)
		return
	}
	if cfg.VolatilityRegime.LowVolPercentile < 0 || cfg.VolatilityRegime.LowVolPercentile > 100 ||
		cfg.VolatilityRegime.HighVolPercentile < 0 || cfg.VolatilityRegime.HighVolPercentile > 100 {
		http.Error(w, "percentile values must be 0-100", http.StatusBadRequest)
		return
	}
	if cfg.VolatilityRegime.LowVolPenaltyPct < 0 || cfg.VolatilityRegime.LowVolPenaltyPct > 100 ||
		cfg.VolatilityRegime.HighVolBonusPct < 0 || cfg.VolatilityRegime.HighVolBonusPct > 100 {
		http.Error(w, "penalty/bonus values must be 0-100", http.StatusBadRequest)
		return
	}
	if cfg.ATRSlope.EMAPeriod < 1 {
		http.Error(w, "ema_period must be >= 1", http.StatusBadRequest)
		return
	}
	if cfg.ATRSlope.RisingBonusPct < 0 || cfg.ATRSlope.RisingBonusPct > 100 {
		http.Error(w, "rising_bonus_pct must be 0-100", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := appconfig.SaveSignalTuning(s.signalTuningFile(), cfg); err != nil {
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}
	if s.signalTuningHolder != nil {
		s.signalTuningHolder.Set(cfg)
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

// ── symbol profile handlers ──────────────────────────────────────────────────

// ProfileItem is the JSON representation of a profile.
type ProfileItem struct {
	Name                 string   `json:"name"`
	AllowedMethodologies []string `json:"allowed_methodologies"`
	BlockedMethodologies []string `json:"blocked_methodologies"`
	AllowedRules         []string `json:"allowed_rules"`
	AlertLimitPerDay     int      `json:"alert_limit_per_day"`
	CooldownHours        int      `json:"cooldown_hours"`
	ScoreThreshold       float64  `json:"score_threshold"`
}

// SymbolProfileResponse is the response for GET /api/profiles/{symbol}.
type SymbolProfileResponse struct {
	Symbol  string      `json:"symbol"`
	Profile string      `json:"profile"`
	Detail  ProfileItem `json:"detail"`
}

func (s *Server) getProfiles(w http.ResponseWriter, _ *http.Request) {
	cfg := s.profileHolder.Get()
	items := make([]ProfileItem, 0, len(cfg.Profiles))
	for name, p := range cfg.Profiles {
		items = append(items, ProfileItem{
			Name:                 name,
			AllowedMethodologies: p.AllowedMethodologies,
			BlockedMethodologies: p.BlockedMethodologies,
			AllowedRules:         p.AllowedRules,
			AlertLimitPerDay:     p.AlertLimitPerDay,
			CooldownHours:        p.CooldownHours,
			ScoreThreshold:       p.ScoreThreshold,
		})
	}
	jsonOK(w, items)
}

func (s *Server) getSymbolProfile(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")
	profileName := s.profileHolder.GetProfileName(symbol)
	profile := s.profileHolder.GetProfile(symbol)
	jsonOK(w, SymbolProfileResponse{
		Symbol:  symbol,
		Profile: profileName,
		Detail: ProfileItem{
			Name:                 profileName,
			AllowedMethodologies: profile.AllowedMethodologies,
			BlockedMethodologies: profile.BlockedMethodologies,
			AllowedRules:         profile.AllowedRules,
			AlertLimitPerDay:     profile.AlertLimitPerDay,
			CooldownHours:        profile.CooldownHours,
			ScoreThreshold:       profile.ScoreThreshold,
		},
	})
}

func (s *Server) updateSymbolProfile(w http.ResponseWriter, r *http.Request) {
	symbol := r.PathValue("symbol")

	var body struct {
		Profile string `json:"profile"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	// Validate that the profile exists.
	cfg := s.profileHolder.Get()
	if _, ok := cfg.Profiles[body.Profile]; !ok {
		http.Error(w, fmt.Sprintf("unknown profile: %s", body.Profile), http.StatusBadRequest)
		return
	}

	s.profileHolder.SetSymbolProfile(symbol, body.Profile)

	// Persist to disk.
	updatedCfg := s.profileHolder.Get()
	if err := appconfig.SaveSymbolProfiles(s.configDir+"/symbol_profiles.yaml", updatedCfg); err != nil {
		log.Warn().Err(err).Str("symbol", symbol).Msg("failed to persist symbol profile change")
		http.Error(w, configWriteErrorMessage(err), http.StatusInternalServerError)
		return
	}

	log.Info().Str("symbol", symbol).Str("profile", body.Profile).Msg("symbol profile updated")
	w.WriteHeader(http.StatusNoContent)
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

// validateSymbol handles GET /api/symbols/validate?symbol=BTCUSDT
// Checks Binance (crypto) then Yahoo Finance (stock) and returns type + exchange.
func (s *Server) validateSymbol(w http.ResponseWriter, r *http.Request) {
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if symbol == "" {
		http.Error(w, "symbol query param required", http.StatusBadRequest)
		return
	}
	type ValidateResult struct {
		Found    bool   `json:"found"`
		Type     string `json:"type,omitempty"`
		Exchange string `json:"exchange,omitempty"`
		Name     string `json:"name,omitempty"`
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if exch, name, ok := checkBinanceSymbol(ctx, symbol); ok {
		jsonOK(w, ValidateResult{Found: true, Type: "crypto", Exchange: exch, Name: name})
		return
	}
	if exch, name, ok := checkYahooSymbol(ctx, symbol); ok {
		jsonOK(w, ValidateResult{Found: true, Type: "stock", Exchange: exch, Name: name})
		return
	}
	jsonOK(w, ValidateResult{Found: false})
}

func checkBinanceSymbol(ctx context.Context, symbol string) (string, string, bool) {
	url := "https://api.binance.com/api/v3/ticker/price?symbol=" + symbol
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	defer resp.Body.Close()
	var result struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || result.Symbol == "" {
		return "", "", false
	}
	return "binance", result.Symbol, true
}

func normalizeExchange(fullExchangeName string) string {
	switch fullExchangeName {
	case "NasdaqGS", "NasdaqGM", "NasdaqCM", "NASDAQ":
		return "nasdaq"
	case "NYSE", "NYQ":
		return "nyse"
	case "AMEX", "NYSEArca", "PCX":
		return "amex"
	case "KSC", "KOE":
		return "kospi"
	case "JPX", "OSA", "TYO":
		return "tse"
	case "JASDAQ":
		return "jasdaq"
	case "LSE":
		return "lse"
	default:
		return strings.ToLower(fullExchangeName)
	}
}

func checkYahooSymbol(ctx context.Context, symbol string) (string, string, bool) {
	url := "https://query1.finance.yahoo.com/v8/finance/chart/" + symbol + "?interval=1d&range=1d"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", false
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		return "", "", false
	}
	defer resp.Body.Close()
	var result struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol          string `json:"symbol"`
					FullExchangeName string `json:"fullExchangeName"`
					LongName        string `json:"longName"`
					ShortName       string `json:"shortName"`
				} `json:"meta"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", false
	}
	if len(result.Chart.Result) == 0 || result.Chart.Error != nil {
		return "", "", false
	}
	meta := result.Chart.Result[0].Meta
	name := meta.LongName
	if name == "" {
		name = meta.ShortName
	}
	return normalizeExchange(meta.FullExchangeName), name, true
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

// ── settings.yaml config ──────────────────────────────────────────────────────

// envSentinel is returned for sensitive fields that are set, so the actual value
// is never echoed back to the browser. The client sends it back unchanged to mean "keep as-is".
const envSentinel = "__configured__"

// envSensitiveKeys lists setting keys whose values are masked in GET responses.
var envSensitiveKeys = map[string]bool{
	"TELEGRAM_BOT_TOKEN":   true,
	"DISCORD_WEBHOOK_URL":  true,
	"TIINGO_API_KEY":       true,
	"BINANCE_API_KEY":      true,
	"BINANCE_SECRET_KEY":   true,
	"ALPHAVANTAGE_API_KEY": true,
	"FINNHUB_API_KEY":      true,
	"FMP_API_KEY":          true,
	"ANTHROPIC_API_KEY":    true,
	"OPENAI_API_KEY":       true,
	"GROQ_API_KEY":         true,
	"GEMINI_API_KEY":       true,
	"API_TOKEN":            true,
}

// envExposedKeys is the ordered list of setting keys exposed via the API.
var envExposedKeys = []string{
	"ENV", "SERVER_HOST", "SERVER_PORT", "LOG_LEVEL", "ALERT_COOLDOWN_HOURS", "API_TOKEN",
	"TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID", "DISCORD_WEBHOOK_URL",
	"TIINGO_API_KEY", "TIINGO_POLL_INTERVAL",
	"YAHOO_POLL_INTERVAL",
	"BINANCE_API_KEY", "BINANCE_SECRET_KEY",
	"ALPHAVANTAGE_API_KEY",
	"FINNHUB_API_KEY", "FMP_API_KEY", "CALENDAR_ALERT_WINDOW",
	"LLM_PROVIDER",
	"ANTHROPIC_API_KEY", "OPENAI_API_KEY", "GROQ_API_KEY", "GEMINI_API_KEY",
	"AI_MIN_SCORE", "LLM_LANGUAGE",
}

// getEnvConfig handles GET /api/settings/config.
// Returns all exposed setting keys; sensitive keys that are set are replaced with envSentinel.
func (s *Server) getEnvConfig(w http.ResponseWriter, _ *http.Request) {
	settings := s.readSettingsFile()
	flat := settings.ToMap()
	out := make(map[string]string, len(envExposedKeys))
	for _, k := range envExposedKeys {
		v := flat[k]
		if envSensitiveKeys[k] && v != "" {
			v = envSentinel
		}
		out[k] = v
	}
	jsonOK(w, out)
}

// updateEnvConfig handles PUT /api/settings/config.
// Reads the current settings.yaml, applies non-sentinel updates, and writes the file back.
// Fields set to envSentinel are left unchanged (client means "keep existing").
func (s *Server) updateEnvConfig(w http.ResponseWriter, r *http.Request) {
	var updates map[string]string
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	allowed := make(map[string]bool, len(envExposedKeys))
	for _, k := range envExposedKeys {
		allowed[k] = true
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	settings := s.readSettingsFile()

	filtered := make(map[string]string)
	for k, v := range updates {
		if !allowed[k] || v == envSentinel {
			continue
		}
		filtered[k] = v
	}
	settings.ApplyMap(filtered)

	if err := appconfig.SaveSettings(s.settingsFile, settings); err != nil {
		http.Error(w, "failed to write settings.yaml: "+err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "saved — restart the server to apply changes"})
}

// readSettingsFile loads settings.yaml into a SettingsYAML struct.
func (s *Server) readSettingsFile() *appconfig.SettingsYAML {
	settings, _ := appconfig.LoadSettings(s.settingsFile)
	if settings == nil {
		settings = &appconfig.SettingsYAML{}
	}
	return settings
}

// ── Price Alerts ──────────────────────────────────────────────────────────────

func (s *Server) listPriceAlerts(w http.ResponseWriter, _ *http.Request) {
	alerts, err := s.priceAlertStore.ListPriceAlerts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if alerts == nil {
		alerts = []storage.PriceAlert{}
	}
	jsonOK(w, alerts)
}

func (s *Server) createPriceAlert(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Symbol    string  `json:"symbol"`
		Target    float64 `json:"target"`
		Condition string  `json:"condition"` // "above" | "below"
		Note      string  `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if req.Symbol == "" || req.Target <= 0 {
		http.Error(w, "symbol and target are required", http.StatusBadRequest)
		return
	}
	if req.Condition != "above" && req.Condition != "below" {
		req.Condition = "above"
	}
	id, err := s.priceAlertStore.AddPriceAlert(req.Symbol, req.Condition, req.Target, req.Note)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]int64{"id": id})
}

func (s *Server) deletePriceAlert(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := s.priceAlertStore.DeletePriceAlert(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"message": "deleted"})
}

// ── Economic Calendar ─────────────────────────────────────────────────────────

func (s *Server) getCalendarEvents(w http.ResponseWriter, r *http.Request) {
	fromStr := r.URL.Query().Get("from") // YYYY-MM-DD
	toStr := r.URL.Query().Get("to")

	now := time.Now()
	from := now.AddDate(0, 0, -1)
	to := now.AddDate(0, 0, 14)

	if fromStr != "" {
		if t, err := time.Parse("2006-01-02", fromStr); err == nil {
			from = t
		}
	}
	if toStr != "" {
		if t, err := time.Parse("2006-01-02", toStr); err == nil {
			to = t.Add(24 * time.Hour) // inclusive
		}
	}

	events, err := s.calendarStore.GetEconomicEvents(from, to)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events == nil {
		events = []storage.EconomicEvent{}
	}
	jsonOK(w, events)
}

// corsMiddleware adds CORS headers and handles OPTIONS preflight requests.
// Only origins in s.allowedOrigins receive the Access-Control-Allow-Origin header;
// unrecognised origins get no CORS header (browser will block them).
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Always advertise that the response varies by Origin so caches don't
		// serve a response with one Origin's CORS header to a different Origin.
		w.Header().Set("Vary", "Origin")
		if origin != "" && s.allowedOrigins[origin] {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// authMiddleware enforces bearer-token authentication on mutating (non-GET, non-OPTIONS)
// requests when s.apiToken is non-empty. GET and OPTIONS requests are always allowed so
// that the frontend can read data without auth, preserving backward compatibility.
// When s.apiToken is empty the middleware is a no-op, keeping full backward compatibility
// for users who have not configured a token.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.apiToken == "" || r.Method == http.MethodGet || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}
		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(authHeader) <= len(prefix) || authHeader[:len(prefix)] != prefix {
			http.Error(w, "unauthorized: missing or malformed Authorization header", http.StatusUnauthorized)
			return
		}
		token := authHeader[len(prefix):]
		if token != s.apiToken {
			http.Error(w, "unauthorized: invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// demoScan runs the rule engine on built-in sample data and returns signals.
// No DB, no API key, no watchlist required. Used by the onboarding shadow mode
// so visitors can see what ChartNagari does before adding their own symbols.
//
// GET /api/demo/scan?symbol=DEMO_BTC&timeframe=1D
func (s *Server) demoScan(w http.ResponseWriter, r *http.Request) {
	symbol := r.URL.Query().Get("symbol")
	if symbol == "" {
		symbol = "DEMO_BTC"
	}
	tf := r.URL.Query().Get("timeframe")
	if tf == "" {
		tf = "1D"
	}

	// Generate sample OHLCV data across multiple timeframes for richer signal detection.
	// Primary TF gets 100 bars; other TFs get proportional amounts.
	allBars := map[string][]models.OHLCV{}
	for _, demTF := range []string{"1W", "1D", "4H", "1H"} {
		count := 100
		switch demTF {
		case "1W":
			count = 50
		case "1D":
			count = 100
		case "4H":
			count = 200
		case "1H":
			count = 200
		}
		allBars[demTF] = generateDemoBars(symbol, demTF, count)
	}
	bars := allBars[tf]
	indicators := indicator.Compute(allBars)

	ctx := models.AnalysisContext{
		Symbol:     symbol,
		Timeframes: allBars,
		Indicators: indicators,
	}

	// Run rule engine
	signals := s.demoEngine.Run(ctx)

	// Convert bars to chart-friendly format
	type chartBar struct {
		Time   int64   `json:"time"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	}
	chartBars := make([]chartBar, len(bars))
	for i, b := range bars {
		chartBars[i] = chartBar{
			Time:   b.OpenTime.Unix(),
			Open:   b.Open,
			High:   b.High,
			Low:    b.Low,
			Close:  b.Close,
			Volume: b.Volume,
		}
	}

	resp := struct {
		Symbol    string         `json:"symbol"`
		Timeframe string         `json:"timeframe"`
		Bars      []chartBar     `json:"bars"`
		Signals   []models.Signal `json:"signals"`
	}{
		Symbol:    symbol,
		Timeframe: tf,
		Bars:      chartBars,
		Signals:   signals,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// generateDemoBars creates realistic OHLCV sample data for demo purposes.
// Simulates a Wyckoff-like accumulation → markup → distribution pattern.
func generateDemoBars(symbol, timeframe string, count int) []models.OHLCV {
	bars := make([]models.OHLCV, count)
	baseTime := time.Now().AddDate(0, 0, -count)

	var tfDuration time.Duration
	switch timeframe {
	case "1H":
		tfDuration = time.Hour
	case "4H":
		tfDuration = 4 * time.Hour
	case "1W":
		tfDuration = 7 * 24 * time.Hour
	default: // "1D"
		tfDuration = 24 * time.Hour
	}

	// Start price and phases
	price := 40000.0
	baseVolume := 1000.0

	// Simple deterministic pseudo-random using a seed
	seed := uint64(42)
	nextRand := func() float64 {
		seed = seed*6364136223846793005 + 1442695040888963407
		return float64(seed>>33) / float64(1<<31) // 0.0 to 1.0
	}

	for i := 0; i < count; i++ {
		phase := float64(i) / float64(count)
		var trend float64

		// Wyckoff-like cycle with more volatile swings to trigger ICT/Wyckoff rules
		switch {
		case phase < 0.25:
			// Accumulation: range-bound, low volume
			trend = -0.002 + math.Sin(float64(i)*0.5)*0.003
			baseVolume = 800
		case phase < 0.55:
			// Markup: strong uptrend with pullbacks every ~8 bars
			trend = 0.012
			if i%8 == 0 {
				trend = -0.008 // pullback creates swing lows
			}
			baseVolume = 1800
		case phase < 0.75:
			// Distribution: choppy at top with false breakouts
			trend = 0.002 + math.Sin(float64(i)*0.8)*0.006
			baseVolume = 2200
		default:
			// Markdown: strong downtrend with dead cat bounces
			trend = -0.010
			if i%7 == 0 {
				trend = 0.006 // dead cat bounce
			}
			baseVolume = 1500
		}

		// Spring: sharp dip below range then recovery (triggers liquidity sweep)
		if i == int(float64(count)*0.23) {
			trend = -0.035 // sharp dip below support
			baseVolume = 3000
		}
		if i == int(float64(count)*0.24) {
			trend = 0.04 // strong recovery with high volume
			baseVolume = 3500
		}

		// FVG event: gap up during markup (bar[i].high < bar[i+2].low needs big move)
		if i == int(float64(count)*0.35) {
			trend = 0.025 // strong bullish impulse creates gap
			baseVolume = 2500
		}

		// Upthrust: spike above range then reversal (distribution phase)
		if i == int(float64(count)*0.65) {
			trend = 0.03
			baseVolume = 2800
		}
		if i == int(float64(count)*0.66) {
			trend = -0.035
			baseVolume = 3000
		}

		// Generate OHLCV with wider wicks for more realistic candles
		volatility := 0.02
		change := trend + (nextRand()-0.5)*volatility
		open := price
		close := price * (1 + change)

		wickUp := nextRand() * 0.015
		wickDown := nextRand() * 0.015
		high := math.Max(open, close) * (1 + wickUp)
		low := math.Min(open, close) * (1 - wickDown)

		vol := baseVolume * (0.6 + nextRand()*0.8)

		bars[i] = models.OHLCV{
			Symbol:    symbol,
			Timeframe: timeframe,
			OpenTime:  baseTime.Add(time.Duration(i) * tfDuration),
			Open:      math.Round(open*100) / 100,
			High:      math.Round(high*100) / 100,
			Low:       math.Round(low*100) / 100,
			Close:     math.Round(close*100) / 100,
			Volume:    math.Round(vol),
		}
		price = close
	}

	return bars
}

// ── Signal CSV Export ────────────────────────────────────────────────────────

// exportSignalsCSV handles GET /api/signals/export?format=csv&symbol=X&direction=Y&limit=N.
func (s *Server) exportSignalsCSV(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", `attachment; filename="signals_export.csv"`)
		cw := csv.NewWriter(w)
		cw.Write([]string{"time", "symbol", "timeframe", "rule", "direction", "score", "entry_price", "tp", "sl", "zone_low", "zone_high", "forward_return_5d", "forward_return_10d", "forward_return_20d", "forward_return_40d", "ai_interpretation"}) //nolint:errcheck
		cw.Flush()
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
	limit := 1000
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	sigs, err := s.chartStore.GetSignalsFiltered(symbol, direction, limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="signals_export.csv"`)

	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row
	cw.Write([]string{"time", "symbol", "timeframe", "rule", "direction", "score", "entry_price", "tp", "sl", "zone_low", "zone_high", "forward_return_5d", "forward_return_10d", "forward_return_20d", "forward_return_40d", "ai_interpretation"}) //nolint:errcheck

	for _, sig := range sigs {
		cw.Write([]string{
			sig.CreatedAt.Format(time.RFC3339),
			sig.Symbol,
			sig.Timeframe,
			sig.Rule,
			sig.Direction,
			strconv.FormatFloat(sig.Score, 'f', 2, 64),
			strconv.FormatFloat(sig.EntryPrice, 'f', 6, 64),
			strconv.FormatFloat(sig.TP, 'f', 6, 64),
			strconv.FormatFloat(sig.SL, 'f', 6, 64),
			strconv.FormatFloat(sig.ZoneLow, 'f', 6, 64),
			strconv.FormatFloat(sig.ZoneHigh, 'f', 6, 64),
			strconv.FormatFloat(sig.ForwardReturn5d, 'f', 2, 64),
			strconv.FormatFloat(sig.ForwardReturn10d, 'f', 2, 64),
			strconv.FormatFloat(sig.ForwardReturn20d, 'f', 2, 64),
			strconv.FormatFloat(sig.ForwardReturn40d, 'f', 2, 64),
			sig.AIInterpretation,
		}) //nolint:errcheck
	}
}

// ── DB Backup & Restore ─────────────────────────────────────────────────────

// backupDB handles GET /api/backup/db — streams the SQLite file as a download.
func (s *Server) backupDB(w http.ResponseWriter, _ *http.Request) {
	if s.dbPath == "" {
		http.Error(w, "database path not configured", http.StatusServiceUnavailable)
		return
	}

	f, err := os.Open(s.dbPath)
	if err != nil {
		http.Error(w, "failed to open database: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	filename := fmt.Sprintf("chartnagari_backup_%s.db", time.Now().Format("20060102"))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))

	if _, err := io.Copy(w, f); err != nil {
		log.Error().Err(err).Msg("backup DB streaming failed")
	}
}

// restoreDB handles POST /api/restore/db — replaces the SQLite file with the uploaded one.
func (s *Server) restoreDB(w http.ResponseWriter, r *http.Request) {
	if s.dbPath == "" {
		http.Error(w, "database path not configured", http.StatusServiceUnavailable)
		return
	}

	// Parse multipart; limit to 512 MB.
	if err := r.ParseMultipartForm(512 << 20); err != nil {
		http.Error(w, "failed to parse upload: "+err.Error(), http.StatusBadRequest)
		return
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "missing 'file' field: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Backup existing DB to .bak
	bakPath := s.dbPath + ".bak"
	if err := copyFile(s.dbPath, bakPath); err != nil {
		log.Warn().Err(err).Msg("failed to create .bak before restore (may not exist yet)")
	}

	// Write uploaded file to a temp file, then rename for atomicity.
	tmpPath := s.dbPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		http.Error(w, "failed to create temp file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if _, err := io.Copy(out, file); err != nil {
		out.Close()
		os.Remove(tmpPath)
		http.Error(w, "failed to write uploaded file: "+err.Error(), http.StatusInternalServerError)
		return
	}
	out.Close()

	// Validate the uploaded file before replacing the live database.
	// Step 1: verify the SQLite file magic (first 16 bytes).
	const sqliteMagic = "SQLite format 3\x00"
	if err := validateSQLiteMagic(tmpPath, sqliteMagic); err != nil {
		os.Remove(tmpPath)
		http.Error(w, "invalid database file: "+err.Error(), http.StatusBadRequest)
		return
	}
	// Step 2: open with the driver and run PRAGMA integrity_check.
	if err := validateSQLiteIntegrity(tmpPath); err != nil {
		os.Remove(tmpPath)
		http.Error(w, "database integrity check failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := os.Rename(tmpPath, s.dbPath); err != nil {
		http.Error(w, "failed to replace database: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]string{
		"status":  "ok",
		"message": "Database restored. Please restart the server.",
	})
}

// validateSQLiteMagic reads the first 16 bytes of path and checks for the SQLite magic string.
var errNotSQLite = errors.New("file does not begin with SQLite format 3 magic bytes")

func validateSQLiteMagic(path, magic string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 16)
	if _, err := io.ReadFull(f, buf); err != nil {
		return fmt.Errorf("file too small to be a valid SQLite database: %w", err)
	}
	if string(buf) != magic {
		return errNotSQLite
	}
	return nil
}

// validateSQLiteIntegrity opens the SQLite file and runs PRAGMA integrity_check.
// Returns an error if the database is corrupt or cannot be opened.
func validateSQLiteIntegrity(path string) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("cannot open uploaded database: %w", err)
	}
	defer db.Close()
	row := db.QueryRow("PRAGMA integrity_check")
	var result string
	if err := row.Scan(&result); err != nil {
		return fmt.Errorf("integrity_check query failed: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check reported: %s", result)
	}
	return nil
}

// copyFile copies src to dst (used for .bak backups).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

// ── Settings Export & Import ─────────────────────────────────────────────────

// exportSettings handles GET /api/settings/export — bundles config YAML files into JSON.
func (s *Server) exportSettings(w http.ResponseWriter, _ *http.Request) {
	files := map[string]string{
		"watchlist.yaml":       filepath.Join(s.configDir, "watchlist.yaml"),
		"rules.yaml":           filepath.Join(s.configDir, "rules.yaml"),
		"signal_tuning.yaml":   filepath.Join(s.configDir, "signal_tuning.yaml"),
		"symbol_profiles.yaml": filepath.Join(s.configDir, "symbol_profiles.yaml"),
	}

	result := make(map[string]string, len(files))
	for key, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				result[key] = ""
				continue
			}
			http.Error(w, fmt.Sprintf("failed to read %s: %v", key, err), http.StatusInternalServerError)
			return
		}
		result[key] = string(data)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="chartnagari_settings.json"`)
	json.NewEncoder(w).Encode(result) //nolint:errcheck
}

// importSettings handles POST /api/settings/import — restores config YAML files from JSON.
func (s *Server) importSettings(w http.ResponseWriter, r *http.Request) {
	var payload map[string]string
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	allowedFiles := map[string]bool{
		"watchlist.yaml":       true,
		"rules.yaml":           true,
		"signal_tuning.yaml":   true,
		"symbol_profiles.yaml": true,
	}

	for key, content := range payload {
		if !allowedFiles[key] {
			continue
		}
		if content == "" {
			continue
		}
		filePath := filepath.Join(s.configDir, key)

		// Backup existing file to .bak
		if _, err := os.Stat(filePath); err == nil {
			_ = copyFile(filePath, filePath+".bak")
		}

		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			http.Error(w, fmt.Sprintf("failed to write %s: %v", key, err), http.StatusInternalServerError)
			return
		}
	}

	jsonOK(w, map[string]string{
		"status":  "ok",
		"message": "Settings imported successfully.",
	})
}
