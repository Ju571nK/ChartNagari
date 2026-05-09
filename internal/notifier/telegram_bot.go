package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// AnalysisHandler is called when the bot receives an /analysis command.
// The handler runs the analysis and returns a formatted HTML message.
type AnalysisHandler func(ctx context.Context, symbol string) (string, error)

// MarkStore is the storage interface the bot consumes for marking signals.
// *storage.SignalMarkStore satisfies it.
type MarkStore interface {
	Mark(signalID int64, action string) (newStatus string, err error)
}

// TelegramBot polls for incoming bot commands and dispatches them.
type TelegramBot struct {
	token   string
	chatID  string
	client  *http.Client
	handler AnalysisHandler
	mark    MarkStore // optional; nil disables callback handling
	baseURL string    // override for tests; defaults to "https://api.telegram.org/bot"
}

// NewTelegramBot creates a bot that handles /analysis commands and (when mark is non-nil)
// callback_query updates from inline marking buttons on alert messages.
func NewTelegramBot(token, chatID string, handler AnalysisHandler, mark MarkStore) *TelegramBot {
	return &TelegramBot{
		token:   token,
		chatID:  chatID,
		client:  &http.Client{Timeout: 40 * time.Second},
		handler: handler,
		mark:    mark,
		baseURL: "https://api.telegram.org/bot",
	}
}

// Start begins long-polling for updates. Blocks until ctx is cancelled.
func (b *TelegramBot) Start(ctx context.Context) {
	log.Info().Msg("[TelegramBot] waiting for commands (/analysis SYMBOL)")
	var offset int64

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("[TelegramBot] stopped")
			return
		default:
		}

		updates, err := b.getUpdates(ctx, offset, 30)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn().Err(err).Msg("[TelegramBot] getUpdates failed — retrying in 5s")
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, u := range updates {
			offset = u.UpdateID + 1
			b.handleUpdate(ctx, u)
		}
	}
}

func (b *TelegramBot) handleUpdate(ctx context.Context, u tgUpdate) {
	if u.CallbackQuery != nil {
		b.handleCallback(ctx, u.CallbackQuery)
		return
	}
	if u.Message == nil {
		return
	}
	// Only accept messages from the configured chat
	if fmt.Sprintf("%d", u.Message.Chat.ID) != b.chatID {
		return
	}

	text := strings.TrimSpace(u.Message.Text)
	symbol := parseAnalysisCommand(text)
	if symbol == "" {
		return
	}

	log.Info().Str("symbol", symbol).Msg("[TelegramBot] analysis request received")
	b.sendText(ctx, fmt.Sprintf("⏳ Analyzing <b>%s</b>... (10-20s)", symbol))

	go func() {
		result, err := b.handler(ctx, symbol)
		if err != nil {
			b.sendText(ctx, fmt.Sprintf("❌ Analysis failed for %s: %s", symbol, err.Error()))
			return
		}
		b.sendText(ctx, result)
	}()
}

// parseAnalysisCommand extracts the symbol from /analysis SYMBOL or /분석 SYMBOL.
// Returns "" if the message is not a recognized command.
func parseAnalysisCommand(text string) string {
	lower := strings.ToLower(text)
	var rest string
	switch {
	case strings.HasPrefix(lower, "/analysis "):
		rest = text[len("/analysis "):]
	case strings.HasPrefix(lower, "/분석 "):
		rest = text[len("/분석 "):]
	default:
		return ""
	}
	symbol := strings.ToUpper(strings.TrimSpace(strings.Fields(rest)[0]))
	return symbol
}

func (b *TelegramBot) sendText(ctx context.Context, text string) {
	payload := map[string]string{
		"chat_id":    b.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s%s/sendMessage", b.baseURL, b.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		log.Warn().Err(err).Msg("[TelegramBot] failed to send message")
		return
	}
	resp.Body.Close()
}

