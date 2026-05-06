package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// WatchlistSource returns the currently-configured watchlist. *appconfig.Holder
// patterns in the main binary implement this interface.
type WatchlistSource interface {
	Watchlist() appconfig.WatchlistConfig
}

// ListWatchlistTool is the read-only list tool.
type ListWatchlistTool struct {
	src WatchlistSource
}

// NewListWatchlist constructs a ListWatchlistTool backed by the given source.
func NewListWatchlist(src WatchlistSource) *ListWatchlistTool {
	return &ListWatchlistTool{src: src}
}

func (*ListWatchlistTool) Name() string { return "list_watchlist" }

func (*ListWatchlistTool) Description() string {
	return "List all symbols currently tracked by ChartNagari (enabled and disabled). Use when user asks about their watchlist or which symbols they are tracking."
}

func (*ListWatchlistTool) InputSchema() string { return SchemaListWatchlist }

func (t *ListWatchlistTool) Call(_ context.Context, _ json.RawMessage) (ToolResult, error) {
	cfg := t.src.Watchlist()

	type row struct {
		sym     string
		exch    string
		class   string
		enabled bool
	}

	var rows []row
	for _, s := range cfg.Symbols.Crypto {
		rows = append(rows, row{s.Symbol, s.Exchange, "crypto", s.Enabled})
	}
	for _, s := range cfg.Symbols.Stocks {
		rows = append(rows, row{s.Symbol, s.Exchange, "stock", s.Enabled})
	}
	for _, s := range cfg.Symbols.Indices {
		rows = append(rows, row{s.Symbol, s.Exchange, "index", s.Enabled})
	}

	enabledCount := 0
	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		mark := "—"
		if r.enabled {
			mark = "✓"
			enabledCount++
		}
		tableRows = append(tableRows, []string{r.sym, r.exch, r.class, mark})
	}

	header := fmt.Sprintf("**Watchlist (%d symbols, %d enabled)**\n\n",
		len(rows), enabledCount)
	if len(rows) == 0 {
		return TextResult(header + "_(empty)_"), nil
	}
	table := MarkdownTable(
		[]string{"Symbol", "Exchange", "Class", "Enabled"},
		tableRows,
	)
	return TextResult(header + table), nil
}

// SignalSource returns recent signals for a symbol and the latest close price.
// *storage.DB satisfies this interface (via existing GetSignalsFiltered; LatestClose
// is wired in cmd/server/main.go — see Task 11).
type SignalSource interface {
	GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error)
	LatestClose(symbol string) (float64, error)
}

// GetAnalysisTool produces a multi-timeframe markdown analysis for a symbol.
type GetAnalysisTool struct {
	watch  WatchlistSource
	signal SignalSource
}

func NewGetAnalysis(w WatchlistSource, s SignalSource) *GetAnalysisTool {
	return &GetAnalysisTool{watch: w, signal: s}
}

func (*GetAnalysisTool) Name() string { return "get_analysis" }

func (*GetAnalysisTool) Description() string {
	return "Get current multi-timeframe analysis for a symbol: fired rules, MTF score, direction, key support/resistance. Returns all 4 timeframes (1W/1D/4H/1H). Prefer this over get_ohlcv for pattern questions — it is pre-computed and much more token-efficient."
}

func (*GetAnalysisTool) InputSchema() string { return SchemaGetAnalysis }

type getAnalysisParams struct {
	Symbol string `json:"symbol"`
}

// timeframesOrder is the canonical rendering order in output.
var timeframesOrder = []string{"1W", "1D", "4H", "1H"}

// signalLookbackPerTF controls how many recent signals per TF we consider.
const signalLookbackPerTF = 10

