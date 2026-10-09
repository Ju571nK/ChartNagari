package notifier

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestTelegramTemplateValidationAndRendering(t *testing.T) {
	for _, bad := range []string{"{bogus}", "{symbol", "symbol}", "{{symbol}}", strings.Repeat("界", 501)} {
		if err := ValidateTelegramTemplate(bad); err == nil {
			t.Errorf("accepted invalid %q", bad)
		}
	}
	if err := ValidateTelegramTemplate(strings.Repeat("界", 500)); err != nil {
		t.Fatal(err)
	}
	sig := models.Signal{Symbol: "A<&B", Timeframe: "1H", Direction: "LONG", Score: 12.5, Message: "reason <&>", CreatedAt: time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)}
	got, included := RenderTelegramAlertWithStatus(sig, "한국어 <&> {symbol} {근거} {entry} {tp} {sl}")
	if !included || !strings.Contains(got, "한국어 &lt;&amp;&gt; A&lt;&amp;B reason &lt;&amp;&gt; — — —") {
		t.Fatalf("escaped/absent levels: %s", got)
	}
	if strings.Contains(got, "&amp;lt;") {
		t.Fatal("double escape")
	}
	if got := RenderTelegramAlert(sig, ""); got != formatTelegram(sig) {
		t.Fatal("empty changed fallback")
	}
	sig.Direction = "NEUTRAL"
	if got := RenderTelegramAlert(sig, "Note"); got != formatTelegram(sig) {
		t.Fatal("neutral changed fallback")
	}
	sig.Direction = "LONG"
	sig.Message = strings.Repeat("界", 4080)
	got, included = RenderTelegramAlertWithStatus(sig, "Note")
	if included || got != formatTelegram(sig) {
		t.Fatal("length guard did not preserve core")
	}
}

func TestTelegramSenderLoadsMatchingSymbolAndDirection(t *testing.T) {
	db, err := storage.New(filepath.Join(t.TempDir(), "sender.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store := storage.NewSymbolMessageTemplateStore(db)
	if err := store.Put(storage.SymbolMessageTemplates{Symbol: "EURUSD", Long: "LONG {symbol}", Short: "SHORT {symbol}"}); err != nil {
		t.Fatal(err)
	}
	var messages []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Error(err)
		}
		messages = append(messages, payload.Text)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
	}))
	defer srv.Close()
	sender := NewTelegramSender("token", "chat").WithTemplateStore(store)
	sender.client = srv.Client()
	sender.client.Transport = &rewriteTransport{base: srv.URL}
	for _, sig := range []models.Signal{
		{Symbol: "EURUSD", Direction: "LONG", CreatedAt: time.Now()},
		{Symbol: "EURUSD", Direction: "SHORT", CreatedAt: time.Now()},
		{Symbol: "USDJPY", Direction: "LONG", CreatedAt: time.Now()},
		{Symbol: "EURUSD", Direction: "NEUTRAL", CreatedAt: time.Now()},
	} {
		if _, err := sender.SendAlert(context.Background(), sig); err != nil {
			t.Fatal(err)
		}
	}
	if len(messages) != 4 {
		t.Fatalf("messages = %d", len(messages))
	}
	if !strings.Contains(messages[0], "LONG EURUSD") || strings.Contains(messages[0], "SHORT EURUSD") {
		t.Fatal(messages[0])
	}
	if !strings.Contains(messages[1], "SHORT EURUSD") || strings.Contains(messages[1], "LONG EURUSD") {
		t.Fatal(messages[1])
	}
	if strings.Contains(messages[2], "Custom note") || strings.Contains(messages[3], "Custom note") {
		t.Fatal("unmatched signal customized")
	}
}
