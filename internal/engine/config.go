// Package engine provides the rule execution engine for Chart Analyzer.
// It loads rule configuration, registers AnalysisRule plugins, and produces
// scored signals sorted by strength.
package engine

// RuleConfig is loaded from rules.yaml (engine-specific subset).
// The map key is the rule name that must match AnalysisRule.Name().
type RuleConfig struct {
	Rules map[string]RuleEntry `yaml:"rules"`
}

// RuleEntry holds per-rule activation and scoring metadata.
type RuleEntry struct {
	Enabled   bool    `yaml:"enabled"`
	Timeframe string  `yaml:"timeframe"` // "1H" | "4H" | "1D" | "1W" | "ALL"
	Weight    float64 `yaml:"weight"`    // rule strength weight used in scoring
}

// TFWeight returns the PRD-defined timeframe weight multiplier.
//
//	1W  → 2.0
//	1D  → 1.5
//	4H  → 1.2
//	1H  → 1.0
//	ALL or unknown → 1.0
func TFWeight(tf string) float64 {
	switch tf {
	case "1W":
		return 2.0
	case "1D":
		return 1.5
	case "4H":
		return 1.2
	case "1H":
		return 1.0
	default:
		return 1.0
	}
}
