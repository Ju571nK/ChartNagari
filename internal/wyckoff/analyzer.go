// Package wyckoff provides bar-by-bar Wyckoff phase analysis for the chart overlay.
package wyckoff

import (
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// Phase represents the current Wyckoff market phase.
type Phase string

const (
	PhaseAccumulation Phase = "accumulation"
	PhaseMarkup       Phase = "markup"
	PhaseDistribution Phase = "distribution"
	PhaseMarkdown     Phase = "markdown"
	PhaseRanging      Phase = "ranging"
)

// EventType marks a specific bar as a named Wyckoff event.
type EventType string

const (
	EventSpring         EventType = "spring"
	EventUpthrust       EventType = "upthrust"
	EventVolumeAnomaly  EventType = "volume_anomaly"
	EventSwingHighBreak EventType = "swing_high_break"
	EventSwingLowBreak  EventType = "swing_low_break"
)

// Event marks a single bar with a Wyckoff event type.
type Event struct {
	Type      EventType `json:"type"`
	Time      int64     `json:"time"`       // Unix seconds
	BarIndex  int       `json:"bar_index"`
	Price     float64   `json:"price"`      // close price at event
	VolumeRel float64   `json:"volume_rel"` // vol / VOLUME_MA_20 (0 if N/A)
}

// PhaseZone describes a contiguous range of bars that belong to the same phase.
type PhaseZone struct {
	Phase     Phase   `json:"phase"`
	StartTime int64   `json:"start_time"` // Unix seconds
	EndTime   int64   `json:"end_time"`
	PriceLow  float64 `json:"price_low"`
	PriceHigh float64 `json:"price_high"`
}

// Analysis is the full Wyckoff overlay payload for a symbol/timeframe.
type Analysis struct {
	Symbol     string      `json:"symbol"`
	Timeframe  string      `json:"timeframe"`
	Phase      Phase       `json:"phase"`      // current phase (last bar)
	SwingHigh  float64     `json:"swing_high"` // most-recent swing high
	SwingLow   float64     `json:"swing_low"`  // most-recent swing low
	EMA50      float64     `json:"ema_50"`
	Events     []Event     `json:"events"`
	PhaseZones []PhaseZone `json:"phase_zones"`
}

// Analyze scans bars for Wyckoff phase events and returns an Analysis.
// bars must be sorted oldest-first. At least 50 bars are recommended.
func Analyze(symbol, timeframe string, bars []models.OHLCV) Analysis {
	result := Analysis{
		Symbol:    symbol,
		Timeframe: timeframe,
		Phase:     PhaseRanging,
	}
	if len(bars) < 5 {
		return result
	}

	// Compute indicators once on the full bar set.
	indMap := indicator.Compute(map[string][]models.OHLCV{timeframe: bars})

	tfPfx := timeframe + ":"
	result.SwingHigh = indMap[tfPfx+"SWING_HIGH"]
	result.SwingLow = indMap[tfPfx+"SWING_LOW"]
	result.EMA50 = indMap[tfPfx+"EMA_50"]
	ema50 := result.EMA50
	volMA := indMap[tfPfx+"VOLUME_MA_20"]

	// ── Phase detection on the most-recent bar ───────────────────────────────
	last := bars[len(bars)-1]
	swH := result.SwingHigh
	swL := result.SwingLow

	switch {
	case ema50 > 0 && last.Close > ema50 && swH > 0 && last.High > swH:
		result.Phase = PhaseMarkup
	case ema50 > 0 && last.Close > ema50:
		result.Phase = PhaseDistribution
	case ema50 > 0 && last.Close < ema50 && swL > 0 && last.Low < swL:
		result.Phase = PhaseMarkdown
	case ema50 > 0 && last.Close < ema50:
		result.Phase = PhaseAccumulation
	default:
		result.Phase = PhaseRanging
	}

	// ── Bar-by-bar event scan ────────────────────────────────────────────────
	const springLookback = 5
	const upthrustLookback = 5
	const volAnomalyMult = 2.5

	events := make([]Event, 0, 16)

	for i := springLookback; i < len(bars); i++ {
		curr := bars[i]
		timeUnix := curr.OpenTime.Unix()

		// Recalculate rolling indicators on the window up to bar i
		// (heavyweight — use sliding SWING_HIGH/LOW heuristic instead)
		localBars := bars[max(0, i-49) : i+1]
		localInd := indicator.Compute(map[string][]models.OHLCV{timeframe: localBars})
		localSwH := localInd[tfPfx+"SWING_HIGH"]
		localSwL := localInd[tfPfx+"SWING_LOW"]
		localVolMA := localInd[tfPfx+"VOLUME_MA_20"]
		if localVolMA == 0 && volMA > 0 {
			localVolMA = volMA
		}

		// Spring: any of last 4 bars dipped below SWING_LOW, current closes above
		if localSwL > 0 {
			dipped := false
			for j := max(0, i-springLookback+1); j < i; j++ {
				if bars[j].Low < localSwL {
					dipped = true
					break
				}
			}
			if dipped && curr.Close > localSwL && localVolMA > 0 && curr.Volume >= 1.5*localVolMA {
				events = append(events, Event{
					Type:      EventSpring,
					Time:      timeUnix,
					BarIndex:  i,
					Price:     curr.Close,
					VolumeRel: volRel(curr.Volume, localVolMA),
				})
			}
		}

		// Upthrust: any of last 4 bars pierced above SWING_HIGH, current closes below
		if localSwH > 0 {
			pierced := false
			for j := max(0, i-upthrustLookback+1); j < i; j++ {
				if bars[j].High > localSwH {
					pierced = true
					break
				}
			}
			if pierced && curr.Close < localSwH && localVolMA > 0 && curr.Volume >= 1.5*localVolMA {
				events = append(events, Event{
					Type:      EventUpthrust,
					Time:      timeUnix,
					BarIndex:  i,
					Price:     curr.Close,
					VolumeRel: volRel(curr.Volume, localVolMA),
				})
			}
		}

		// Volume anomaly
		if localVolMA > 0 && curr.Volume >= volAnomalyMult*localVolMA {
			events = append(events, Event{
				Type:      EventVolumeAnomaly,
				Time:      timeUnix,
				BarIndex:  i,
				Price:     curr.Close,
				VolumeRel: volRel(curr.Volume, localVolMA),
			})
		}
	}

	result.Events = events

	// ── Phase zones: merge consecutive bars with same phase ──────────────────
	result.PhaseZones = buildPhaseZones(bars, ema50, swH, swL)

	return result
}

// buildPhaseZones assigns a phase to each bar and merges consecutive same-phase runs.
func buildPhaseZones(bars []models.OHLCV, ema50, swH, swL float64) []PhaseZone {
	if len(bars) == 0 {
		return nil
	}

	type barPhase struct {
		ph Phase
		b  models.OHLCV
	}

	phases := make([]barPhase, len(bars))
	for i, b := range bars {
		var ph Phase
		switch {
		case ema50 > 0 && b.Close > ema50 && swH > 0 && b.High > swH:
			ph = PhaseMarkup
		case ema50 > 0 && b.Close > ema50:
			ph = PhaseDistribution
		case ema50 > 0 && b.Close < ema50 && swL > 0 && b.Low < swL:
			ph = PhaseMarkdown
		case ema50 > 0 && b.Close < ema50:
			ph = PhaseAccumulation
		default:
			ph = PhaseRanging
		}
		phases[i] = barPhase{ph: ph, b: b}
	}

	// Merge consecutive same-phase runs.
	zones := make([]PhaseZone, 0, 8)
	start := 0
	for i := 1; i <= len(phases); i++ {
		if i == len(phases) || phases[i].ph != phases[start].ph {
			// Build zone from [start, i)
			lo, hi := phases[start].b.Low, phases[start].b.High
			for j := start + 1; j < i; j++ {
				if phases[j].b.Low < lo {
					lo = phases[j].b.Low
				}
				if phases[j].b.High > hi {
					hi = phases[j].b.High
				}
			}
			zones = append(zones, PhaseZone{
				Phase:     phases[start].ph,
				StartTime: phases[start].b.OpenTime.Unix(),
				EndTime:   phases[i-1].b.OpenTime.Unix(),
				PriceLow:  lo,
				PriceHigh: hi,
			})
			start = i
		}
	}

	return zones
}

func volRel(vol, volMA float64) float64 {
	if volMA == 0 {
		return 0
	}
	return vol / volMA
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
