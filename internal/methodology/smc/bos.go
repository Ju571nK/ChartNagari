package smc

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SMCBOSRule detects Break of Structure (BOS) — trend continuation signals.
//
// BOS occurs when price breaks through a structural high (in an uptrend) or
// structural low (in a downtrend), confirming that the existing trend continues.
//
// Algorithm (per TF):
//   - Minimum 30 bars required
//   - trend = trendDir(bars, 20)           — last 20 bars including current
//   - lookbackBars = bars[:len-1]          — exclude current bar
//   - structBars = lookbackBars[len-20:]   — 20-bar structural reference window
//   - UP:   curr.Close > structuralHigh(structBars) → LONG
//   - DOWN: curr.Close < structuralLow(structBars)  → SHORT
//   - rawScore = clamp((distance / level) × 20, 0.1, 1.0)
//
// The highest weighted-score signal across all timeframes is returned.
type SMCBOSRule struct{}

func (r *SMCBOSRule) Name() string                 { return "smc_bos" }
func (r *SMCBOSRule) RequiredIndicators() []string { return nil }

func (r *SMCBOSRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const minBars = 30
	const lookbackN = 20

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < minBars {
			continue
		}

		n := len(bars)
		curr := bars[n-1]
		lookbackBars := bars[:n-1]

		trend := trendDir(bars, lookbackN)
		if trend == "NONE" {
			continue
		}

		lbStart := len(lookbackBars) - lookbackN
		if lbStart < 0 {
			lbStart = 0
		}
		structBars := lookbackBars[lbStart:]

		var dir string
		var rawScore float64

		if trend == "UP" {
			sh := structuralHigh(structBars)
			if sh > 0 && curr.Close > sh {
				dir = "LONG"
				rawScore = clamp((curr.Close-sh)/sh*20, 0.1, 1.0)
			}
		}

		if trend == "DOWN" {
			sl := structuralLow(structBars)
			if sl > 0 && curr.Close < sl {
				dir = "SHORT"
				rawScore = clamp((sl-curr.Close)/sl*20, 0.1, 1.0)
			}
		}

		if dir == "" {
			continue
		}

		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] SMC BOS 감지 → %s (돌파 강도: %.2f)", tf, dir, rawScore),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}

// clamp restricts v to [min, max].
func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
