package mcp

import (
	"encoding/json"
	"testing"
)

func TestSchemas_ValidJSON(t *testing.T) {
	cases := map[string]string{
		"list_watchlist":        SchemaListWatchlist,
		"get_analysis":          SchemaGetAnalysis,
		"get_signal_history":    SchemaGetSignalHistory,
		"get_ohlcv":             SchemaGetOHLCV,
		"get_economic_calendar": SchemaGetEconomicCalendar,
	}
	for name, schema := range cases {
		var js any
		if err := json.Unmarshal([]byte(schema), &js); err != nil {
			t.Errorf("%s: invalid JSON: %v", name, err)
		}
	}
}

func TestSchemas_RequireExpectedProperties(t *testing.T) {
	cases := []struct {
		name, schema, mustContain string
	}{
		{"get_analysis", SchemaGetAnalysis, `"symbol"`},
		{"get_signal_history", SchemaGetSignalHistory, `"symbol"`},
		{"get_ohlcv", SchemaGetOHLCV, `"timeframe"`},
		{"get_economic_calendar", SchemaGetEconomicCalendar, `"start"`},
	}
	for _, c := range cases {
		if !contains(c.schema, c.mustContain) {
			t.Errorf("%s missing %s", c.name, c.mustContain)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
