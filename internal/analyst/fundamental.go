package analyst

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ju571nK/Chatter/internal/llm"
	"github.com/rs/zerolog/log"
)

func fundamentalSystemPrompt(lang string) string {
	langInstr := map[string]string{
		"ko": "Write the analysis in Korean.",
		"ja": "Write the analysis in Japanese.",
	}[lang]
	if langInstr == "" {
		langInstr = "Write the analysis in English."
	}
	return "You are a fundamental price analysis specialist. " +
		"Analyze price-based valuation proxies (price vs moving averages, band position) " +
		"and earnings cycles using historical data and current price structure.\n" +
		langInstr + "\n" +
		"Always end your response with: BULL: X% / BEAR: Y% / SIDEWAYS: Z% (total 100%)"
}

// FundamentalAnalyst calls an LLM with a fundamental-analysis focused prompt.
type FundamentalAnalyst struct {
	provider llm.Provider
}

// NewFundamentalAnalyst creates a FundamentalAnalyst using the given LLM provider.
func NewFundamentalAnalyst(provider llm.Provider) *FundamentalAnalyst {
	return &FundamentalAnalyst{provider: provider}
}

// Analyze runs a fundamental analysis and returns an AnalystOutput.
func (a *FundamentalAnalyst) Analyze(ctx context.Context, input AnalystInput) AnalystOutput {
	prompt := buildFundamentalPrompt(input)
	text, err := a.provider.Complete(ctx, fundamentalSystemPrompt(input.Language), prompt)
	if err != nil {
		log.Error().Err(err).Str("analyst", "fundamental").Msg("LLM call failed")
		return AnalystOutput{Name: "fundamental", Err: err}
	}
	bull, bear, sideways := parsePercentages(text)
	if bull == 0 && bear == 0 && sideways == 0 {
		log.Warn().Str("analyst", "fundamental").Str("response", text).Msg("failed to parse BULL/BEAR/SIDEWAYS percentages")
	}
	return AnalystOutput{Name: "fundamental", Text: text, Bull: bull, Bear: bear, Sideways: sideways}
}

func buildFundamentalPrompt(input AnalystInput) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Fundamental Analysis\n\n", input.Symbol))
	if input.MacroContext != "" {
		sb.WriteString("### S&P 500 (SPY) Market Backdrop\n")
		sb.WriteString(input.MacroContext)
		sb.WriteString("\n---\n\n")
	}
	sb.WriteString(fmt.Sprintf("### %s Price History Summary\n", input.Symbol))
	sb.WriteString(input.HistorySummary)
	sb.WriteString("\n### Current Price Structure Indicators\n")
	for _, k := range []string{"1D:EMA_20", "1D:EMA_50", "1D:EMA_200", "1D:BB_upper", "1D:BB_middle", "1D:BB_lower", "1D:BB_pct", "1W:EMA_50", "1W:EMA_200", "1D:SWING_HIGH", "1D:SWING_LOW"} {
		if v, ok := input.RecentIndicators[k]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %.4f\n", k, v))
		}
	}
	if input.RuleSignalText != "" {
		sb.WriteString("\n### Recent Technical Signals\n")
		sb.WriteString(input.RuleSignalText)
	}
	sb.WriteString("\n\nAnalyze from the following perspectives:\n" +
		"1. Current price position relative to long-term moving averages (overbought/fair/undervalued)\n" +
		"2. Price position within Bollinger Bands and its implications\n" +
		"3. Current level relative to historical highs/lows\n" +
		"4. 6-month to 1-year outlook\n\n" +
		"Last line: BULL: X% / BEAR: Y% / SIDEWAYS: Z%")
	return sb.String()
}
