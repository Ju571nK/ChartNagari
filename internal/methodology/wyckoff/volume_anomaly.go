package wyckoff

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// WyckoffVolumeAnomalyRule detects abnormal volume indicating institutional activity.
//
// Trigger: current volume ≥ 2.5× VOLUME_MA_20
// Direction: Close > Open → LONG (institutional buying), else → SHORT
// Score = clamp((vol/volMA - 2.5)/2.5 + 0.1, 0.1, 1.0)
// Requires VOLUME_MA_20 and ≥ 1 bar.
type WyckoffVolumeAnomalyRule struct{}

func (r *WyckoffVolumeAnomalyRule) Name() string                 { return "wyckoff_volume_anomaly" }
func (r *WyckoffVolumeAnomalyRule) RequiredIndicators() []string { return nil }

func (r *WyckoffVolumeAnomalyRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	const threshold = 2.5

	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	bestScore := 0.0
	bestTF := ""
	bestDir := ""
	bestRatio := 0.0

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 1 {
			continue
		}

		volMAKey := tf + ":VOLUME_MA_20"
		volMA, hasVolMA := ctx.Indicators[volMAKey]
		if !hasVolMA || volMA == 0 {
			continue
		}

		curr := bars[len(bars)-1]
		ratio := curr.Volume / volMA

		if ratio < threshold {
			continue
		}

		rawScore := math.Max(0.1, math.Min(1.0, (ratio-threshold)/threshold+0.1))
		weighted := rawScore * tfW[tf]

		if weighted > bestScore {
			bestScore = weighted
			bestTF = tf
			bestRatio = ratio
			if curr.Close > curr.Open {
				bestDir = "LONG"
			} else {
				bestDir = "SHORT"
			}
		}
	}

	if bestTF == "" {
		return nil, nil
	}

	bars := ctx.Timeframes[bestTF]
	curr := bars[len(bars)-1]
	volMA := ctx.Indicators[bestTF+":VOLUME_MA_20"]
	ratio := curr.Volume / volMA
	rawScore := math.Max(0.1, math.Min(1.0, (ratio-threshold)/threshold+0.1))

	return &models.Signal{
		Symbol:    ctx.Symbol,
		Timeframe: bestTF,
		Rule:      r.Name(),
		Direction: bestDir,
		Score:     rawScore,
		Message:   fmt.Sprintf("[%s] Wyckoff 거래량 이상 (%.1fx MA) → %s", bestTF, bestRatio, bestDir),
		CreatedAt: time.Now(),
	}, nil
}
