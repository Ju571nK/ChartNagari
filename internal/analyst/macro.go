package analyst

import (
	"context"
	"fmt"
	"strings"

	"github.com/Ju571nK/Chatter/internal/llm"
	"github.com/rs/zerolog/log"
)

func macroSystemPrompt(lang string) string {
	langInstr := map[string]string{
		"ko": "Write the analysis in Korean.",
		"ja": "Write the analysis in Japanese.",
	}[lang]
	if langInstr == "" {
		langInstr = "Write the analysis in English."
	}
	return "You are a macroeconomic analyst specializing in interest rate cycles, " +
		"central bank policy, sector rotation, and market cycles (bull-bear transitions). " +
		"Analyze using historical data and current indicators.\n" +
		langInstr + "\n" +
		"Always end your response with: BULL: X% / BEAR: Y% / SIDEWAYS: Z% (total 100%)"
}

// MacroAnalyst calls an LLM with a macro-economics focused prompt.
type MacroAnalyst struct {
	provider llm.Provider
}

// NewMacroAnalyst creates a MacroAnalyst using the given LLM provider.
func NewMacroAnalyst(provider llm.Provider) *MacroAnalyst {
	return &MacroAnalyst{provider: provider}
}

// Analyze runs a macro-economics analysis and returns an AnalystOutput.
func (a *MacroAnalyst) Analyze(ctx context.Context, input AnalystInput) AnalystOutput {
	prompt := buildMacroPrompt(input)
	text, err := a.provider.Complete(ctx, macroSystemPrompt(input.Language), prompt)
	if err != nil {
		log.Error().Err(err).Str("analyst", "macro").Msg("LLM call failed")
		return AnalystOutput{Name: "macro", Err: err}
	}
	bull, bear, sideways := parsePercentages(text)
	if bull == 0 && bear == 0 && sideways == 0 {
		log.Warn().Str("analyst", "macro").Str("response", text).Msg("failed to parse BULL/BEAR/SIDEWAYS percentages")
	}
	return AnalystOutput{Name: "macro", Text: text, Bull: bull, Bear: bear, Sideways: sideways}
}

func buildMacroPrompt(input AnalystInput) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## %s Macro Analysis\n\n", input.Symbol))
	if input.MacroContext != "" {
		sb.WriteString("### S&P 500 (SPY) Macro Backdrop\n")
		sb.WriteString(input.MacroContext)
		sb.WriteString("\n---\n\n")
	}
	sb.WriteString(fmt.Sprintf("### %s Price History Summary\n", input.Symbol))
	sb.WriteString(input.HistorySummary)
	sb.WriteString("\n### Current Indicators\n")
	for _, k := range []string{"1W:RSI_14", "1D:RSI_14", "1W:EMA_50", "1D:EMA_50", "1D:EMA_200", "1W:MACD_hist", "1D:MACD_hist"} {
		if v, ok := input.RecentIndicators[k]; ok {
			sb.WriteString(fmt.Sprintf("- %s: %.4f\n", k, v))
		}
	}
	if input.RuleSignalText != "" {
		sb.WriteString("\n### Recent Technical Signals\n")
		sb.WriteString(input.RuleSignalText)
	}
	sb.WriteString("\n\nAnalyze from the following perspectives:\n" +
		"1. Current market cycle position (expansion/peak/contraction/trough)\n" +
		"2. Interest rate environment and equity valuation pressure\n" +
		"3. Sector rotation signals (defensive vs growth)\n" +
		"4. 1-year outlook\n\n" +
		"Last line: BULL: X% / BEAR: Y% / SIDEWAYS: Z%")
	return sb.String()
}
