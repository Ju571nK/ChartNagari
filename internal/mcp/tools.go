package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
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

// Compile-time guard for future tool handlers: strconv is used by later tools.
var _ = strconv.Itoa
