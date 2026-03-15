// Package history compresses a long OHLCV price history into a compact text
// summary suitable for inclusion in an AI prompt (~600 tokens).
package history

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// Summarizer compresses OHLCV history into compact text for AI prompts.
type Summarizer struct{}

// New creates a new Summarizer.
func New() *Summarizer { return &Summarizer{} }

// Summarize converts daily bars into a compact text summary.
func (s *Summarizer) Summarize(symbol string, bars []models.OHLCV) string {
	if len(bars) == 0 {
		return fmt.Sprintf("%s: 데이터 없음", symbol)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s 가격 이력 요약 (%d개 일봉):\n\n", symbol, len(bars)))

	// Yearly returns
	sb.WriteString("연도별 수익률:\n")
	sb.WriteString(yearlyReturns(bars))

	// Quarterly groups
	groups := groupByQuarter(bars)
	keys := sortedKeys(groups)

	var older, recent []string
	if len(keys) > 20 {
		older = keys[:len(keys)-20]
		recent = keys[len(keys)-20:]
	} else {
		recent = keys
	}

	if len(older) > 0 {
		sb.WriteString("\n과거 연도별 (압축):\n")
		yearMap := make(map[int][2]float64)
		for _, k := range older {
			q := groups[k]
			year := q[0].OpenTime.Year()
			if _, exists := yearMap[year]; !exists {
				yearMap[year] = [2]float64{q[0].Open, q[len(q)-1].Close}
			} else {
				prev := yearMap[year]
				yearMap[year] = [2]float64{prev[0], q[len(q)-1].Close}
			}
		}
		yearKeys := make([]int, 0, len(yearMap))
		for y := range yearMap {
			yearKeys = append(yearKeys, y)
		}
		sort.Ints(yearKeys)
		for _, y := range yearKeys {
			v := yearMap[y]
			ret := (v[1] - v[0]) / v[0] * 100
			sb.WriteString(fmt.Sprintf("  %d: %.0f→%.0f (%+.1f%%)\n", y, v[0], v[1], ret))
		}
	}

	sb.WriteString("\n최근 5년 분기별 시세:\n")
	for _, k := range recent {
		q := groups[k]
		open := q[0].Open
		close := q[len(q)-1].Close
		high := maxHigh(q)
		low := minLow(q)
		ret := (close - open) / open * 100
		sb.WriteString(fmt.Sprintf("  %s: %.2f→%.2f (H:%.2f L:%.2f %+.1f%%)\n",
			k, open, close, high, low, ret))
	}

	// Top 5 swing highs/lows
	highs, lows := topSwings(bars, 5)
	sb.WriteString("\n주요 스윙 포인트:\n  고점: ")
	for i, p := range highs {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("$%.2f(%s)", p.price, p.label))
	}
	sb.WriteString("\n  저점: ")
	for i, p := range lows {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(fmt.Sprintf("$%.2f(%s)", p.price, p.label))
	}
	sb.WriteString("\n")

	sb.WriteString("\n현재 국면: " + currentPhase(bars) + "\n")
	return sb.String()
}

func yearlyReturns(bars []models.OHLCV) string {
	type yearData struct {
		firstOpen float64
		lastClose float64
	}
	ym := make(map[int]*yearData)
	for _, b := range bars {
		y := b.OpenTime.Year()
		if _, ok := ym[y]; !ok {
			ym[y] = &yearData{firstOpen: b.Open}
		}
		ym[y].lastClose = b.Close
	}
	years := make([]int, 0, len(ym))
	for y := range ym {
		years = append(years, y)
	}
	sort.Ints(years)

	var sb strings.Builder
	for i, y := range years {
		d := ym[y]
		ret := (d.lastClose - d.firstOpen) / d.firstOpen * 100
		sb.WriteString(fmt.Sprintf("  %d: %+.1f%%", y, ret))
		if (i+1)%5 == 0 {
			sb.WriteString("\n")
		} else {
			sb.WriteString(" | ")
		}
	}
	sb.WriteString("\n")
	return sb.String()
}

type swingPoint struct {
	price float64
	label string
}

func topSwings(bars []models.OHLCV, n int) (highs, lows []swingPoint) {
	type indexedBar struct {
		price float64
		label string
	}
	allHighs := make([]indexedBar, len(bars))
	allLows := make([]indexedBar, len(bars))
	for i, b := range bars {
		q := fmt.Sprintf("%dQ%d", b.OpenTime.Year(), (int(b.OpenTime.Month())-1)/3+1)
		allHighs[i] = indexedBar{price: b.High, label: q}
		allLows[i] = indexedBar{price: b.Low, label: q}
	}
	sort.Slice(allHighs, func(i, j int) bool { return allHighs[i].price > allHighs[j].price })
	sort.Slice(allLows, func(i, j int) bool { return allLows[i].price < allLows[j].price })

	if n > len(bars) {
		n = len(bars)
	}
	for i := 0; i < n; i++ {
		highs = append(highs, swingPoint{price: allHighs[i].price, label: allHighs[i].label})
		lows = append(lows, swingPoint{price: allLows[i].price, label: allLows[i].label})
	}
	return
}

func groupByQuarter(bars []models.OHLCV) map[string][]models.OHLCV {
	groups := make(map[string][]models.OHLCV)
	for _, b := range bars {
		q := fmt.Sprintf("%d-Q%d", b.OpenTime.Year(), (int(b.OpenTime.Month())-1)/3+1)
		groups[q] = append(groups[q], b)
	}
	return groups
}

func sortedKeys(m map[string][]models.OHLCV) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func maxHigh(bars []models.OHLCV) float64 {
	v := math.Inf(-1)
	for _, b := range bars {
		if b.High > v {
			v = b.High
		}
	}
	return v
}

func minLow(bars []models.OHLCV) float64 {
	v := math.Inf(1)
	for _, b := range bars {
		if b.Low < v {
			v = b.Low
		}
	}
	return v
}

func currentPhase(bars []models.OHLCV) string {
	if len(bars) < 50 {
		return "데이터 부족"
	}
	last := bars[len(bars)-1]
	prev50 := bars[len(bars)-50]
	change := (last.Close - prev50.Close) / prev50.Close * 100

	start := len(bars) - 252
	if start < 0 {
		start = 0
	}
	high52 := math.Inf(-1)
	low52 := math.Inf(1)
	for _, b := range bars[start:] {
		if b.High > high52 {
			high52 = b.High
		}
		if b.Low < low52 {
			low52 = b.Low
		}
	}
	distFromHigh := (high52 - last.Close) / high52 * 100
	distFromLow := (last.Close - low52) / low52 * 100

	phase := fmt.Sprintf("현재가 $%.2f (52W H:$%.2f L:$%.2f, 고점대비 -%.1f%%, 저점대비 +%.1f%%, 50일변화 %+.1f%%)",
		last.Close, high52, low52, distFromHigh, distFromLow, change)

	switch {
	case distFromHigh < 5:
		phase += " → 고점 근접 (분배 구간 가능)"
	case distFromLow < 10:
		phase += " → 저점 근접 (축적 구간 가능)"
	case change > 10:
		phase += " → 상승 추세"
	case change < -10:
		phase += " → 하락 추세"
	default:
		phase += " → 횡보/조정 구간"
	}
	return phase
}
