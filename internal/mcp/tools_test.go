package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
)

// fakeWatchlistSource is an in-memory WatchlistSource for tests.
type fakeWatchlistSource struct{ cfg appconfig.WatchlistConfig }

func (f *fakeWatchlistSource) Watchlist() appconfig.WatchlistConfig { return f.cfg }

func TestListWatchlist_RendersMarkdownTable(t *testing.T) {
	src := &fakeWatchlistSource{cfg: appconfig.WatchlistConfig{
		Symbols: struct {
			Crypto  []appconfig.SymbolEntry `yaml:"crypto"`
			Stocks  []appconfig.SymbolEntry `yaml:"stocks"`
			Indices []appconfig.SymbolEntry `yaml:"indices"`
		}{
			Crypto: []appconfig.SymbolEntry{{Symbol: "BTCUSDT", Exchange: "BINANCE", Enabled: true}},
			Stocks: []appconfig.SymbolEntry{{Symbol: "AAPL", Exchange: "NASDAQ", Enabled: true}},
		},
	}}
	tool := NewListWatchlist(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "BTCUSDT") || !strings.Contains(text, "AAPL") {
		t.Errorf("missing symbols in output: %q", text)
	}
	if !strings.Contains(text, "| Symbol | Exchange |") {
		t.Errorf("missing table header: %q", text)
	}
	if !strings.Contains(text, "2 symbols") {
		t.Errorf("missing count summary: %q", text)
	}
}

func TestListWatchlist_EmptyWatchlist(t *testing.T) {
	src := &fakeWatchlistSource{}
	tool := NewListWatchlist(src)
	res, err := tool.Call(context.Background(), json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	text := res.Content[0].Text
	if !strings.Contains(text, "0 symbols") {
		t.Errorf("empty watchlist summary missing: %q", text)
	}
}

func TestListWatchlist_MetaFields(t *testing.T) {
	tool := NewListWatchlist(&fakeWatchlistSource{})
	if tool.Name() != "list_watchlist" {
		t.Errorf("name: %s", tool.Name())
	}
	if !strings.Contains(tool.Description(), "watchlist") {
		t.Errorf("description missing 'watchlist': %s", tool.Description())
	}
	if tool.InputSchema() != SchemaListWatchlist {
		t.Error("schema mismatch")
	}
}
