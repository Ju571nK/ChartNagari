package general_ta

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// VolumeSpikeRule signals when volume is ≥ 2× the 20-bar volume MA.
//
// Volume spike + close > open → LONG (bullish)
// Volume spike + close < open → SHORT (bearish)
// Requires VOLUME_MA_20 in indicators and ≥1 bar in TF.
type VolumeSpikeRule struct{}

func (r *VolumeSpikeRule) Name() string                 { return "volume_spike" }
func (r *VolumeSpikeRule) RequiredIndicators() []string { return nil }

func (r *VolumeSpikeRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		volMA, ok := ctx.Indicators[tf+":VOLUME_MA_20"]
		if !ok || volMA == 0 {
			continue
		}

		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) == 0 {
			continue
		}

		curr := bars[len(bars)-1]
		if curr.Volume < 2.0*volMA {
			continue
		}

		var dir string
		if curr.Close > curr.Open {
			dir = "LONG"
		} else {
			dir = "SHORT"
		}

		ratio := curr.Volume / volMA
		rawScore := (ratio-2.0)/3.0 + 0.1
		if rawScore < 0.1 {
			rawScore = 0.1
		} else if rawScore > 1.0 {
			rawScore = 1.0
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
				Message:   fmt.Sprintf("[%s] 거래량 급등 (%.1fx MA) → %s", tf, ratio, dir),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
