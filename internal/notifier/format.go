package notifier

import (
	"fmt"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// directionIcon returns an emoji representing the signal direction.
func directionIcon(dir string) string {
	switch dir {
	case "LONG":
		return "📈"
	case "SHORT":
		return "📉"
	default:
		return "🔔"
	}
}

// formatTelegram produces an HTML-formatted message for Telegram.
func formatTelegram(sig models.Signal) string {
	return fmt.Sprintf(
		"%s <b>%s</b> — %s [%s]\n룰: %s\n스코어: %.2f\n%s\n⏰ %s UTC",
		directionIcon(sig.Direction),
		sig.Direction,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Score,
		sig.Message,
		sig.CreatedAt.UTC().Format("2006-01-02 15:04:05"),
	)
}

// discordColor maps signal direction to a Discord embed color (decimal RGB).
func discordColor(direction string) int {
	switch direction {
	case "LONG":
		return 0x00C851 // green
	case "SHORT":
		return 0xFF4444 // red
	default:
		return 0xFFBB33 // amber (neutral / kill zone)
	}
}
