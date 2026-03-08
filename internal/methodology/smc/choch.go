package smc

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SMCChoCHRule detects Change of Character (CHoCH) — trend reversal signals.
//
// CHoCH occurs when price breaks AGAINST the prior trend's structural level,
// signaling a potential trend reversal.
//
// Algorithm (per TF):
//   - Minimum 30 bars required
//   - priorBars = bars[:len-5]            — exclude recent 5 bars to isolate prior structure
//   - trend = trendDir(priorBars, 20)     — prior trend direction
//   - structBars = priorBars[len-20:]     — 20-bar structural reference window
//   - DOWN trend: curr.Close > structuralHigh(structBars) → LONG  (reversal from downtrend)
//   - UP trend:   curr.Close < structuralLow(structBars)  → SHORT (reversal from uptrend)
//   - rawScore = clamp((distance / level) × 20, 0.1, 1.0)
//
// The highest weighted-score signal across all timeframes is returned.
type SMCChoCHRule struct{}

func (r *SMCChoCHRule) Name() string                 { return "smc_choch" }
func (r *SMCChoCHRule) RequiredIndicators() []string { return nil }

func (r *SMCChoCHRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const minBars = 30
	const recentExclude = 5
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
		priorBars := bars[:n-recentExclude]

		trend := trendDir(priorBars, lookbackN)
		if trend == "NONE" {
			continue
		}

		lbStart := len(priorBars) - lookbackN
		if lbStart < 0 {
			lbStart = 0
		}
		structBars := priorBars[lbStart:]

		var dir string
		var rawScore float64

		// Prior downtrend: break of structural high → bullish reversal
		if trend == "DOWN" {
			sh := structuralHigh(structBars)
			if sh > 0 && curr.Close > sh {
				dir = "LONG"
				rawScore = clamp((curr.Close-sh)/sh*20, 0.1, 1.0)
			}
		}

		// Prior uptrend: break of structural low → bearish reversal
		if trend == "UP" {
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
				Message:   fmt.Sprintf("[%s] SMC CHoCH 감지 → %s (반전 강도: %.2f)", tf, dir, rawScore),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
