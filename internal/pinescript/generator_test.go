package pinescript

import (
	"strings"
	"testing"
)

func TestGenerate_KnownRules(t *testing.T) {
	rules := SupportedRules()
	if len(rules) == 0 {
		t.Fatal("no supported rules")
	}
	for _, rule := range rules {
		t.Run(rule, func(t *testing.T) {
			script, err := Generate(rule, 55.0, 1.8)
			if err != nil {
				t.Fatalf("Generate(%q) error: %v", rule, err)
			}
			if !strings.Contains(script, "//@version=5") {
				t.Errorf("expected Pine Script v5 header in output")
			}
			if !strings.Contains(script, "55.0%") {
				t.Errorf("expected win rate in header comment")
			}
		})
	}
}

func TestGenerate_UnknownRule(t *testing.T) {
	_, err := Generate("nonexistent_rule", 0, 0)
	if err == nil {
		t.Fatal("expected error for unknown rule")
	}
}