func (t *GetAnalysisTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getAnalysisParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
	}
	if p.Symbol == "" {
		return ToolResult{}, NewInvalidParams("symbol required", "Provide {\"symbol\":\"BTCUSDT\"}.")
	}

	wl := t.watch.Watchlist()
	if !watchlistHas(wl, p.Symbol) {
		return ToolResult{}, NewInvalidParams(
			fmt.Sprintf("symbol '%s' not found in watchlist", p.Symbol),
			"Call list_watchlist to see available symbols.",
		)
	}

	price, _ := t.signal.LatestClose(p.Symbol) // ignore error — show 0 on failure

	allSignals, err := t.signal.GetSignalsFiltered(p.Symbol, "", signalLookbackPerTF*len(timeframesOrder))
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: "signal lookup failed: " + err.Error()}
	}
	byTF := make(map[string][]models.Signal)
	for _, s := range allSignals {
		byTF[s.Timeframe] = append(byTF[s.Timeframe], s)
	}

	var tableRows [][]string
	var supports, resistances []float64
	for _, tf := range timeframesOrder {
		sigs := byTF[tf]
		dir := dominantDirection(sigs)
		score := sumScore(sigs)
		rules := renderRules(sigs)
		tableRows = append(tableRows, []string{tf, DashIfEmpty(dir), fmt.Sprintf("%.1f", score), DashIfEmpty(rules)})
		for _, s := range sigs {
			if s.EntryPrice > 0 {
				if s.Direction == "LONG" {
					supports = append(supports, s.EntryPrice)
				} else if s.Direction == "SHORT" {
					resistances = append(resistances, s.EntryPrice)
				}
			}
		}
	}

	now := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	header := fmt.Sprintf("**%s** · $%.2f · %s\n\n", p.Symbol, price, now)
	table := MarkdownTable([]string{"TF", "Dir", "Score", "Rules"}, tableRows)
	levels := fmt.Sprintf("**Support:** %s · **Resistance:** %s\n",
		formatLevels(dedupTopN(supports, 3)),
		formatLevels(dedupTopN(resistances, 3)))

	return TextResult(header + table + "\n" + levels), nil
}

func watchlistHas(cfg appconfig.WatchlistConfig, symbol string) bool {
	for _, s := range cfg.Symbols.Crypto {
		if s.Symbol == symbol {
			return true
		}
	}
	for _, s := range cfg.Symbols.Stocks {
		if s.Symbol == symbol {
			return true
		}
	}
	for _, s := range cfg.Symbols.Indices {
		if s.Symbol == symbol {
			return true
		}
	}
	return false
}

func dominantDirection(sigs []models.Signal) string {
	counts := map[string]int{}
	for _, s := range sigs {
		counts[s.Direction]++
	}
	if counts["LONG"] > counts["SHORT"] && counts["LONG"] > counts["NEUTRAL"] {
		return "LONG"
	}
	if counts["SHORT"] > counts["LONG"] && counts["SHORT"] > counts["NEUTRAL"] {
		return "SHORT"
	}
	if len(sigs) == 0 {
		return ""
	}
	return "NEUTRAL"
}

func sumScore(sigs []models.Signal) float64 {
	var s float64
	for _, x := range sigs {
		s += x.Score
	}
	return s
}

func renderRules(sigs []models.Signal) string {
	var names []string
	seen := map[string]bool{}
	for _, s := range sigs {
		if seen[s.Rule] {
			continue
		}
		seen[s.Rule] = true
		names = append(names, s.Rule)
	}
	return strings.Join(names, ", ")
}

func formatLevels(nums []float64) string {
	if len(nums) == 0 {
		return "—"
	}
	out := make([]string, 0, len(nums))
	for _, n := range nums {
		out = append(out, fmt.Sprintf("%.0f", n))
	}
	return strings.Join(out, ", ")
}

func dedupTopN(nums []float64, n int) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, v := range nums {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) >= n {
			break
		}
	}
	return out
}

// OHLCVSource satisfies the minimal interface needed by GetOHLCVTool.
// *storage.DB satisfies it via its existing GetOHLCV method.
type OHLCVSource interface {
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
}

type GetOHLCVTool struct {
	src OHLCVSource
}

func NewGetOHLCV(s OHLCVSource) *GetOHLCVTool {
	return &GetOHLCVTool{src: s}
}

func (*GetOHLCVTool) Name() string { return "get_ohlcv" }

