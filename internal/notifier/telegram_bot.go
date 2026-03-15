package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// AnalysisHandler is called when the bot receives an /analysis command.
// The handler runs the analysis and returns a formatted HTML message.
type AnalysisHandler func(ctx context.Context, symbol string) (string, error)

// TelegramBot polls for incoming bot commands and dispatches them.
type TelegramBot struct {
	token   string
	chatID  string
	client  *http.Client
	handler AnalysisHandler
}

// NewTelegramBot creates a bot that handles /analysis commands.
func NewTelegramBot(token, chatID string, handler AnalysisHandler) *TelegramBot {
	return &TelegramBot{
		token:   token,
		chatID:  chatID,
		client:  &http.Client{Timeout: 40 * time.Second},
		handler: handler,
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
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", b.token)
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
		"https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=%d",
		b.token, offset, timeout,
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

type tgUpdate struct {
	UpdateID int64      `json:"update_id"`
	Message  *tgMessage `json:"message"`
}

type tgMessage struct {
	Text string `json:"text"`
	Chat struct {
		ID int64 `json:"id"`
	} `json:"chat"`
}
