package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// TelegramSender dispatches signals to a Telegram chat via the Bot API.
// Requires a valid bot token and chat ID configured in .env.
type TelegramSender struct {
	token  string
	chatID string
	client *http.Client
}

// NewTelegramSender creates a TelegramSender with a 10-second HTTP timeout.
func NewTelegramSender(token, chatID string) *TelegramSender {
	return &TelegramSender{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *TelegramSender) Name() string { return "telegram" }

// SendText sends a raw HTML text message, bypassing signal formatting.
// Implements TextSender — used by Notifier.Announce().
func (s *TelegramSender) SendText(ctx context.Context, text string) error {
	if s.token == "" || s.chatID == "" {
		return fmt.Errorf("telegram: token 또는 chatID 미설정")
	}
	payload := map[string]string{
		"chat_id":    s.chatID,
		"text":       text,
		"parse_mode": "HTML",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("telegram: 페이로드 직렬화 실패: %w", err)
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telegram: 요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("telegram: HTTP 전송 실패: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return fmt.Errorf("telegram: HTTP %d — %s", resp.StatusCode, string(detail))
	}
	return nil
}

// SendAlert posts the signal as an HTML message with the PENDING inline keyboard
// and returns the Telegram message_id. The message_id lets the bot later call
// editMessageReplyMarkup when the user taps a callback button.
// Returns an error when token/chatID are empty or on HTTP failure.
func (s *TelegramSender) SendAlert(ctx context.Context, sig models.Signal) (int64, error) {
	if s.token == "" || s.chatID == "" {
		return 0, fmt.Errorf("telegram: token 또는 chatID 미설정")
	}

	payload := map[string]any{
		"chat_id":      s.chatID,
		"text":         formatTelegram(sig),
		"parse_mode":   "HTML",
		"reply_markup": KeyboardForStatus("PENDING", sig.ID),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("telegram: 페이로드 직렬화 실패: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", s.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("telegram: 요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("telegram: HTTP 전송 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return 0, fmt.Errorf("telegram: HTTP %d — %s", resp.StatusCode, string(detail))
	}

	var out struct {
		Result struct {
			MessageID int64 `json:"message_id"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, fmt.Errorf("telegram: 응답 디코드 실패: %w", err)
	}
	return out.Result.MessageID, nil
}

// Send posts the signal as an HTML message to the configured chat.
// Existing callers that do not need the message_id can continue using Send;
// new callers should prefer SendAlert.
func (s *TelegramSender) Send(ctx context.Context, sig models.Signal) error {
	_, err := s.SendAlert(ctx, sig)
	return err
}