func (*GetOHLCVTool) Description() string {
	return "Get raw OHLCV candles for a symbol/timeframe. Use ONLY when you need to analyze raw price action yourself. Prefer get_analysis for pattern detection — it is pre-computed and more token-efficient."
}

func (*GetOHLCVTool) InputSchema() string { return SchemaGetOHLCV }

type getOHLCVParams struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
	Limit     int    `json:"limit"`
}

var allowedOHLCVTF = map[string]bool{"1W": true, "1D": true, "4H": true, "1H": true}

const (
	defaultOHLCVLimit = 50
	maxOHLCVLimit     = 500
)

type ohlcvOut struct {
	Symbol  string        `json:"symbol"`
	TF      string        `json:"tf"`
	Candles []ohlcvCandle `json:"candles"`
}

type ohlcvCandle struct {
	T string  `json:"t"`
	O float64 `json:"o"`
	H float64 `json:"h"`
	L float64 `json:"l"`
	C float64 `json:"c"`
	V float64 `json:"v"`
}

func (t *GetOHLCVTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getOHLCVParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
	}
	if p.Symbol == "" {
		return ToolResult{}, NewInvalidParams("symbol required", "")
	}
	if !allowedOHLCVTF[p.Timeframe] {
		return ToolResult{}, NewInvalidParams(
			"invalid timeframe — must be 1W, 1D, 4H, or 1H",
			"Call list_watchlist to see the configured timeframes.",
		)
	}
	if p.Limit <= 0 {
		p.Limit = defaultOHLCVLimit
	}
	if p.Limit > maxOHLCVLimit {
		p.Limit = maxOHLCVLimit
	}

	rows, err := t.src.GetOHLCV(p.Symbol, p.Timeframe, p.Limit)
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: err.Error()}
	}

	out := ohlcvOut{Symbol: p.Symbol, TF: p.Timeframe, Candles: make([]ohlcvCandle, 0, len(rows))}
	for _, r := range rows {
		out.Candles = append(out.Candles, ohlcvCandle{
			T: r.OpenTime.UTC().Format(time.RFC3339),
			O: r.Open, H: r.High, L: r.Low, C: r.Close, V: r.Volume,
		})
	}
	buf, _ := json.Marshal(out)
	return TextResult(string(buf)), nil
}

// CalendarSource returns economic events. *storage.DB satisfies this.
type CalendarSource interface {
	GetEconomicEvents(from, to time.Time) ([]storage.EconomicEvent, error)
}

type GetEconomicCalendarTool struct {
	src CalendarSource
}

func NewGetEconomicCalendar(s CalendarSource) *GetEconomicCalendarTool {
	return &GetEconomicCalendarTool{src: s}
}

func (*GetEconomicCalendarTool) Name() string { return "get_economic_calendar" }

func (*GetEconomicCalendarTool) Description() string {
	return "Get economic events (FOMC, CPI, employment, earnings) within a time range. Use for news or macro context questions."
}

func (*GetEconomicCalendarTool) InputSchema() string { return SchemaGetEconomicCalendar }

type getCalendarParams struct {
	Start     string `json:"start"`
	End       string `json:"end"`
	ImpactMin string `json:"impact_min"`
}

var impactOrder = map[string]int{"low": 0, "medium": 1, "high": 2}

