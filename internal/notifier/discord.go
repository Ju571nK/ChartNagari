package notifier

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// DiscordSender dispatches signals to a Discord channel via an incoming Webhook.
type DiscordSender struct {
	webhookURL string
	publicURL  string
	client     *http.Client
}

// NewDiscordSender creates a DiscordSender with a 10-second HTTP timeout.
func NewDiscordSender(webhookURL string) *DiscordSender {
	return &DiscordSender{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

// WithPublicURL sets the browser origin used for the My Trades review link.
// An empty or unusable value leaves the embed without a link.
func (s *DiscordSender) WithPublicURL(base string) *DiscordSender {
	s.publicURL = strings.TrimSpace(base)
	return s
}

func (s *DiscordSender) Name() string { return "discord" }

// SendText sends a plain text message via Discord webhook.
// Implements TextSender — used by Notifier.Announce().
func (s *DiscordSender) SendText(ctx context.Context, text string) error {
	if s.webhookURL == "" {
		return fmt.Errorf("discord: webhookURL 미설정")
	}
	payload := map[string]string{"content": text}
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

// Send posts an embed message to the Discord webhook.
// Returns an error when webhookURL is empty or on HTTP failure.
func (s *DiscordSender) Send(ctx context.Context, sig models.Signal) error {
	if s.webhookURL == "" {
		return fmt.Errorf("discord: webhookURL 미설정")
	}

	desc := fmt.Sprintf("**룰:** %s\n**스코어:** %.2f\n**메시지:** %s",
		sig.Rule, sig.Score, sig.Message)
	if sig.MacroNote != "" {
		desc += "\n\n" + sig.MacroNote
	}
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
			{"name": "💰 진입가", "value": formatEntry(sig), "inline": true},
			{"name": "🎯 TP", "value": formatLevel(sig, sig.TP), "inline": true},
			{"name": "🛡 SL", "value": formatLevel(sig, sig.SL), "inline": true},
		}
	}
	if sig.DataProxy {
		desc += "\n⚠️ Futures proxy data"
	}
	if sig.VolumeUnconfirmed {
		desc += "\nVolume unconfirmed"
	}
	// A webhook cannot host Took or Skipped buttons. The link opens the same mark actions on the web.
	if link := ReviewPendingURL(s.publicURL); link != "" {
		desc += "\n\n[Took / Skipped](" + link + ")"
	}
	embed["description"] = desc

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

// ReviewPendingURL builds the My Trades Pending deep link for Discord embeds.
// Empty or unparseable bases return "" so the embed omits the link.
func ReviewPendingURL(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	q := u.Query()
	q.Set("view", "my-trades")
	q.Set("marks", "pending")
	u.RawQuery = q.Encode()
	return u.String()
}

// PublicURLIsLoopback reports whether base is a usable origin whose host is
// loopback (localhost / 127.0.0.1 / ::1). Phones opening Discord cannot use it.
func PublicURLIsLoopback(base string) bool {
	base = strings.TrimSpace(base)
	if base == "" {
		return false
	}
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
