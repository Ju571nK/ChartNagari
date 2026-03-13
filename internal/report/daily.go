// Package report generates and sends daily stock reports via Telegram.
package report

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ReportStore is satisfied by *storage.DB
type ReportStore interface {
	GetSignalsByDate(symbol string, date time.Time) ([]models.Signal, error)
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
}

// Notifier is satisfied by *notifier.Notifier
type Notifier interface {
	Announce(ctx context.Context, message string)
}

// DailyReporter generates and sends the daily stock report.
type DailyReporter struct {
	store     ReportStore
	notifier  Notifier
	stockSyms []string // stock-only symbols (no crypto)
	log       zerolog.Logger
}

// NewDailyReporter creates a DailyReporter.
func NewDailyReporter(store ReportStore, notifier Notifier, stockSymbols []string, log zerolog.Logger) *DailyReporter {
	return &DailyReporter{
		store:     store,
		notifier:  notifier,
		stockSyms: stockSymbols,
		log:       log,
	}
}

// symbolReport holds per-symbol data for one report run.
type symbolReport struct {
	symbol    string
	signals   []models.Signal
	closePrice float64
	prevClose  float64
	hasOHLCV  bool
}

// Generate builds and sends the daily report for the given date.
func (r *DailyReporter) Generate(ctx context.Context, cfg appconfig.DailyReportConfig, date time.Time) error {
	if len(r.stockSyms) == 0 {
		r.log.Info().Msg("일일 리포트: 주식 종목 없음 — 스킵")
		return nil
	}

	reports := make([]symbolReport, 0, len(r.stockSyms))

	for _, sym := range r.stockSyms {
		sr := symbolReport{symbol: sym}

		// 신호 조회
		sigs, err := r.store.GetSignalsByDate(sym, date)
		if err != nil {
			r.log.Warn().Err(err).Str("symbol", sym).Msg("신호 조회 실패")
		} else {
			sr.signals = sigs
		}

		// OHLCV 조회 (최근 2개: 오늘 + 전날)
		bars, err := r.store.GetOHLCV(sym, "1D", 2)
		if err != nil || len(bars) == 0 {
			r.log.Warn().Err(err).Str("symbol", sym).Msg("OHLCV 조회 실패 — graceful skip")
		} else {
			sr.hasOHLCV = true
			sr.closePrice = bars[0].Close // DESC 순서이므로 첫 번째가 가장 최근
			if len(bars) >= 2 {
				sr.prevClose = bars[1].Close
			} else {
				sr.prevClose = bars[0].Close
			}
		}

		reports = append(reports, sr)
	}

	// only_if_signals=true 이고 모든 종목에 신호 없으면 스킵
	if cfg.OnlyIfSignals {
		hasAny := false
		for _, sr := range reports {
			if len(sr.signals) > 0 {
				hasAny = true
				break
			}
		}
		if !hasAny {
			r.log.Info().Msg("일일 리포트: 신호 없음 — only_if_signals=true로 스킵")
			return nil
		}
	}

	var message string
	if cfg.Compact {
		message = buildCompactMessage(reports, date)
	} else {
		message = buildNormalMessage(reports, date, cfg)
	}

	// 4000자 초과 시 분할 발송
	chunks := splitMessage(message)
	for _, chunk := range chunks {
		r.notifier.Announce(ctx, chunk)
	}

	r.log.Info().
		Str("date", date.Format("2006-01-02")).
		Int("symbols", len(reports)).
		Int("chunks", len(chunks)).
		Msg("일일 리포트 발송 완료")

	return nil
}

