package pipeline

import (
	"fmt"
	"math"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// applyVolumeProfileBoost adjusts signal scores based on proximity to
// Volume Profile levels (HVN, LVN, POC). Each rule type has different
// VP interactions:
//
//   - Liquidity Sweep + HVN: +15% (strong S/R confirms sweep reversal)
//   - FVG + LVN: +15% (both indicate fast-move zone)
//   - FVG + POC/HVN: -15% (high-volume zone contradicts gap logic)
//   - Order Block + HVN: +20% (institutional volume confirms OB zone)
//   - Order Block + POC: +10%
//   - Order Block + LVN: -10% (thin volume weakens OB reliability)
func applyVolumeProfileBoost(signals []models.Signal, indicators map[string]float64) {
	for i := range signals {
		sig := &signals[i]
		tf := sig.Timeframe

		atr, hasATR := indicators[tf+":ATR_14"]
		if !hasATR || atr <= 0 {
			continue // no ATR → skip VP boost
		}

		poc := indicators[tf+":VP_POC"]
		hvns := collectVPLevels(indicators, tf, "VP_HVN", 3)
		lvns := collectVPLevels(indicators, tf, "VP_LVN", 3)

		if len(hvns) == 0 && len(lvns) == 0 && poc == 0 {
			continue // no VP data
		}

		switch sig.Rule {
		case "ict_liquidity_sweep":
			// Sweep level approximated by entry price (close at signal)
			level := sig.EntryPrice
			if level == 0 {
				continue
			}
			if nearAny(level, hvns, 0.5*atr) {
				sig.Score *= 1.15
			}

		case "ict_fair_value_gap":
			if sig.ZoneLow == 0 && sig.ZoneHigh == 0 {
				continue
			}
			gapMid := (sig.ZoneLow + sig.ZoneHigh) / 2

			if nearAny(gapMid, lvns, 0.3*atr) {
				sig.Score *= 1.15 // LVN confirms fast-move zone
			} else if nearAny(gapMid, hvns, 0.3*atr) || near(gapMid, poc, 0.3*atr) {
				sig.Score *= 0.85 // high-volume zone contradicts gap
			}

		case "ict_order_block":
			if sig.ZoneLow == 0 && sig.ZoneHigh == 0 {
				continue
			}
			if zoneOverlapsAny(sig.ZoneLow, sig.ZoneHigh, hvns, 0.5*atr) {
				sig.Score *= 1.20 // HVN confirms institutional zone
			} else if zoneContains(sig.ZoneLow, sig.ZoneHigh, poc, 0.5*atr) {
				sig.Score *= 1.10 // POC inside OB
			} else if zoneOverlapsAny(sig.ZoneLow, sig.ZoneHigh, lvns, 0.5*atr) {
				sig.Score *= 0.90 // thin volume weakens OB
			}
		}
	}
}

// collectVPLevels gathers VP_HVN_1, VP_HVN_2, etc. from indicators.
func collectVPLevels(indicators map[string]float64, tf, prefix string, count int) []float64 {
	var levels []float64
	for j := 1; j <= count; j++ {
		key := fmt.Sprintf("%s:%s_%d", tf, prefix, j)
		if v, ok := indicators[key]; ok && v > 0 {
			levels = append(levels, v)
		}
	}
	return levels
}

// near checks if a is within threshold of b.
func near(a, b, threshold float64) bool {
	return math.Abs(a-b) <= threshold
}

// nearAny checks if level is within threshold of any value in targets.
func nearAny(level float64, targets []float64, threshold float64) bool {
	for _, t := range targets {
		if near(level, t, threshold) {
			return true
		}
	}
	return false
}

// zoneOverlapsAny checks if the zone [low, high] is within threshold of any target.
func zoneOverlapsAny(low, high float64, targets []float64, threshold float64) bool {
	for _, t := range targets {
		if t >= low-threshold && t <= high+threshold {
			return true
		}
	}
	return false
}

// zoneContains checks if target is inside or near the zone [low, high].
func zoneContains(low, high, target, threshold float64) bool {
	return target >= low-threshold && target <= high+threshold
}
