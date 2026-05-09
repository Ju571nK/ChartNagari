package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/marks"
)

// RollupSource is the minimal interface get_my_performance needs.
// *marks.Aggregator satisfies it.
type RollupSource interface {
	Rollup(by marks.GroupBy, since time.Time) ([]marks.RollupRow, error)
}

type getMyPerformanceParams struct {
	By        string `json:"by"`
	SinceDays int    `json:"since_days"`
	Filter    *struct {
		Rule        string `json:"rule"`
		Symbol      string `json:"symbol"`
		Methodology string `json:"methodology"`
	} `json:"filter"`
}

// GetMyPerformanceTool exposes personal signal-mark stats to MCP/LLM clients.
type GetMyPerformanceTool struct {
	src RollupSource
}

// NewGetMyPerformance constructs a GetMyPerformanceTool backed by the given source.
func NewGetMyPerformance(src RollupSource) *GetMyPerformanceTool {
	return &GetMyPerformanceTool{src: src}
}

func (t *GetMyPerformanceTool) Name() string { return "get_my_performance" }

func (t *GetMyPerformanceTool) Description() string {
	return "Personal trading performance from user-marked alerts. Aggregates Took/Skipped/Win/Loss/BE counts grouped by rule, symbol, methodology, or timeframe over a recent window."
}

const schemaGetMyPerformance = `{
  "type": "object",
  "properties": {
    "by": { "type": "string", "enum": ["rule","symbol","methodology","timeframe"], "default": "rule" },
    "since_days": { "type": "integer", "minimum": 1, "maximum": 730, "default": 30 },
    "filter": {
      "type": "object",
      "properties": {
        "rule":        { "type": "string" },
        "symbol":      { "type": "string" },
        "methodology": { "type": "string", "enum": ["ict","wyckoff","smc","general_ta","candlestick"] }
      }
    }
  },
  "additionalProperties": false
}`

func (t *GetMyPerformanceTool) InputSchema() string { return schemaGetMyPerformance }

func (t *GetMyPerformanceTool) Call(_ context.Context, raw json.RawMessage) (ToolResult, error) {
	var p getMyPerformanceParams
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &p); err != nil {
			return ToolResult{}, NewInvalidParams("invalid params: "+err.Error(), "")
		}
	}
	if p.By == "" {
		p.By = "rule"
	}
	switch p.By {
	case "rule", "symbol", "methodology", "timeframe":
	default:
		return ToolResult{}, NewInvalidParams(fmt.Sprintf("invalid 'by' value: %q", p.By), "")
	}
	if p.SinceDays <= 0 {
		p.SinceDays = 30
	}
	if p.SinceDays > 730 {
		p.SinceDays = 730
	}

	since := time.Now().Add(-time.Duration(p.SinceDays) * 24 * time.Hour)
	rows, err := t.src.Rollup(marks.GroupBy(p.By), since)
	if err != nil {
		return ToolResult{}, &Error{Code: ErrCodeInternalError, Message: err.Error()}
	}

	if p.Filter != nil {
		rows = filterRollup(rows, p.By, p.Filter.Rule, p.Filter.Symbol, p.Filter.Methodology)
	}

	header := fmt.Sprintf("**Personal Performance · last %d days · by %s**\n\n", p.SinceDays, p.By)
	if len(rows) == 0 {
		return TextResult(header + "**No marked trades in window.** _(Mark some alerts via Telegram or the My Trades tab first.)_"), nil
	}

	tableRows := make([][]string, 0, len(rows))
	for _, r := range rows {
		tableRows = append(tableRows, []string{
			r.Key,
			fmt.Sprintf("%d", r.Took),
			fmt.Sprintf("%d", r.Wins),
			fmt.Sprintf("%d", r.Losses),
			fmt.Sprintf("%d", r.BreakEvens),
			fmt.Sprintf("%.1f%%", r.HitRate*100),
			fmt.Sprintf("%.1f%%", r.SkipRate*100),
		})
	}
	headers := []string{titleByDimension(p.By), "Took", "Win", "Loss", "BE", "Hit Rate", "Skip Rate"}
	table := MarkdownTable(headers, tableRows)
	footer := "\n_Hit Rate = Wins / (Wins + Losses + BE).  Skip Rate = Skipped / (Took + Skipped)._"
	return TextResult(header + table + footer), nil
}

func filterRollup(rows []marks.RollupRow, by, ruleFilter, symbolFilter, methodFilter string) []marks.RollupRow {
	out := make([]marks.RollupRow, 0, len(rows))
	for _, r := range rows {
		if methodFilter != "" {
			if by == "rule" {
				if appconfig.RuleMethodology(r.Key) != methodFilter {
					continue
				}
			} else if by == "methodology" {
				if r.Key != methodFilter {
					continue
				}
			}
		}
		if ruleFilter != "" && by == "rule" && r.Key != ruleFilter {
			continue
		}
		if symbolFilter != "" && by == "symbol" && !strings.EqualFold(r.Key, symbolFilter) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func titleByDimension(by string) string {
	switch by {
	case "rule":
		return "Rule"
	case "symbol":
		return "Symbol"
	case "methodology":
		return "Methodology"
	case "timeframe":
		return "Timeframe"
	}
	return by
}
