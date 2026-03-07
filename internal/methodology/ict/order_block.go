package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTOrderBlockRule detects ICT Order Blocks and signals when price returns to them.
//
// Bullish OB: the last bearish candle immediately before an impulse upward move
//   -> price returning to that candle's range -> LONG
// Bearish OB: the last bullish candle immediately before an impulse downward move
//   -> price returning to that candle's range -> SHORT
//
// Simplified: scan last 20 bars. Find bullish OB = last bearish candle where
//   bars[i+1] is bullish and bars[i+2].close > bars[i].open (impulse).
//   If current close within OB range -> LONG.
//   Score = 1.0 for confirmed OB touch.
//
// Requires >= 5 bars per TF.
type ICTOrderBlockRule struct{}

func (r *ICTOrderBlockRule) Name() string                 { return "ict_order_block" }
func (r *ICTOrderBlockRule) RequiredIndicators() []string { return nil }

func (r *ICTOrderBlockRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 5 {
			continue
		}

		n := len(bars)
		current := bars[n-1]

		// Scan the last 20 bars (excluding current bar)
		start := n - 20
		if start < 0 {
			start = 0
		}

		var obLow, obHigh float64
		var dir string

		// Scan from newest to oldest (excluding current bar = index n-1)
		// We need bars[i], bars[i+1], bars[i+2] — so i+2 must be <= n-2
		for i := n - 2 - 2; i >= start; i-- {
			b0 := bars[i]
			b1 := bars[i+1]
			b2 := bars[i+2]

			// Bullish OB: b0 bearish, b1 bullish, b2.close > b0.open (impulse up)
			if b0.Close < b0.Open && b1.Close > b1.Open && b2.Close > b0.Open {
				obLow = b0.Low
				obHigh = b0.High
				if current.Close >= obLow && current.Close <= obHigh {
					dir = "LONG"
					break
				}
			}

			// Bearish OB: b0 bullish, b1 bearish, b2.close < b0.open (impulse down)
			if b0.Close > b0.Open && b1.Close < b1.Open && b2.Close < b0.Open {
				obLow = b0.Low
				obHigh = b0.High
				if current.Close >= obLow && current.Close <= obHigh {
					dir = "SHORT"
					break
				}
			}
		}

		if dir == "" {
			continue
		}

		rawScore := 1.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				Message:   fmt.Sprintf("[%s] ICT Order Block 감지 → %s (OB Zone: %.4f-%.4f)", tf, dir, obLow, obHigh),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
