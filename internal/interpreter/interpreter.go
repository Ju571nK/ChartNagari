// Package interpreter enriches high-scoring signal groups with Claude AI interpretation.
// When the API key is empty the interpreter is disabled and signals are returned unchanged,
// allowing the rest of the pipeline to function without an Anthropic subscription.
package interpreter

import (
	"context"
	"fmt"
	"strings"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const (
	model     = anthropic.ModelClaudeOpus4_6
	maxTokens = int64(800)
)

const systemPrompt = "당신은 ICT(Inner Circle Trader)와 Wyckoff 방법론을 전문으로 하는 " +
	"숙련된 트레이더 겸 리스크 매니저입니다. 분석은 한국어로 작성하며, " +
	"구체적인 가격 레벨과 리스크:보상 비율을 포함합니다."

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

	params := anthropic.MessageNewParams{
		Model:     model,
		MaxTokens: maxTokens,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(prompt)),
		},
	}

	msg, err := i.client.Messages.New(ctx, params)
	if err != nil {
		// 1회 재시도: 2초 대기 후 재호출
		time.Sleep(2 * time.Second)
		msg, err = i.client.Messages.New(ctx, params)
		if err != nil {
			// Graceful degradation: return signals without AI text on API error.
			return g.Signals
		}
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

	// MTF confluence summary
	longs, shorts := 0, 0
	for _, s := range g.Signals {
		switch s.Direction {
		case "LONG":
			longs++
		case "SHORT":
			shorts++
		}
	}
	dominant := "혼재"
	if longs > shorts {
		dominant = "롱 우세"
	} else if shorts > longs {
		dominant = "숏 우세"
	}
	sb.WriteString(fmt.Sprintf("## MTF 합류 분석: %s (LONG %d개 / SHORT %d개)\n\n", dominant, longs, shorts))

	sb.WriteString(fmt.Sprintf("%s 차트에서 다음 신호가 감지되었습니다:\n\n", g.Symbol))
	for _, s := range g.Signals {
		sb.WriteString(fmt.Sprintf("- [%s] %s → %s (스코어: %.2f)\n  %s\n",
			s.Timeframe, s.Rule, s.Direction, s.Score, s.Message))
		if s.EntryPrice > 0 && s.TP > 0 && s.SL > 0 {
			var rr float64
			if s.Direction == "LONG" {
				risk := s.EntryPrice - s.SL
				if risk > 0 {
					rr = (s.TP - s.EntryPrice) / risk
				}
			} else {
				risk := s.SL - s.EntryPrice
				if risk > 0 {
					rr = (s.EntryPrice - s.TP) / risk
				}
			}
			sb.WriteString(fmt.Sprintf("  진입: %.4f | TP: %.4f | SL: %.4f | R:R=1:%.2f\n",
				s.EntryPrice, s.TP, s.SL, rr))
		}
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

	sb.WriteString("\n다음 4가지 항목으로 한국어 분석을 작성해줘:\n" +
		"1. 시장구조: 현재 추세와 구조적 특징\n" +
		"2. 진입근거: ICT/Wyckoff 관점의 핵심 진입 이유\n" +
		"3. 위험요인: 시나리오가 무효화되는 조건\n" +
		"4. 결론: LONG/SHORT/관망 중 하나와 간략한 이유")
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
