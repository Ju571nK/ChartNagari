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

// DiscordSender dispatches signals to a Discord channel via an incoming Webhook.
type DiscordSender struct {
	webhookURL string
	client     *http.Client
}

// NewDiscordSender creates a DiscordSender with a 10-second HTTP timeout.
func NewDiscordSender(webhookURL string) *DiscordSender {
	return &DiscordSender{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (s *DiscordSender) Name() string { return "discord" }

// Send posts an embed message to the Discord webhook.
// Returns an error when webhookURL is empty or on HTTP failure.
func (s *DiscordSender) Send(ctx context.Context, sig models.Signal) error {
	if s.webhookURL == "" {
		return fmt.Errorf("discord: webhookURL 미설정")
	}

	embed := map[string]interface{}{
		"title": fmt.Sprintf("%s [%s] %s",
			directionIcon(sig.Direction), sig.Timeframe, sig.Symbol),
		"description": fmt.Sprintf("**룰:** %s\n**스코어:** %.2f\n**메시지:** %s",
			sig.Rule, sig.Score, sig.Message),
		"color": discordColor(sig.Direction),
		"footer": map[string]string{
			"text": sig.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		},
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{embed},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("discord: 페이로드 직렬화 실패: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("discord: 요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("discord: HTTP 전송 실패: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord: 비정상 응답 HTTP %d", resp.StatusCode)
	}
	return nil
}
