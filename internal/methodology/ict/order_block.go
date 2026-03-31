package ict

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// Compile-time interface compliance check.
// If ICTOrderBlockRule is missing any AnalysisRule method, this line will fail to compile.
var _ rule.AnalysisRule = (*ICTOrderBlockRule)(nil)

// ICTOrderBlockRule detects ICT Order Blocks and signals when price returns to them.
//
// Bullish OB: the last bearish candle immediately before an impulse upward move
//   -> price returning to that candle's range -> LONG
// Bearish OB: the last bullish candle immediately before an impulse downward move
//   -> price returning to that candle's range -> SHORT
//
// Enhancements:
//   - Mitigation tracking: if price has already revisited and closed through the OB zone,
//     the OB is considered "mitigated" and will not signal again.
//   - Impulse strength filter: the impulse move (bars[i+1], bars[i+2]) must have a combined
//     body size >= 1.5x ATR_14. Skipped if ATR_14 is not available.
//
// Requires >= 5 bars per TF.
type ICTOrderBlockRule struct{}

func (r *ICTOrderBlockRule) Name() string                 { return "ict_order_block" }
func (r *ICTOrderBlockRule) RequiredIndicators() []string { return []string{"ATR_14"} }

// isMitigated checks whether an OB zone has been mitigated by any bar between
// the OB formation and the current bar.
//
// Bullish OB is mitigated if any bar's close < obLow (price closed below the zone).
// Bearish OB is mitigated if any bar's close > obHigh (price closed above the zone).
func isMitigated(bars []models.OHLCV, fromIdx, toIdx int, obLow, obHigh float64, dir string) bool {
	for j := fromIdx; j <= toIdx; j++ {
		if dir == "LONG" && bars[j].Close < obLow {
			return true
		}
		if dir == "SHORT" && bars[j].Close > obHigh {
			return true
		}
	}
	return false
}

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

		atr, hasATR := ctx.Indicators[tf+":ATR_14"]

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

			candidateDir := ""
			candidateLow := b0.Low
			candidateHigh := b0.High

			// Bullish OB: b0 bearish, b1 bullish, b2.close > b0.open (impulse up)
			if b0.Close < b0.Open && b1.Close > b1.Open && b2.Close > b0.Open {
				if current.Close >= candidateLow && current.Close <= candidateHigh {
					candidateDir = "LONG"
				}
			}

			// Bearish OB: b0 bullish, b1 bearish, b2.close < b0.open (impulse down)
			if candidateDir == "" && b0.Close > b0.Open && b1.Close < b1.Open && b2.Close < b0.Open {
				if current.Close >= candidateLow && current.Close <= candidateHigh {
					candidateDir = "SHORT"
				}
			}

			if candidateDir == "" {
				continue
			}

			// Impulse strength filter: combined body of b1+b2 must be >= 1.5x ATR
			// Skip this filter if ATR is not available (allow signal through)
			if hasATR && atr > 0 {
				combinedBody := math.Abs(b1.Close-b1.Open) + math.Abs(b2.Close-b2.Open)
				if combinedBody < 1.5*atr {
					continue // impulse too weak
				}
			}

			// Mitigation check: has the OB zone been revisited and closed through
			// between formation (i+3) and the bar before current (n-2)?
			mitigationStart := i + 3
			mitigationEnd := n - 2
			if mitigationStart <= mitigationEnd {
				if isMitigated(bars, mitigationStart, mitigationEnd, candidateLow, candidateHigh, candidateDir) {
					continue // OB already mitigated
				}
			}

			obLow = candidateLow
			obHigh = candidateHigh
			dir = candidateDir
			break
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
