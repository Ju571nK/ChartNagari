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

	desc := fmt.Sprintf("**룰:** %s\n**스코어:** %.2f\n**메시지:** %s",
		sig.Rule, sig.Score, sig.Message)
	if sig.AIInterpretation != "" {
		desc += "\n\n💡 " + sig.AIInterpretation
	}

	embed := map[string]interface{}{
		"title":       fmt.Sprintf("%s [%s] %s", directionIcon(sig.Direction), sig.Timeframe, sig.Symbol),
		"description": desc,
		"color":       discordColor(sig.Direction),
		"footer": map[string]string{
			"text": sig.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		},
	}
	// Add trade level fields when ATR data is available.
	if sig.EntryPrice > 0 {
		embed["fields"] = []map[string]interface{}{
			{"name": "💰 진입가", "value": fmtPrice(sig.EntryPrice), "inline": true},
			{"name": "🎯 TP", "value": fmtPrice(sig.TP), "inline": true},
			{"name": "🛡 SL", "value": fmtPrice(sig.SL), "inline": true},
		}
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
