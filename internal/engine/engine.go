package engine

import (
	"sort"
	"strings"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ruleRecord pairs a registered AnalysisRule with its config entry.
type ruleRecord struct {
	impl  rule.AnalysisRule
	entry RuleEntry
}

// RuleEngine executes registered AnalysisRule plugins and scores the resulting
// signals according to the timeframe weight and rule weight defined in config.
type RuleEngine struct {
	cfg     RuleConfig
	records []ruleRecord
}

// New creates a RuleEngine from the provided rule configuration.
func New(cfg RuleConfig) *RuleEngine {
	return &RuleEngine{cfg: cfg}
}

// Register adds a rule plugin to the engine.
// The rule is silently skipped when its Name() is not present in the active
// config or when its config entry has Enabled = false.
func (e *RuleEngine) Register(r rule.AnalysisRule) {
	entry, ok := e.cfg.Rules[r.Name()]
	if !ok || !entry.Enabled {
		return
	}
	e.records = append(e.records, ruleRecord{impl: r, entry: entry})
}

// Run executes all registered active rules against ctx.
// For each rule:
//  1. RequiredIndicators() keys are checked against ctx.Indicators. Bare keys
//     must all exist on the same available timeframe, or the rule is skipped.
//  2. Analyze() is called; a nil return is treated as "no signal".
//  3. The signal's Score is multiplied by TFWeight(entry.Timeframe) × entry.Weight.
//
// Returns all produced signals sorted by Score descending.
func (e *RuleEngine) Run(ctx models.AnalysisContext) []models.Signal {
	var signals []models.Signal

	for _, rec := range e.records {
		if len(rec.entry.AssetClasses) > 0 {
			assetClass := ctx.AssetClass
			if assetClass == "" {
				assetClass = models.AssetStock
			}
			allowed := false
			for _, class := range rec.entry.AssetClasses {
				if class == assetClass {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		if !hasRequiredIndicators(ctx, rec.impl.RequiredIndicators()) {
			continue
		}

		sig, err := rec.impl.Analyze(ctx)
		if err != nil || sig == nil {
			continue
		}

		// Apply scoring multiplier: TF weight × rule weight.
		sig.Score *= TFWeight(rec.entry.Timeframe) * rec.entry.Weight

		signals = append(signals, *sig)
	}

	// Sort by Score descending.
	sort.Slice(signals, func(i, j int) bool {
		return signals[i].Score > signals[j].Score
	})

	return signals
}

// Bare requirements must coexist on one available timeframe. Rules that scan
// multiple timeframes still select their own qualifying series in Analyze.
func hasRequiredIndicators(ctx models.AnalysisContext, required []string) bool {
	if len(required) == 0 {
		return true
	}
	var bare []string
	for _, key := range required {
		if strings.Contains(key, ":") {
			if _, ok := ctx.Indicators[key]; !ok {
				return false
			}
		} else {
			bare = append(bare, key)
		}
	}
	if len(bare) == 0 {
		return true
	}
	for tf, bars := range ctx.Timeframes {
		if len(bars) == 0 {
			continue
		}
		all := true
		for _, key := range bare {
			if _, ok := ctx.Indicators[tf+":"+key]; !ok {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	// Preserve callers that explicitly supply unprefixed, single-series maps.
	for _, key := range bare {
		if _, ok := ctx.Indicators[key]; !ok {
			return false
		}
	}
	return true
}