func (t *GetEconomicCalendarTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getCalendarParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
	}
	start, err := time.Parse(time.RFC3339, p.Start)
	if err != nil {
		return ToolResult{}, NewInvalidParams("invalid 'start' (ISO 8601)", "")
	}
	end, err := time.Parse(time.RFC3339, p.End)
	if err != nil {
		return ToolResult{}, NewInvalidParams("invalid 'end' (ISO 8601)", "")
	}
	if !end.After(start) {
		return ToolResult{}, NewInvalidParams("'end' must be after 'start'", "")
	}
	if p.ImpactMin == "" {
		p.ImpactMin = "medium"
	}
	minOrd, ok := impactOrder[p.ImpactMin]
	if !ok {
		return ToolResult{}, NewInvalidParams("invalid impact_min — must be low/medium/high", "")
	}

	events, err := t.src.GetEconomicEvents(start, end)
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: err.Error()}
	}
	var filtered []storage.EconomicEvent
	for _, e := range events {
		if impactOrder[e.Impact] >= minOrd {
			filtered = append(filtered, e)
		}
	}

	header := fmt.Sprintf("**Economic events · %s to %s · impact ≥ %s**\n\n",
		start.UTC().Format("2006-01-02"), end.UTC().Format("2006-01-02"), p.ImpactMin)
	if len(filtered) == 0 {
		return TextResult(header + "_(no events)_"), nil
	}
	rows := make([][]string, 0, len(filtered))
	for _, e := range filtered {
		rows = append(rows, []string{
			e.EventTime.UTC().Format("01-02 15:04"),
			e.Event,
			e.Impact,
			DashIfEmpty(e.Actual),
			DashIfEmpty(e.Forecast),
			DashIfEmpty(e.Previous),
		})
	}
	table := MarkdownTable(
		[]string{"Time (UTC)", "Event", "Impact", "Actual", "Forecast", "Previous"},
		rows,
	)
	return TextResult(header + table), nil
}

// GetSignalHistoryTool returns recent alert history for a symbol.
type GetSignalHistoryTool struct {
	signal SignalSource
}

func NewGetSignalHistory(s SignalSource) *GetSignalHistoryTool {
	return &GetSignalHistoryTool{signal: s}
}

func (*GetSignalHistoryTool) Name() string { return "get_signal_history" }

func (*GetSignalHistoryTool) Description() string {
	return "Get recent alert history for a symbol — rules that fired above the alert threshold, newest first. Default: last 7 days, 50 items. Use for 'what alerts did X trigger recently'."
}

func (*GetSignalHistoryTool) InputSchema() string { return SchemaGetSignalHistory }

type getSignalHistoryParams struct {
	Symbol string `json:"symbol"`
	Since  string `json:"since"`
	Limit  int    `json:"limit"`
}

const (
	defaultHistoryLimit  = 50
	maxHistoryLimit      = 200
	defaultHistoryWindow = 7 * 24 * time.Hour
)

func (t *GetSignalHistoryTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getSignalHistoryParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
	}
	if p.Symbol == "" {
		return ToolResult{}, NewInvalidParams("symbol required", "Provide {\"symbol\":\"BTCUSDT\"}.")
	}
	if p.Limit <= 0 {
		p.Limit = defaultHistoryLimit
	}
	if p.Limit > maxHistoryLimit {
		p.Limit = maxHistoryLimit
	}

	since := time.Now().Add(-defaultHistoryWindow)
	if p.Since != "" {
		parsed, err := time.Parse(time.RFC3339, p.Since)
		if err != nil {
			return ToolResult{}, NewInvalidParams("invalid 'since' format — want ISO 8601", "")
		}
		since = parsed
	}

	raw2, err := t.signal.GetSignalsFiltered(p.Symbol, "", p.Limit*2)
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: err.Error()}
	}
	var filtered []models.Signal
	for _, s := range raw2 {
		if s.CreatedAt.Before(since) {
			continue
		}
		filtered = append(filtered, s)
		if len(filtered) >= p.Limit {
			break
		}
	}

	header := fmt.Sprintf("**%s · %d alerts in window**\n\n", p.Symbol, len(filtered))
	if len(filtered) == 0 {
		return TextResult(header + "_(no recent alerts)_"), nil
	}

	rows := make([][]string, 0, len(filtered))
	for _, s := range filtered {
		rows = append(rows, []string{
			s.CreatedAt.UTC().Format("2006-01-02 15:04"),
			s.Timeframe,
			s.Direction,
			fmt.Sprintf("%.1f", s.Score),
			s.Rule,
		})
	}
	table := MarkdownTable([]string{"Time (UTC)", "TF", "Dir", "Score", "Rule"}, rows)
	return TextResult(header + table), nil
}
