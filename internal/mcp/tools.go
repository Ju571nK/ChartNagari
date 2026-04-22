package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
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