// buildNormalMessage constructs the full-format Telegram report message.
func buildNormalMessage(reports []symbolReport, date time.Time, cfg appconfig.DailyReportConfig) string {
	var sb strings.Builder

	dateStr := date.Format("2006-01-02")
	sb.WriteString(fmt.Sprintf("📅 *일일 리포트 — %s*\n", dateStr))
	sb.WriteString("미국 주식 종가 기준 | KST " + cfg.Time + "\n\n")
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━\n")

	bullTotal := 0
	bearTotal := 0
	neutralTotal := 0

	for _, sr := range reports {
		bullCount, bearCount := countDirections(sr.signals)
		dirEmoji := directionEmoji(bullCount, bearCount)

		if len(sr.signals) > 0 {
			sb.WriteString("\n")
			// 가격 변동
			priceStr := formatPriceChange(sr)
			sb.WriteString(fmt.Sprintf("%s *%s* %s\n", dirEmoji, sr.symbol, priceStr))
			sb.WriteString(fmt.Sprintf("  신호: 🟢 BULL ×%d / 🔴 BEAR ×%d\n", bullCount, bearCount))

			// 최강 신호
			best := bestSignal(sr.signals)
			if best != nil {
				sb.WriteString(fmt.Sprintf("  최강: %s (Score %.1f)\n", best.Rule, best.Score))
			}
			sb.WriteString("\n─────────────────────\n")

			if bullCount > bearCount {
				bullTotal++
			} else if bearCount > bullCount {
				bearTotal++
			} else {
				neutralTotal++
			}
		} else {
			maxScore := maxSignalScore(sr.signals)
			sb.WriteString(fmt.Sprintf("\n%s *%s*  무신호 (Score 최대 %.1f)\n", dirEmoji, sr.symbol, maxScore))
			neutralTotal++
		}
	}

	sb.WriteString("\n━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("📊 요약: BULL %d / NEUTRAL %d\n", bullTotal, neutralTotal))
	sb.WriteString("⚙️ Chartter | 다음 리포트: 내일 " + cfg.Time)

	_ = bearTotal
	return sb.String()
}

// buildCompactMessage constructs the compact-format Telegram report message.
func buildCompactMessage(reports []symbolReport, date time.Time) string {
	var sb strings.Builder

	dateStr := date.Format("2006-01-02")
	sb.WriteString(fmt.Sprintf("📅 일일 리포트 — %s\n", dateStr))

	for _, sr := range reports {
		bullCount, bearCount := countDirections(sr.signals)

		// 가격 방향 기호
		priceArrow := "─"
		pct := 0.0
		if sr.hasOHLCV && sr.prevClose > 0 {
			pct = (sr.closePrice - sr.prevClose) / sr.prevClose * 100
			if pct > 0.05 {
				priceArrow = "▲"
			} else if pct < -0.05 {
				priceArrow = "▼"
			}
		}

		pctStr := fmt.Sprintf("%+.1f%%", pct)
		if len(sr.signals) == 0 {
			sb.WriteString(fmt.Sprintf("%s %s%s ⚪ 무신호\n", sr.symbol, priceArrow, pctStr))
		} else {
			sb.WriteString(fmt.Sprintf("%s %s%s 🟢×%d 🔴×%d\n", sr.symbol, priceArrow, pctStr, bullCount, bearCount))
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// splitMessage splits a message into chunks of at most 4000 characters.
// It splits on the ─────────────────────  separator lines.
func splitMessage(message string) []string {
	if len(message) <= 4000 {
		return []string{message}
	}

	const separator = "─────────────────────"
	parts := strings.Split(message, separator)
	var chunks []string
	current := ""

	for i, part := range parts {
		candidate := current
		if i > 0 {
			candidate += separator
		}
		candidate += part

		if len(candidate) > 4000 && current != "" {
			chunks = append(chunks, strings.TrimRight(current, "\n"))
			current = part
		} else {
			current = candidate
		}
	}

	if strings.TrimSpace(current) != "" {
		chunks = append(chunks, strings.TrimRight(current, "\n"))
	}

	return chunks
}

// countDirections returns the number of BULL (LONG) and BEAR (SHORT) signals.
func countDirections(signals []models.Signal) (bull, bear int) {
	for _, s := range signals {
		switch s.Direction {
		case "LONG":
			bull++
		case "SHORT":
			bear++
		}
	}
	return
}

// directionEmoji returns the emoji for the net direction.
func directionEmoji(bull, bear int) string {
	if bull > bear {
		return "📈"
	}
	if bear > bull {
		return "📉"
	}
	return "⚪"
}

// formatPriceChange returns "+1.8% ($214.50)" or similar.
func formatPriceChange(sr symbolReport) string {
	if !sr.hasOHLCV {
		return "(데이터 없음)"
	}
	pct := 0.0
	if sr.prevClose > 0 {
		pct = (sr.closePrice - sr.prevClose) / sr.prevClose * 100
	}
	return fmt.Sprintf("%+.1f%% ($%.2f)", pct, sr.closePrice)
}

// bestSignal returns the highest-scoring signal.
func bestSignal(signals []models.Signal) *models.Signal {
	if len(signals) == 0 {
		return nil
	}
	best := &signals[0]
	for i := range signals[1:] {
		if signals[i+1].Score > best.Score {
			best = &signals[i+1]
		}
	}
	return best
}

// maxSignalScore returns the maximum score in the list, or 0.0 if empty.
func maxSignalScore(signals []models.Signal) float64 {
	max := 0.0
	for _, s := range signals {
		if s.Score > max {
			max = s.Score
		}
	}
	return max
}
