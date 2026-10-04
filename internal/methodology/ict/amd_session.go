package ict

import (
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// ICTAMDSessionRule detects Accumulation-Manipulation-Distribution session structures.
//
//  1. Asia session (20:00–00:00 New York): Compute the high/low range.
//  2. London session (02:00–05:00 New York): Detect a breach of the Asian range followed
//     by a close back inside — this is the Manipulation phase.
//  3. New York AM (07:00–10:00 New York): If a manipulation was detected, signal in the
//     opposite direction of the manipulation (Distribution).
//
// Bullish AMD: London breaches Asian low (fake breakdown) → NY move up → LONG
// Bearish AMD: London breaches Asian high (fake breakout) → NY move down → SHORT
type ICTAMDSessionRule struct{}

func (r *ICTAMDSessionRule) Name() string                 { return "ict_amd_session" }
func (r *ICTAMDSessionRule) RequiredIndicators() []string { return nil }

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

		// Use only bars available before the current bar for the Asian range.
		sessions := ctx.Session
		if !sessions.AsianRangeValid && tf == "1H" {
			sessions = market.AsianRange(bars[:n-1], curr.OpenTime)
		}
		if !sessions.NewYork.Contains(curr.OpenTime) {
			continue
		}
		if !sessions.AsianRangeValid {
			continue
		}
		asiaHigh, asiaLow := sessions.AsianHigh, sessions.AsianLow

		// Phase 2: Check London session for manipulation
		// breachLow: London bar dipped below Asian low then closed back inside
		// breachHigh: London bar broke above Asian high then closed back inside
		breachLow := false
		breachHigh := false
		for i := 0; i < n-1; i++ {
			if !sessions.London.Contains(bars[i].OpenTime) {
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
