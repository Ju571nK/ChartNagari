package analyst

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var pctRegexp = regexp.MustCompile(`(?i)BULL:\s*(\d+(?:\.\d+)?)%\s*/\s*BEAR:\s*(\d+(?:\.\d+)?)%\s*/\s*SIDEWAYS:\s*(\d+(?:\.\d+)?)%`)

// parsePercentages extracts BULL/BEAR/SIDEWAYS percentages from analyst response text.
func parsePercentages(text string) (bull, bear, sideways float64) {
	for _, line := range strings.Split(text, "\n") {
		m := pctRegexp.FindStringSubmatch(line)
		if len(m) >= 4 {
			bull, _ = strconv.ParseFloat(m[1], 64)
			bear, _ = strconv.ParseFloat(m[2], 64)
			sideways, _ = strconv.ParseFloat(m[3], 64)
			return
		}
	}
	return 0, 0, 0
}

// Aggregate combines multiple AnalystOutputs into a ScenarioResult.
// rsi1D is the 1D RSI_14 value used for RSI correction.
func Aggregate(outputs []AnalystOutput, rsi1D float64) ScenarioResult {
	if len(outputs) == 0 {
		return ScenarioResult{Final: "SIDEWAYS", Confidence: "LOW"}
	}

	var bullSum, bearSum, sidewaysSum float64
	var count int
	var validOutputs []AnalystOutput

	for _, o := range outputs {
		if o.Err != nil || (o.Bull == 0 && o.Bear == 0 && o.Sideways == 0) {
			continue
		}
		bullSum += o.Bull
		bearSum += o.Bear
		sidewaysSum += o.Sideways
		count++
		validOutputs = append(validOutputs, o)
	}

	if count == 0 {
		var errMsgs []string
		for _, o := range outputs {
			if o.Err != nil {
				errMsgs = append(errMsgs, fmt.Sprintf("[%s] %s", o.Name, o.Err.Error()))
			} else if o.Bull == 0 && o.Bear == 0 && o.Sideways == 0 {
				errMsgs = append(errMsgs, fmt.Sprintf("[%s] 확률 파싱 실패 (응답에 BULL/BEAR/SIDEWAYS 없음)", o.Name))
			}
		}
		reason := "모든 애널리스트 호출 실패"
		if len(errMsgs) > 0 {
			reason += ": " + strings.Join(errMsgs, " | ")
		}
		return ScenarioResult{Final: "SIDEWAYS", Confidence: "LOW", AggregatorReason: reason}
	}

	bull := bullSum / float64(count)
	bear := bearSum / float64(count)
	sw := sidewaysSum / float64(count)

	// RSI correction
	reason := fmt.Sprintf("3개 애널리스트 평균 — BULL:%.1f%% BEAR:%.1f%% SIDEWAYS:%.1f%%", bull, bear, sw)
	if rsi1D > 70 {
		bear += 5
		bull -= 2.5
		sw -= 2.5
		reason += fmt.Sprintf(" | RSI1D=%.1f>70 → Bear +5pt 보정", rsi1D)
	} else if rsi1D < 30 {
		bull += 5
		bear -= 2.5
		sw -= 2.5
		reason += fmt.Sprintf(" | RSI1D=%.1f<30 → Bull +5pt 보정", rsi1D)
	}

	bull = clamp(bull, 0, 100)
	bear = clamp(bear, 0, 100)
	sw = clamp(sw, 0, 100)

	// Normalize
	total := bull + bear + sw
	if total > 0 {
		bull = bull / total * 100
		bear = bear / total * 100
		sw = sw / total * 100
	}

	final := "SIDEWAYS"
	maxPct := sw
	if bull > maxPct {
		final = "BULL"
		maxPct = bull
	}
	if bear > maxPct {
		final = "BEAR"
		maxPct = bear
	}

	confidence := "LOW"
	switch {
	case maxPct >= 60:
		confidence = "HIGH"
	case maxPct >= 45:
		confidence = "MEDIUM"
	}

	sr := ScenarioResult{
		BullPct:          roundTo1(bull),
		BearPct:          roundTo1(bear),
		SidewaysPct:      roundTo1(sw),
		Final:            final,
		Confidence:       confidence,
		AggregatorReason: reason,
	}
	for _, o := range validOutputs {
		switch o.Name {
		case "macro":
			sr.MacroText = o.Text
		case "fundamental":
			sr.FundamentalText = o.Text
		case "sentiment":
			sr.SentimentText = o.Text
		}
	}
	return sr
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func roundTo1(v float64) float64 {
	return float64(int(v*10+0.5)) / 10
}
