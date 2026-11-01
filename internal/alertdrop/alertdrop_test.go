package alertdrop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestRegistryComplete(t *testing.T) {
	seen := map[string]Reason{}
	for _, r := range Reasons() {
		if !r.Valid() {
			t.Fatalf("reason %d is not valid", r)
		}
		code := r.Code()
		if code == "" {
			t.Fatalf("reason %d has an empty code", r)
		}
		if prev, ok := seen[code]; ok {
			t.Fatalf("code %q used by %d and %d", code, prev, r)
		}
		seen[code] = r
		if r.Stage() != StagePipeline && r.Stage() != StageNotifier {
			t.Fatalf("%s has no stage", code)
		}
		if r.Stage().String() == "" {
			t.Fatalf("%s stage string is empty", code)
		}
		if r.Knob() == "" {
			t.Fatalf("%s has an empty knob", code)
		}
	}
	if len(seen) != int(reasonCount)-1 {
		t.Fatalf("registry has %d codes, want %d", len(seen), int(reasonCount)-1)
	}
	if Reason(0).Valid() {
		t.Fatal("zero Reason is valid")
	}
}

func TestReasonTextRoundTrip(t *testing.T) {
	for _, r := range Reasons() {
		b, err := r.MarshalText()
		if err != nil {
			t.Fatalf("marshal %s: %v", r.Code(), err)
		}
		var got Reason
		if err := got.UnmarshalText(b); err != nil {
			t.Fatalf("unmarshal %s: %v", r.Code(), err)
		}
		if got != r {
			t.Fatalf("round trip %s got %d", r.Code(), got)
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatalf("json %s: %v", r.Code(), err)
		}
		if string(raw) != `"`+r.Code()+`"` {
			t.Fatalf("json %s = %s", r.Code(), raw)
		}
	}
	var unknown Reason
	if err := unknown.UnmarshalText([]byte("nope")); err == nil {
		t.Fatal("unknown code accepted")
	}
	if _, err := Reason(0).MarshalText(); err == nil {
		t.Fatal("invalid reason marshaled")
	}
}

func TestReasonLocalesCovered(t *testing.T) {
	for _, lang := range []string{"en", "ko", "ja"} {
		path := filepath.Join("..", "..", "web", "src", "i18n", "locales", lang+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var root map[string]any
		if err := json.Unmarshal(raw, &root); err != nil {
			t.Fatalf("parse %s: %v", lang, err)
		}
		drops, _ := root["alertDrops"].(map[string]any)
		reasons, _ := drops["reason"].(map[string]any)
		for _, r := range Reasons() {
			val, ok := reasons[r.Code()].(string)
			if !ok || val == "" {
				t.Errorf("%s missing alertDrops.reason.%s", lang, r.Code())
			}
		}
	}
}

func TestOfCopiesSignalIdentity(t *testing.T) {
	sig := models.Signal{
		ID:        9,
		Symbol:    "BTCUSDT",
		Rule:      "ict_fvg",
		Timeframe: "1H",
		Direction: "LONG",
		Score:     14.2,
	}
	ev := Of(Cooldown, sig).WithThreshold(4).WithDetail("cooldown=4h").WithSender("telegram")
	if ev.Reason != Cooldown || ev.Symbol != sig.Symbol || ev.Rule != sig.Rule ||
		ev.Timeframe != sig.Timeframe || ev.Direction != sig.Direction ||
		ev.Score != sig.Score || ev.SignalID != sig.ID {
		t.Fatalf("identity not copied: %+v", ev)
	}
	if ev.Threshold != 4 || ev.Detail != "cooldown=4h" || ev.Sender != "telegram" {
		t.Fatalf("builders: %+v", ev)
	}
	if ev.At.IsZero() || ev.At.Location() != time.UTC {
		t.Fatalf("At = %v", ev.At)
	}
	again := ev.WithDetail("other")
	if ev.Detail != "cooldown=4h" || again.Detail != "other" {
		t.Fatal("WithDetail mutated the original")
	}
}

func TestQueryNormalize(t *testing.T) {
	got := (Query{Symbol: " btcusdt ", Limit: 0}).Normalize()
	if got.Symbol != "BTCUSDT" || got.Limit != DefaultLimit {
		t.Fatalf("zero limit: %+v", got)
	}
	got = (Query{Limit: MaxLimit + 10}).Normalize()
	if got.Limit != MaxLimit {
		t.Fatalf("clamp: %+v", got)
	}
	got = (Query{Limit: 3}).Normalize()
	if got.Limit != 3 || got.Symbol != "" {
		t.Fatalf("keep: %+v", got)
	}
}

func TestEnabled(t *testing.T) {
	if Enabled(nil) {
		t.Fatal("nil store is enabled")
	}
	if Enabled(Nop{}) {
		t.Fatal("Nop is enabled")
	}
	if !Enabled(recordingStore{}) {
		t.Fatal("recording store is disabled")
	}
}

type recordingStore struct{}

func (recordingStore) Record(Event) {}
func (recordingStore) Recent(context.Context, Query) ([]Summary, error) {
	return nil, nil
}
