package notifier

import (
	"fmt"
	"strconv"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// fmtPrice formats a price value with context-appropriate decimal places.
// Precision scales with price magnitude to avoid float64 noise:
//
//	>= 100  → 2 dp  (e.g. 157.16)
//	>= 10   → 3 dp  (e.g. 12.345)
//	>= 1    → 4 dp  (e.g. 1.2345)
//	>= 0.01 → 6 dp  (e.g. 0.012345)
//	< 0.01  → 8 dp  (e.g. 0.00012345)
func fmtPrice(p float64) string {
	var prec int
	switch {
	case p >= 100:
		prec = 2
	case p >= 10:
		prec = 3
	case p >= 1:
		prec = 4
	case p >= 0.01:
		prec = 6
	default:
		prec = 8
	}
	return strconv.FormatFloat(p, 'f', prec, 64)
}

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
// Includes ATR-based entry/TP/SL when available (EntryPrice > 0).
// When sig.AIInterpretation is set, it is appended as an italic paragraph.
func formatTelegram(sig models.Signal) string {
	text := fmt.Sprintf(
		"%s <b>%s</b> — %s [%s]\nRule: %s\nScore: %.2f\n%s",
		directionIcon(sig.Direction),
		sig.Direction,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Score,
		sig.Message,
	)
	if sig.EntryPrice > 0 {
		text += fmt.Sprintf(
			"\n💰 Entry: <b>%s</b>  |  TP: <b>%s</b>  |  SL: <b>%s</b>",
			fmtPrice(sig.EntryPrice), fmtPrice(sig.TP), fmtPrice(sig.SL),
		)
	}
	text += "\n⏰ " + sig.CreatedAt.UTC().Format("2006-01-02 15:04:05") + " UTC"
	if sig.AIInterpretation != "" {
		text += "\n\n💡 <i>" + sig.AIInterpretation + "</i>"
	}
	return text
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
