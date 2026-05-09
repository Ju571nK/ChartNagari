package notifier

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeMarkStore captures Mark calls for verification.
type fakeMarkStore struct {
	mu       sync.Mutex
	calls    []string
	nextErr  error
	nextStat string
}

func (f *fakeMarkStore) Mark(signalID int64, action string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, action)
	if f.nextErr != nil {
		return "", f.nextErr
	}
	if f.nextStat != "" {
		return f.nextStat, nil
	}
	return strings.ToUpper(action), nil
}

func TestTelegramBot_HandleCallback_RoutesToMarkStore(t *testing.T) {
	var apiCalls []string
	var apiMu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiMu.Lock()
		apiCalls = append(apiCalls, r.URL.Path)
		apiMu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer srv.Close()

	store := &fakeMarkStore{nextStat: "TOOK"}
	bot := &TelegramBot{
		token:   "test",
		chatID:  "12345",
		client:  &http.Client{Timeout: 5 * time.Second},
		mark:    store,
		baseURL: srv.URL + "/bot",
	}

	cb := &tgCallbackQuery{
		ID:   "cb1",
		Data: "took:42",
		From: tgUser{ID: 7},
		Message: &tgMessage{
			MessageID: 100,
			Text:      "🔔 BTCUSDT alert",
			Chat:      tgChat{ID: 12345},
		},
	}
	bot.handleCallback(context.Background(), cb)

	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.calls) != 1 || store.calls[0] != "took" {
		t.Fatalf("Mark calls = %v, want [took]", store.calls)
	}
	apiMu.Lock()
	defer apiMu.Unlock()
	if len(apiCalls) < 2 {
		t.Errorf("apiCalls = %v, want >= 2 (answerCallbackQuery + edit*)", apiCalls)
	}
}

func TestTelegramBot_HandleCallback_RejectsWrongChat(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("should not call Telegram API for wrong chat: %s", r.URL.Path)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	store := &fakeMarkStore{}
	bot := &TelegramBot{
		token: "test", chatID: "12345",
		client: &http.Client{Timeout: 5 * time.Second}, mark: store,
		baseURL: srv.URL + "/bot",
	}
	cb := &tgCallbackQuery{
		ID:      "cb1",
		Data:    "took:42",
		Message: &tgMessage{MessageID: 100, Chat: tgChat{ID: 99999}}, // wrong chat
	}
	bot.handleCallback(context.Background(), cb)
	if len(store.calls) != 0 {
		t.Errorf("Mark called for wrong chat: %v", store.calls)
	}
}

func TestParseCallbackData(t *testing.T) {
	cases := []struct {
		in      string
		wantAct string
		wantID  int64
		wantErr bool
	}{
		{"took:42", "took", 42, false},
		{"win:123456", "win", 123456, false},
		{"undo:1", "undo", 1, false},
		{"explode:1", "", 0, true},
		{"took:notanumber", "", 0, true},
		{"malformed", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			act, id, err := parseCallbackData(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Errorf("expected err, got act=%q id=%d", act, id)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if act != tc.wantAct || id != tc.wantID {
				t.Errorf("got (%q,%d), want (%q,%d)", act, id, tc.wantAct, tc.wantID)
			}
		})
	}
}

func TestKeyboardForStatus(t *testing.T) {
	// PENDING shows Took + Skipped.
	kb := KeyboardForStatus("PENDING", 42).(map[string]any)
	rows := kb["inline_keyboard"].([][]map[string]string)
	if len(rows[0]) != 2 {
		t.Errorf("PENDING keyboard len = %d, want 2", len(rows[0]))
	}
	// TOOK shows Win/Loss/BE/Undo.
	kb = KeyboardForStatus("TOOK", 42).(map[string]any)
	rows = kb["inline_keyboard"].([][]map[string]string)
	if len(rows[0]) != 4 {
		t.Errorf("TOOK keyboard len = %d, want 4", len(rows[0]))
	}
	// WIN/LOSS/BE shows Edit only (1 button).
	for _, status := range []string{"WIN", "LOSS", "BE"} {
		kb = KeyboardForStatus(status, 42).(map[string]any)
		rows = kb["inline_keyboard"].([][]map[string]string)
		if len(rows[0]) != 1 {
			t.Errorf("%s keyboard len = %d, want 1 (Edit)", status, len(rows[0]))
		}
	}
	// callback_data includes signal_id.
	kb = KeyboardForStatus("PENDING", 999).(map[string]any)
	rows = kb["inline_keyboard"].([][]map[string]string)
	if !strings.Contains(rows[0][0]["callback_data"], "999") {
		t.Errorf("callback_data missing signal_id: %v", rows[0][0])
	}
}
