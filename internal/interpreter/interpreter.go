// Package interpreter enriches high-scoring signal groups with Claude AI interpretation.
// When the API key is empty the interpreter is disabled and signals are returned unchanged,
// allowing the rest of the pipeline to function without an Anthropic subscription.
package interpreter

import (
	"context"
	"fmt"
	"strings"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	model     = anthropic.ModelClaudeOpus4_6
	maxTokens = int64(600)
)

// SignalGroup bundles all signals detected for a single symbol in one analysis cycle,
// together with the flat indicator map used to build the AI prompt.
type SignalGroup struct {
	Symbol     string
	Signals    []models.Signal
	Indicators map[string]float64
}

// Interpreter enriches SignalGroups via the Claude Messages API.
// It is safe for concurrent use after initialization.
type Interpreter struct {
	client   anthropic.Client
	minScore float64 // minimum sum of group scores required to trigger an API call
	enabled  bool
}

// New creates an Interpreter.
//   - apiKey: Anthropic API key; empty string disables enrichment.
//   - minScore: minimum total signal score for a group to trigger an AI call.
//   - clientOpts: optional SDK options (e.g. option.WithBaseURL for tests).
func New(apiKey string, minScore float64, clientOpts ...option.RequestOption) *Interpreter {
	if apiKey == "" {
		return &Interpreter{enabled: false}
	}
	opts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, clientOpts...)
	return &Interpreter{
		client:   anthropic.NewClient(opts...),
		minScore: minScore,
		enabled:  true,
	}
}

// Enrich iterates groups, calls Claude for those whose total score meets minScore,
// populates Signal.AIInterpretation, and returns a flat signal slice.
// Groups below minScore or when disabled are returned as-is.
func (i *Interpreter) Enrich(ctx context.Context, groups []SignalGroup) []models.Signal {
	var out []models.Signal
	for _, g := range groups {
		out = append(out, i.enrichGroup(ctx, g)...)
	}
	return out
}

func (i *Interpreter) enrichGroup(ctx context.Context, g SignalGroup) []models.Signal {
	total := 0.0
	for _, s := range g.Signals {
		total += s.Score
	}

	if !i.enabled || total < i.minScore {
		return g.Signals
	}

	prompt := buildPrompt(g)

	msg, err := i.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	})
	if err != nil {
		// Graceful degradation: return signals without AI text on API error.
		return g.Signals
	}

	interpretation := extractText(msg)
	if interpretation == "" {
		return g.Signals
	}

	result := make([]models.Signal, len(g.Signals))
	for idx, s := range g.Signals {
		s.AIInterpretation = interpretation
		result[idx] = s
	}
	return result
}

// buildPrompt constructs a Korean-language prompt with signal context and key indicators.
func buildPrompt(g SignalGroup) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s 차트에서 다음 신호가 감지되었습니다:\n\n", g.Symbol))
	for _, s := range g.Signals {
		sb.WriteString(fmt.Sprintf("- [%s] %s → %s (스코어: %.2f)\n  %s\n",
			s.Timeframe, s.Rule, s.Direction, s.Score, s.Message))
	}

	// Include the most relevant key indicators in the prompt.
	keyIndicators := []string{
		"1H:RSI_14", "4H:RSI_14", "1D:RSI_14",
		"1H:EMA_50", "4H:EMA_50",
		"1H:MACD_hist", "4H:MACD_hist",
		"1H:ATR_14", "4H:ATR_14",
		"1H:VOLUME_MA_20", "4H:VOLUME_MA_20",
		"1H:SWING_HIGH", "1H:SWING_LOW",
		"4H:SWING_HIGH", "4H:SWING_LOW",
	}
	wrote := false
	for _, k := range keyIndicators {
		if v, ok := g.Indicators[k]; ok {
			if !wrote {
				sb.WriteString("\n주요 지표:\n")
				wrote = true
			}
			sb.WriteString(fmt.Sprintf("- %s: %.4f\n", k, v))
		}
	}

	sb.WriteString("\n위 상황을 ICT 및 기술적분석 관점에서 해석하고, " +
		"진입 근거와 주의사항을 한국어로 200자 내외로 간결하게 설명해줘.")
	return sb.String()
}

func extractText(msg *anthropic.Message) string {
	for _, block := range msg.Content {
		switch v := block.AsAny().(type) {
		case anthropic.TextBlock:
			return v.Text
		}
	}
	return ""
}
