package analyst

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ju571nK/Chatter/internal/llm"
	"github.com/rs/zerolog/log"
)

func sentimentSystemPrompt(lang string) string {
	langInstr := map[string]string{
		"ko": "Write the analysis in Korean.",
		"ja": "Write the analysis in Japanese.",
	}[lang]
	if langInstr == "" {
		langInstr = "Write the analysis in English."
	}
	return "You are a market sentiment and technical indicator analyst. " +
		"Analyze RSI, MACD, volume, volatility (ATR), and Bollinger Bands to assess " +
		"market greed/fear phases and near-term directionality.\n" +
		langInstr + "\n" +
		"Always end your response with: BULL: X% / BEAR: Y% / SIDEWAYS: Z% (total 100%)"
}

// SentimentAnalyst calls an LLM with a sentiment/technical-indicator focused prompt.
type SentimentAnalyst struct {
	provider llm.Provider
}

// NewSentimentAnalyst creates a SentimentAnalyst using the given LLM provider.
func NewSentimentAnalyst(provider llm.Provider) *SentimentAnalyst {
	return &SentimentAnalyst{provider: provider}
}

// Analyze runs a sentiment analysis and returns an AnalystOutput.
func (a *SentimentAnalyst) Analyze(ctx context.Context, input AnalystInput) AnalystOutput {
	prompt := buildSentimentPrompt(input)
	text, err := a.provider.Complete(ctx, sentimentSystemPrompt(input.Language), prompt)
	if err != nil {
		log.Error().Err(err).Str("analyst", "sentiment").Msg("LLM call failed")
		return AnalystOutput{Name: "sentiment", Err: err}
	}
	bull, bear, sideways := parsePercentages(text)
	if bull == 0 && bear == 0 && sideways == 0 {
		log.Warn().Str("analyst", "sentiment").Str("response", text).Msg("failed to parse BULL/BEAR/SIDEWAYS percentages")
	}
	return AnalystOutput{Name: "sentiment", Text: text, Bull: bull, Bear: bear, Sideways: sideways}
}

func buildSentimentPrompt(input AnalystInput) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Market Sentiment Analysis\n\n", input.Symbol))
	if input.MacroContext != "" {
		sb.WriteString("### S&P 500 (SPY) Market Backdrop — Risk-On/Off Context\n")
		sb.WriteString(input.MacroContext)
		sb.WriteString("\n---\n\n")
	}
	sb.WriteString(fmt.Sprintf("### %s Price History Summary\n", input.Symbol))
	sb.WriteString(input.HistorySummary)
	sb.WriteString("\n### Current Technical Indicators\n")
	for _, k := range []string{"1H:RSI_14", "4H:RSI_14", "1D:RSI_14", "1H:MACD_hist", "4H:MACD_hist", "1D:MACD_hist", "1H:ATR_14", "4H:ATR_14", "1D:ATR_14", "1D:VOLUME_MA_20", "1H:VOLUME_MA_20", "1D:BB_width", "1D:BB_pct", "4H:OBV", "1D:OBV"} {
		if v, ok := input.RecentIndicators[k]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %.4f\n", k, v))
		}
	}
	if input.RuleSignalText != "" {
		sb.WriteString("\n### Recent Technical Signals\n")
		sb.WriteString(input.RuleSignalText)
	}
	sb.WriteString("\n\nAnalyze from the following perspectives:\n" +
		"1. RSI-based overbought/oversold phase (greed vs fear)\n" +
		"2. MACD momentum direction and strength\n" +
		"3. Volume/volatility anomaly signals\n" +
		"4. Near-term (1-3 month) directional outlook\n\n" +
		"Last line: BULL: X% / BEAR: Y% / SIDEWAYS: Z%")
	return sb.String()
}
