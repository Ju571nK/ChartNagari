package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

// Send posts the signal as an HTML message to the configured chat.
// Returns an error when token/chatID are empty or on HTTP failure.
func (s *TelegramSender) Send(ctx context.Context, sig models.Signal) error {
	if s.token == "" || s.chatID == "" {
		return fmt.Errorf("telegram: token 또는 chatID 미설정")
	}

	payload := map[string]string{
		"chat_id":    s.chatID,
		"text":       formatTelegram(sig),
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
		return fmt.Errorf("telegram: 비정상 응답 HTTP %d", resp.StatusCode)
	}
	return nil
}
