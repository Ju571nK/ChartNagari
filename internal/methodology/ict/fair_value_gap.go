package ict

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTFairValueGapRule detects Fair Value Gaps (3-candle imbalance pattern).
//
// Bullish FVG: bars[i].high < bars[i+2].low (gap between candle i high and candle i+2 low)
//   -> Price entering the gap -> LONG
// Bearish FVG: bars[i].low > bars[i+2].high (gap between candle i low and candle i+2 high)
//   -> Price entering the gap -> SHORT
//
// Scan last 15 bars for FVG patterns. Check if current bar's close is within any gap.
//
// Quality Score (0.1–1.0) is computed from three factors:
//   1. Gap size vs ATR_14 (weight 0.35): larger gap relative to ATR = more significant
//   2. Impulse strength (weight 0.35): volume and body size of the middle candle
//   3. Unfilled duration (weight 0.30): how many bars the gap remained unfilled
//
// Requires >= 4 bars.
type ICTFairValueGapRule struct{}

func (r *ICTFairValueGapRule) Name() string                 { return "ict_fair_value_gap" }
func (r *ICTFairValueGapRule) RequiredIndicators() []string { return []string{"ATR_14", "VOLUME_MA_20"} }

// fvgRelevance computes a 0.1–1.0 relevance score for a Fair Value Gap.
//
//	gapSize:      gap high - gap low (absolute)
//	atr:          ATR_14 value (0 if unavailable → neutral 0.5)
//	impulseBody:  abs(close - open) of the middle candle (bars[i+1])
//	impulseVol:   volume of the middle candle
//	volMA:        VOLUME_MA_20 (0 if unavailable → neutral 0.5 for vol component)
//	unfilledBars: number of bars the gap remained unfilled
func fvgRelevance(gapSize, atr, impulseBody, impulseVol, volMA float64, unfilledBars int) float64 {
	// Factor 1: gap size vs ATR (weight 0.35)
	// A gap equal to 1x ATR scores 1.0; smaller gaps score proportionally less.
	var gapScore float64
	if atr <= 0 {
		gapScore = 0.5 // no ATR data → neutral
	} else {
		gapScore = math.Min(1.0, gapSize/atr)
	}

	// Factor 2: impulse strength (weight 0.35)
	// Combined from volume ratio and body size relative to gap.
	var volScore float64
	if volMA <= 0 || impulseVol <= 0 {
		volScore = 0.5 // no volume data → neutral
	} else {
		volScore = math.Min(1.0, (impulseVol/volMA-0.5)/2.5)
		if volScore < 0 {
			volScore = 0
		}
	}
	var bodyScore float64
	if gapSize > 0 {
		bodyScore = math.Min(1.0, impulseBody/gapSize)
	}
	impulseScore := volScore*0.5 + bodyScore*0.5

	// Factor 3: unfilled duration (weight 0.30)
	// Unfilled for 10+ bars → 1.0; just formed (0 bars) → 0.0
	durScore := math.Min(1.0, float64(unfilledBars)/10.0)

	raw := gapScore*0.35 + impulseScore*0.35 + durScore*0.30

	// Clamp to [0.1, 1.0]
	if raw < 0.1 {
		return 0.1
	}
	if raw > 1.0 {
		return 1.0
	}
	return math.Round(raw*100) / 100
}

func (r *ICTFairValueGapRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 4 {
			continue
		}

		n := len(bars)
		current := bars[n-1]

		// Scan from bars[len-15] to bars[len-3], checking pattern with bars[i], bars[i+1], bars[i+2]
		// i+2 must be <= n-2 (excluding current bar index n-1)
		start := n - 15
		if start < 0 {
			start = 0
		}

		var gapLow, gapHigh float64
		var dir string
		var fvgIdx int // index of bars[i] where FVG was detected

		// Scan from most recent to oldest to find latest valid FVG
		for i := n - 2 - 2; i >= start; i-- {
			b0 := bars[i]
			b2 := bars[i+2]

			// Bullish FVG: b0.high < b2.low, gap = [b0.high, b2.low]
			gl := b0.High
			gh := b2.Low
			if gh > gl && current.Close >= gl && current.Close <= gh {
				gapLow = gl
				gapHigh = gh
				dir = "LONG"
				fvgIdx = i
				break
			}

			// Bearish FVG: b0.low > b2.high, gap = [b2.high, b0.low]
			gl2 := b2.High
			gh2 := b0.Low
			if gh2 > gl2 && current.Close >= gl2 && current.Close <= gh2 {
				gapLow = gl2
				gapHigh = gh2
				dir = "SHORT"
				fvgIdx = i
				break
			}
		}

		if dir == "" {
			continue
		}

		// Compute FVG relevance score
		gapSize := gapHigh - gapLow
		impulseCandle := bars[fvgIdx+1]
		impulseBody := math.Abs(impulseCandle.Close - impulseCandle.Open)
		impulseVol := impulseCandle.Volume

		// Count unfilled bars: from i+2 to n-2, count bars where gap was NOT filled
		unfilledBars := 0
		for j := fvgIdx + 2; j <= n-2; j++ {
			b := bars[j]
			filled := false
			if dir == "LONG" {
				// Bullish FVG filled if bar closes below gap low
				filled = b.Close < gapLow
			} else {
				// Bearish FVG filled if bar closes above gap high
				filled = b.Close > gapHigh
			}
			if !filled {
				unfilledBars++
			}
		}

		atr, _ := ctx.Indicators[tf+":ATR_14"]
		volMA, _ := ctx.Indicators[tf+":VOLUME_MA_20"]

		rawScore := fvgRelevance(gapSize, atr, impulseBody, impulseVol, volMA, unfilledBars)
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] ICT FVG 진입 → %s (Gap: %.4f-%.4f, 품질: %.0f%%)", tf, dir, gapLow, gapHigh, rawScore*100),
				ZoneLow:   gapLow,
				ZoneHigh:  gapHigh,
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
