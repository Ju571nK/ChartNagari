package analyst

import (
	"strings"
	"testing"
)

func TestAnalystPromptsIncludeSelected15MContext(t *testing.T) {
	input := AnalystInput{Symbol: "USDJPY", Timeframe: "15M", RecentIndicators: map[string]float64{"15M:RSI_14": 21.25, "15M:ATR_14": 0.12}}
	for _, build := range []func(AnalystInput) string{buildSentimentPrompt, buildMacroPrompt, buildFundamentalPrompt} {
		prompt := build(input)
		if !strings.Contains(prompt, "Selected chart timeframe: 15M") || !strings.Contains(prompt, "15M:RSI_14: 21.2500") || !strings.Contains(prompt, "15M:ATR_14: 0.1200") {
			t.Fatal(prompt)
		}
		input.RecentIndicators["15M:RSI_14"] = 78.5
		if build(input) == prompt {
			t.Fatal("changed 15M candle indicators ignored")
		}
		input.RecentIndicators["15M:RSI_14"] = 21.25
	}
}
