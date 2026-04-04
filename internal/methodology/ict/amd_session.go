package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTAMDSessionRule detects Accumulation-Manipulation-Distribution session structures.
//
// 1. Asia session (UTC 00:00–07:00): Compute the high/low range.
// 2. London session (UTC 08:00–11:00): Detect a breach of the Asian range followed
//    by a close back inside — this is the Manipulation phase.
// 3. New York session (UTC 13:00+): If a manipulation was detected, signal in the
//    opposite direction of the manipulation (Distribution).
//
// Bullish AMD: London breaches Asian low (fake breakdown) → NY move up → LONG
// Bearish AMD: London breaches Asian high (fake breakout) → NY move down → SHORT
type ICTAMDSessionRule struct{}

func (r *ICTAMDSessionRule) Name() string                 { return "ict_amd_session" }
func (r *ICTAMDSessionRule) RequiredIndicators() []string { return nil }

// sessionOf classifies a bar's UTC hour into a session.
func sessionOf(t time.Time) string {
	h := t.UTC().Hour()
	switch {
	case h >= 0 && h <= 7:
		return "asia"
	case h >= 8 && h <= 11:
		return "london"
	case h >= 13:
		return "newyork"
	default:
		return ""
	}
}

// sameDay checks whether two times fall on the same UTC date.
func sameDay(a, b time.Time) bool {
	ay, am, ad := a.UTC().Date()
	by, bm, bd := b.UTC().Date()
	return ay == by && am == bm && ad == bd
}

func (r *ICTAMDSessionRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) < 3 {
			continue
		}

		n := len(bars)
		curr := bars[n-1]

		// Current bar must be in the New York session
		if sessionOf(curr.OpenTime) != "newyork" {
			continue
		}

		today := curr.OpenTime

		// Phase 1: Gather Asian session range for today
		var asiaHigh, asiaLow float64
		asiaFound := false
		for i := 0; i < n-1; i++ {
			if !sameDay(bars[i].OpenTime, today) {
				continue
			}
			if sessionOf(bars[i].OpenTime) != "asia" {
				continue
			}
			if !asiaFound {
				asiaHigh = bars[i].High
				asiaLow = bars[i].Low
				asiaFound = true
			} else {
				if bars[i].High > asiaHigh {
					asiaHigh = bars[i].High
				}
				if bars[i].Low < asiaLow {
					asiaLow = bars[i].Low
				}
			}
		}
		if !asiaFound {
			continue
		}

		// Phase 2: Check London session for manipulation
		// breachLow: London bar dipped below Asian low then closed back inside
		// breachHigh: London bar broke above Asian high then closed back inside
		breachLow := false
		breachHigh := false
		for i := 0; i < n-1; i++ {
			if !sameDay(bars[i].OpenTime, today) {
				continue
			}
			if sessionOf(bars[i].OpenTime) != "london" {
				continue
			}
			if bars[i].Low < asiaLow && bars[i].Close >= asiaLow {
				breachLow = true
			}
			if bars[i].High > asiaHigh && bars[i].Close <= asiaHigh {
				breachHigh = true
			}
		}

		if !breachLow && !breachHigh {
			continue
		}

		// Phase 3: Determine signal direction
		var dir string
		if breachLow {
			dir = "LONG" // Fake breakdown → expect move up
		}
		if breachHigh {
			dir = "SHORT" // Fake breakout → expect move down (takes priority if both)
		}

		rawScore := 1.0
		weighted := rawScore * tfW[tf]
		if weighted > bestWeighted {
			bestWeighted = weighted
			manipulation := "아시아 하이 돌파"
			if dir == "LONG" {
				manipulation = "아시아 로우 돌파"
			}
			bestSig = &models.Signal{
				Symbol:    ctx.Symbol,
				Timeframe: tf,
				Rule:      r.Name(),
				Direction: dir,
				Score:     rawScore,
				ZoneLow:   asiaLow,
				ZoneHigh:  asiaHigh,
				Message:   fmt.Sprintf("[%s] ICT AMD — 런던 %s (조작) → 뉴욕 %s", tf, manipulation, dir),
				CreatedAt: time.Now(),
			}
		}
	}

	return bestSig, nil
}