func (b *TelegramBot) getUpdates(ctx context.Context, offset int64, timeout int) ([]tgUpdate, error) {
	url := fmt.Sprintf(
		"%s%s/getUpdates?offset=%d&timeout=%d",
		b.baseURL, b.token, offset, timeout,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	var result struct {
		OK     bool       `json:"ok"`
		Result []tgUpdate `json:"result"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse getUpdates response: %w", err)
	}
	if !result.OK {
		return nil, fmt.Errorf("Telegram API error")
	}
	return result.Result, nil
}

// ── Telegram API types ────────────────────────────────────────────────────────

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	ID int64 `json:"id"`
}

type tgUpdate struct {
	UpdateID      int64            `json:"update_id"`
	Message       *tgMessage       `json:"message"`
	CallbackQuery *tgCallbackQuery `json:"callback_query"`
}

type tgMessage struct {
	MessageID int64  `json:"message_id"`
	Text      string `json:"text"`
	Chat      tgChat `json:"chat"`
}

type tgCallbackQuery struct {
	ID      string     `json:"id"`
	Data    string     `json:"data"`
	From    tgUser     `json:"from"`
	Message *tgMessage `json:"message"`
}

// ── Callback handling ─────────────────────────────────────────────────────────

// handleCallback processes a callback_query: validate chat, parse data, mark, edit message + keyboard, answer.
func (b *TelegramBot) handleCallback(ctx context.Context, cb *tgCallbackQuery) {
	if cb.Message == nil {
		return
	}
	if fmt.Sprintf("%d", cb.Message.Chat.ID) != b.chatID {
		return
	}
	if b.mark == nil {
		_ = b.answerCallback(ctx, cb.ID, "Marking disabled on this server")
		return
	}
	action, signalID, err := parseCallbackData(cb.Data)
	if err != nil {
		_ = b.answerCallback(ctx, cb.ID, "Invalid action")
		return
	}
	newStatus, err := b.mark.Mark(signalID, action)
	if err != nil {
		log.Warn().Err(err).Int64("signal_id", signalID).Str("action", action).Msg("[TelegramBot] mark failed")
		_ = b.answerCallback(ctx, cb.ID, "❌ Save failed")
		return
	}
	_ = b.answerCallback(ctx, cb.ID, "✓ "+statusLabel(newStatus))
	if err := b.editKeyboard(ctx, cb.Message.Chat.ID, cb.Message.MessageID, KeyboardForStatus(newStatus, signalID)); err != nil {
		log.Warn().Err(err).Msg("[TelegramBot] editMessageReplyMarkup failed")
	}
	if err := b.appendStatusLine(ctx, cb.Message.Chat.ID, cb.Message.MessageID, cb.Message.Text, newStatus); err != nil {
		log.Warn().Err(err).Msg("[TelegramBot] editMessageText failed")
	}
}

// parseCallbackData splits "{action}:{signal_id}" into (action, id).
func parseCallbackData(data string) (string, int64, error) {
	parts := strings.SplitN(data, ":", 2)
	if len(parts) != 2 {
		return "", 0, fmt.Errorf("malformed callback_data: %q", data)
	}
	switch parts[0] {
	case "took", "skip", "win", "loss", "be", "undo":
	default:
		return "", 0, fmt.Errorf("unknown action: %q", parts[0])
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("invalid signal_id: %q", parts[1])
	}
	return parts[0], id, nil
}

func statusLabel(status string) string {
	switch status {
	case "TOOK":
		return "Took"
	case "SKIPPED":
		return "Skipped"
	case "WIN":
		return "Win"
	case "LOSS":
		return "Loss"
	case "BE":
		return "BE"
	case "":
		return "Reset"
	default:
		return status
	}
}

func (b *TelegramBot) answerCallback(ctx context.Context, callbackID, text string) error {
	body := map[string]any{"callback_query_id": callbackID, "text": text}
	return b.postTG(ctx, "answerCallbackQuery", body)
}

func (b *TelegramBot) editKeyboard(ctx context.Context, chatID int64, messageID int64, keyboard any) error {
	if keyboard == nil {
		return nil
	}
	body := map[string]any{
		"chat_id":      chatID,
		"message_id":   messageID,
		"reply_markup": keyboard,
	}
	return b.postTG(ctx, "editMessageReplyMarkup", body)
}

func (b *TelegramBot) appendStatusLine(ctx context.Context, chatID int64, messageID int64, currentText string, newStatus string) error {
	label := statusLabel(newStatus)
	stamp := time.Now().Format("15:04 MST")
	addition := fmt.Sprintf("✓ %s at %s", label, stamp)
	if newStatus == "WIN" || newStatus == "LOSS" || newStatus == "BE" {
		emoji := map[string]string{"WIN": "💰", "LOSS": "💸", "BE": "⚖"}[newStatus]
		addition = "→ " + emoji + " " + label + " at " + stamp
	}
	newText := currentText
	if !strings.Contains(newText, addition) {
		newText = newText + "\n" + addition
	}
	body := map[string]any{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       newText,
		"parse_mode": "HTML",
	}
	return b.postTG(ctx, "editMessageText", body)
}

func (b *TelegramBot) postTG(ctx context.Context, method string, body map[string]any) error {
	buf, _ := json.Marshal(body)
	url := fmt.Sprintf("%s%s/%s", b.baseURL, b.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		bb, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram %s returned %d: %s", method, resp.StatusCode, string(bb))
	}
	return nil
}

// KeyboardForStatus returns the inline keyboard appropriate for the new status,
// embedding the signal_id in callback_data. Exported so the alert sender can
// mount the PENDING keyboard on initial dispatch.
func KeyboardForStatus(status string, signalID int64) any {
	idStr := strconv.FormatInt(signalID, 10)
	switch status {
	case "TOOK":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "💰 Win", "callback_data": "win:" + idStr},
			{"text": "💸 Loss", "callback_data": "loss:" + idStr},
			{"text": "⚖ BE", "callback_data": "be:" + idStr},
			{"text": "↺ Undo", "callback_data": "undo:" + idStr},
		}}}
	case "SKIPPED":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "↺ Undo", "callback_data": "undo:" + idStr},
		}}}
	case "WIN", "LOSS", "BE":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "↺ Edit", "callback_data": "undo:" + idStr},
		}}}
	case "PENDING", "":
		return map[string]any{"inline_keyboard": [][]map[string]string{{
			{"text": "✅ Took", "callback_data": "took:" + idStr},
			{"text": "❌ Skipped", "callback_data": "skip:" + idStr},
		}}}
	}
	return nil
}
