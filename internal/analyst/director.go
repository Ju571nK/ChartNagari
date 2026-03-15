package analyst

import (
	"context"
	"sync"

	"github.com/Ju571nK/Chatter/internal/llm"
)

// Director orchestrates 3 parallel analyst goroutines and aggregates their results.
type Director struct {
	macro       *MacroAnalyst
	fundamental *FundamentalAnalyst
	sentiment   *SentimentAnalyst
}

// NewDirector creates a Director using the given LLM provider.
func NewDirector(provider llm.Provider) *Director {
	return &Director{
		macro:       NewMacroAnalyst(provider),
		fundamental: NewFundamentalAnalyst(provider),
		sentiment:   NewSentimentAnalyst(provider),
	}
}

// Analyze runs all three analysts in parallel and returns the aggregated ScenarioResult.
func (d *Director) Analyze(ctx context.Context, input AnalystInput) ScenarioResult {
	outputs := make([]AnalystOutput, 3)
	var wg sync.WaitGroup
	wg.Add(3)

	go func() { defer wg.Done(); outputs[0] = d.macro.Analyze(ctx, input) }()
	go func() { defer wg.Done(); outputs[1] = d.fundamental.Analyze(ctx, input) }()
	go func() { defer wg.Done(); outputs[2] = d.sentiment.Analyze(ctx, input) }()
	wg.Wait()

	rsi1D := input.RecentIndicators["1D:RSI_14"]
	result := Aggregate(outputs, rsi1D)
	result.Symbol = input.Symbol
	return result
}
