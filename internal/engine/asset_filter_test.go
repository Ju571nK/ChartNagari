package engine

import (
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestAssetClassesFilter(t *testing.T) {
	for _, tc := range []struct {
		name    string
		allowed []models.AssetClass
		asset   models.AssetClass
		want    int
	}{
		{"unrestricted forex", nil, models.AssetForex, 1},
		{"forex allowed", []models.AssetClass{models.AssetForex}, models.AssetForex, 1},
		{"forex excluded", []models.AssetClass{models.AssetStock}, models.AssetForex, 0},
		{"legacy zero class stock", []models.AssetClass{models.AssetStock}, "", 1},
		{"legacy zero class excluded", []models.AssetClass{models.AssetForex}, "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := New(RuleConfig{Rules: map[string]RuleEntry{"static": {Enabled: true, Timeframe: "1H", Weight: 1, AssetClasses: tc.allowed}}})
			e.Register(&staticRule{name: "static", baseScore: 1})
			got := e.Run(models.AnalysisContext{AssetClass: tc.asset, Indicators: map[string]float64{}})
			if len(got) != tc.want {
				t.Fatalf("got %d signals, want %d", len(got), tc.want)
			}
		})
	}
}
