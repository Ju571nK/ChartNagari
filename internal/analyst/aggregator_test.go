package analyst

import (
	"errors"
	"strings"
	"testing"
)

func TestAggregateFailures(t *testing.T) {
	for _, outputs := range [][]AnalystOutput{nil, {{Name: "macro", Err: errors.New("model unavailable")}}, {{Name: "macro", Text: "unparseable"}}} {
		result := Aggregate(outputs, 50)
		if !result.Failed() || result.Status != "failed" || result.Final != "ERROR" || result.Confidence != "" {
			t.Fatalf("failure shown as market judgment: %+v", result)
		}
	}
}

func TestAggregatePartialSuccess(t *testing.T) {
	result := Aggregate([]AnalystOutput{
		{Name: "macro", Bull: 60, Bear: 20, Sideways: 20, Text: "valid"},
		{Name: "fundamental", Err: errors.New("unavailable")},
	}, 50)
	if result.Failed() || result.Status != "success" || result.Final != "BULL" || result.BullPct != 60 || !strings.Contains(result.AggregatorReason, "1개") {
		t.Fatalf("partial success changed: %+v", result)
	}
}

func TestNormalizeLegacyAnalysis(t *testing.T) {
	failed := ScenarioResult{Final: "SIDEWAYS", Confidence: "LOW"}
	failed.NormalizeStatus()
	if failed.Status != "failed" || failed.Final != "ERROR" || failed.Confidence != "" {
		t.Fatalf("legacy failure not normalized: %+v", failed)
	}
	valid := ScenarioResult{Final: "SIDEWAYS", Confidence: "HIGH", SidewaysPct: 100}
	valid.NormalizeStatus()
	if valid.Failed() || valid.Final != "SIDEWAYS" || valid.Status != "success" {
		t.Fatalf("valid neutral result rejected: %+v", valid)
	}
}
