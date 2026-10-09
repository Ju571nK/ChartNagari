package notifier

import (
	"fmt"
	"html"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const MaxTelegramTemplateLength = 500
const telegramMessageLimit = 4096

func telegramSourceUnits(value string) int {
	units := 0
	for _, r := range value {
		units += utf16.RuneLen(r)
	}
	return units
}

var templateTokens = map[string]struct{}{
	"종목": {}, "시간봉": {}, "방향": {}, "진입가": {}, "TP": {}, "SL": {}, "점수": {}, "근거": {},
	"symbol": {}, "timeframe": {}, "direction": {}, "entry": {}, "tp": {}, "sl": {}, "score": {}, "reason": {},
}

// ValidateTelegramTemplate accepts plain text and a closed set of placeholders.
// Every brace must belong to one supported placeholder.
func ValidateTelegramTemplate(template string) error {
	if utf8.RuneCountInString(template) > MaxTelegramTemplateLength {
		return fmt.Errorf("template exceeds %d Unicode characters", MaxTelegramTemplateLength)
	}
	for i := 0; i < len(template); {
		switch template[i] {
		case '}':
			return fmt.Errorf("malformed placeholder: unexpected closing brace")
		case '{':
			end := strings.IndexByte(template[i+1:], '}')
			if end < 0 {
				return fmt.Errorf("malformed placeholder: missing closing brace")
			}
			name := template[i+1 : i+1+end]
			if strings.ContainsRune(name, '{') {
				return fmt.Errorf("malformed placeholder: nested opening brace")
			}
			if _, ok := templateTokens[name]; !ok {
				return fmt.Errorf("unknown placeholder {%s}", name)
			}
			i += end + 2
		default:
			i++
		}
	}
	return nil
}

func templateValue(sig models.Signal, name string) string {
	switch name {
	case "종목", "symbol":
		return sig.Symbol
	case "시간봉", "timeframe":
		return sig.Timeframe
	case "방향", "direction":
		return sig.Direction
	case "진입가", "entry":
		if sig.EntryPrice <= 0 {
			return "—"
		}
		return formatEntry(sig)
	case "TP", "tp":
		if sig.EntryPrice <= 0 || sig.TP <= 0 {
			return "—"
		}
		return formatLevel(sig, sig.TP)
	case "SL", "sl":
		if sig.EntryPrice <= 0 || sig.SL <= 0 {
			return "—"
		}
		return formatLevel(sig, sig.SL)
	case "점수", "score":
		return fmt.Sprintf("%.2f", sig.Score)
	case "근거", "reason":
		return sig.Message
	default:
		return ""
	}
}

func renderTemplateHTML(sig models.Signal, template string) string {
	var out strings.Builder
	for len(template) > 0 {
		open := strings.IndexByte(template, '{')
		if open < 0 {
			out.WriteString(html.EscapeString(template))
			break
		}
		out.WriteString(html.EscapeString(template[:open]))
		end := strings.IndexByte(template[open+1:], '}')
		name := template[open+1 : open+1+end]
		out.WriteString(html.EscapeString(templateValue(sig, name)))
		template = template[open+end+2:]
	}
	return out.String()
}

// RenderTelegramAlert is the single formatting path used by preview and the
// actual Telegram sender. The existing signal details are retained verbatim.
// If the optional block cannot fit, only that block is omitted.
func RenderTelegramAlert(sig models.Signal, template string) string {
	message, _ := RenderTelegramAlertWithStatus(sig, template)
	return message
}

// RenderTelegramAlertWithStatus also reports whether the custom block fit.
func RenderTelegramAlertWithStatus(sig models.Signal, template string) (string, bool) {
	core := formatTelegram(sig)
	if template == "" || (sig.Direction != "LONG" && sig.Direction != "SHORT") || ValidateTelegramTemplate(template) != nil {
		return core, false
	}
	block := "\n\n<b>Custom note</b>\n" + renderTemplateHTML(sig, template)
	// Count source Unicode characters conservatively: Telegram's parsed HTML
	// text is no longer than the escaped source string.
	if telegramSourceUnits(core)+telegramSourceUnits(block) > telegramMessageLimit {
		return core, false
	}
	return core + block, true
}

// TelegramSampleSummary uses the same price formatting as the alert renderer.
func TelegramSampleSummary(sig models.Signal) string {
	return fmt.Sprintf("%s · %s · entry %s · TP %s · SL %s · score %.2f · %s",
		sig.Symbol, sig.Timeframe, templateValue(sig, "entry"), templateValue(sig, "tp"),
		templateValue(sig, "sl"), sig.Score, sig.Message)
}
