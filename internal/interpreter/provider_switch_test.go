package interpreter

import (
	"context"
	"testing"

	"github.com/Ju571nK/Chatter/internal/llm"
	"github.com/Ju571nK/Chatter/pkg/models"
)

type explanationProvider struct{ output string }

func (p explanationProvider) Complete(context.Context, string, string) (string, error) {
	return p.output, nil
}

func TestActivatedProviderEnrichesSignalsFromDisabledStart(t *testing.T) {
	i := New("", 12, "en")
	signals := []SignalGroup{{Symbol: "SPY", Signals: []models.Signal{{Symbol: "SPY", Score: 15}}}}
	if got := i.Enrich(context.Background(), signals)[0].AIInterpretation; got != "" {
		t.Fatalf("unexpected initial explanation: %q", got)
	}
	switcher := llm.NewSwitch(explanationProvider{"first model"})
	i.SetProvider(switcher)
	if got := i.Enrich(context.Background(), signals)[0].AIInterpretation; got != "first model" {
		t.Fatalf("first activation: %q", got)
	}
	switcher.Set(explanationProvider{"second model"})
	if got := i.Enrich(context.Background(), signals)[0].AIInterpretation; got != "second model" {
		t.Fatalf("runtime switch: %q", got)
	}
}
