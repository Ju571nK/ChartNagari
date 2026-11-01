package notifier

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/alertdrop"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

type memDrops struct {
	events []alertdrop.Event
}

func (m *memDrops) Record(ev alertdrop.Event) { m.events = append(m.events, ev) }
func (m *memDrops) Recent(context.Context, alertdrop.Query) ([]alertdrop.Summary, error) {
	return nil, nil
}

type namedFail struct {
	name string
	err  error
}

func (s namedFail) Name() string                              { return s.name }
func (s namedFail) Send(context.Context, models.Signal) error { return s.err }

type alertFail struct {
	name string
	err  error
}

func (s alertFail) Name() string                              { return s.name }
func (s alertFail) Send(context.Context, models.Signal) error { return nil }
func (s alertFail) SendAlert(context.Context, models.Signal) (int64, error) {
	return 0, s.err
}

type alertOK struct{ id int64 }

func (s alertOK) Name() string                              { return "telegram" }
func (s alertOK) Send(context.Context, models.Signal) error { return nil }
func (s alertOK) SendAlert(context.Context, models.Signal) (int64, error) {
	return s.id, nil
}

type failMarks struct{}

func (failMarks) SetMessageID(int64, int64) error { return errors.New("db down") }

func TestNotifyRecordsScoreFloorCooldownAndDailyLimit(t *testing.T) {
	mem := &memDrops{}
	n := newNotifier(5, time.Hour).WithDropStore(mem)
	ms := &mockSender{}
	n.Register(ms)

	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 4.9)})
	if len(ms.calls) != 0 || len(mem.events) != 1 || mem.events[0].Reason != alertdrop.ScoreFloor || mem.events[0].Threshold != 5 {
		t.Fatalf("score floor: sends=%d events=%+v", len(ms.calls), mem.events)
	}

	mem.events = nil
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 8)})
	if len(ms.calls) != 1 || len(mem.events) != 0 {
		t.Fatalf("first send: calls=%d events=%d", len(ms.calls), len(mem.events))
	}
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 8)})
	if len(ms.calls) != 1 || len(mem.events) != 1 || mem.events[0].Reason != alertdrop.Cooldown {
		t.Fatalf("cooldown: calls=%d events=%+v", len(ms.calls), mem.events)
	}
	if mem.events[0].Detail != time.Hour.String() {
		t.Fatalf("cooldown detail = %q", mem.events[0].Detail)
	}

	limited := &memDrops{}
	holder := appconfig.NewSymbolProfilesHolder(appconfig.SymbolProfilesConfig{
		DefaultProfile: "tight",
		Profiles: map[string]appconfig.Profile{
			"tight": {AlertLimitPerDay: 1},
		},
	})
	ln := New(Config{ScoreThreshold: 1, CooldownDur: 0}, nopLog()).WithProfileHolder(holder).WithDropStore(limited)
	ok := &mockSender{}
	ln.Register(ok)
	sig := makeSig("ETHUSDT", "macd", "SHORT", 3)
	ln.Notify(context.Background(), []models.Signal{sig})
	ln.Notify(context.Background(), []models.Signal{sig})
	if len(ok.calls) != 1 || len(limited.events) != 1 || limited.events[0].Reason != alertdrop.DailyLimit || limited.events[0].Threshold != 1 {
		t.Fatalf("daily limit: calls=%d events=%+v", len(ok.calls), limited.events)
	}
}

func TestNotifyDeliveryFailStoresFixedDetail(t *testing.T) {
	secret := "https://hooks.example/botTOKEN-should-not-persist"
	mem := &memDrops{}
	n := newNotifier(1, 0).WithDropStore(mem)
	n.Register(namedFail{name: "discord", err: errors.New(secret)})
	n.Register(alertFail{name: "telegram", err: errors.New(secret)})
	n.Notify(context.Background(), []models.Signal{makeSig("AAPL", "ict_fvg", "LONG", 9)})

	if len(mem.events) != 2 {
		t.Fatalf("events = %+v", mem.events)
	}
	seen := map[string]bool{}
	for _, ev := range mem.events {
		if ev.Reason != alertdrop.DeliveryFail || ev.Detail != alertdrop.DeliveryFailDetail {
			t.Fatalf("event = %+v", ev)
		}
		if strings.Contains(ev.Detail, "TOKEN") || strings.Contains(ev.Detail, "hooks") || strings.Contains(ev.Sender, "TOKEN") {
			t.Fatalf("secret leaked into %+v", ev)
		}
		seen[ev.Sender] = true
	}
	if !seen["discord"] || !seen["telegram"] {
		t.Fatalf("senders = %+v", mem.events)
	}
}

func TestMessageIDFailureIsNotADeliveryDrop(t *testing.T) {
	mem := &memDrops{}
	n := newNotifier(1, 0).WithDropStore(mem).WithMarkStore(failMarks{})
	n.Register(alertOK{id: 42})
	n.Notify(context.Background(), []models.Signal{makeSig("AAPL", "ict_fvg", "LONG", 9)})
	if len(mem.events) != 0 {
		t.Fatalf("message id failure recorded as drop: %+v", mem.events)
	}
}

func TestWithDropStoreNilIsNop(t *testing.T) {
	n := newNotifier(5, time.Hour).WithDropStore(nil)
	n.Notify(context.Background(), []models.Signal{makeSig("BTCUSDT", "rsi", "LONG", 1)})
}
