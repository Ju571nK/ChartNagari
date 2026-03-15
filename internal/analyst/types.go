// Package analyst provides multi-analyst AI analysis for extended price history.
// Three analysts (Macro, Fundamental, Sentiment) run in parallel goroutines,
// each calling Claude independently, and their results are aggregated into
// a single ScenarioResult with BULL/BEAR/SIDEWAYS probability percentages.
package analyst

// AnalystInput bundles the data passed to each analyst.
type AnalystInput struct {
	Symbol           string
	HistorySummary   string             // output from history.Summarizer (~600 tokens)
	MacroContext     string             // SPY 20-year summary used as S&P 500 backdrop (empty when analyzing SPY itself)
	RecentIndicators map[string]float64 // keyed as "TF:IndicatorName"
	RuleSignalText   string             // recent signals as plain text
	Language         string             // "en" | "ko" | "ja" (default "en")
}

// AnalystOutput holds a single analyst's raw response.
type AnalystOutput struct {
	Name     string  // "macro" | "fundamental" | "sentiment"
	Text     string  // full Claude response text
	Bull     float64 // parsed BULL percentage (0-100)
	Bear     float64 // parsed BEAR percentage (0-100)
	Sideways float64 // parsed SIDEWAYS percentage (0-100)
	Err      error   // non-nil if Claude call failed
}

// ScenarioResult is the aggregated output returned to the API caller.
type ScenarioResult struct {
	ID               int64   `json:"id,omitempty"`
	Symbol           string  `json:"symbol"`
	BullPct          float64 `json:"bull_pct"`
	BearPct          float64 `json:"bear_pct"`
	SidewaysPct      float64 `json:"sideways_pct"`
	Final            string  `json:"final"`            // "BULL" | "BEAR" | "SIDEWAYS"
	Confidence       string  `json:"confidence"`       // "HIGH" | "MEDIUM" | "LOW"
	MacroText        string  `json:"macro_text"`
	FundamentalText  string  `json:"fundamental_text"`
	SentimentText    string  `json:"sentiment_text"`
	AggregatorReason string  `json:"aggregator_reason"`
}
