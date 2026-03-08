package backtest

import "math"

// Stats holds aggregated performance metrics for a completed backtest run.
type Stats struct {
	WinRate         float64 `json:"win_rate"`          // 승률 0.0–1.0
	AvgRR           float64 `json:"avg_rr"`            // 평균 손익비 (avgWin / avgLoss)
	ProfitFactor    float64 `json:"profit_factor"`     // 총이익 / 총손실
	MaxDrawdown     float64 `json:"max_drawdown"`      // 최대낙폭 0.0–1.0
	Sharpe          float64 `json:"sharpe"`            // 샤프비율 (거래 횟수 기준)
	TotalReturnPct  float64 `json:"total_return_pct"`  // 누적 수익률 %
	MaxConsecLosses int     `json:"max_consec_losses"` // 최대 연속 손실 횟수
}

// ComputeStats derives all performance statistics from trade outcomes.
// Returns zero-value Stats when outcomes is empty.
func ComputeStats(outcomes []TradeOutcome) Stats {
	if len(outcomes) == 0 {
		return Stats{}
	}

	var wins int
	var grossWin, grossLoss float64
	var winReturns, lossReturns, allReturns []float64

	for _, o := range outcomes {
		allReturns = append(allReturns, o.PnLPct)
		if o.Win {
			wins++
			grossWin += o.PnLPct
			winReturns = append(winReturns, o.PnLPct)
		} else {
			grossLoss += math.Abs(o.PnLPct)
			lossReturns = append(lossReturns, math.Abs(o.PnLPct))
		}
	}

	n := len(outcomes)
	s := Stats{}

	// Win rate
	s.WinRate = float64(wins) / float64(n)

	// Average R:R (avgWin / avgLoss)
	avgWin := meanSlice(winReturns)
	avgLoss := meanSlice(lossReturns)
	if avgLoss > 0 {
		s.AvgRR = avgWin / avgLoss
	}

	// Profit factor (총이익 / 총손실)
	if grossLoss > 0 {
		s.ProfitFactor = grossWin / grossLoss
	} else if grossWin > 0 {
		s.ProfitFactor = 99.99 // 손실 없음 → 사실상 무한대
	}

	// Total return (단순 합산)
	for _, r := range allReturns {
		s.TotalReturnPct += r
	}

	// Max drawdown (자본 곡선 기준)
	s.MaxDrawdown = calcMaxDrawdown(allReturns)

	// Sharpe ratio: (평균수익 / 표준편차) × sqrt(N)
	m := meanSlice(allReturns)
	std := stddevSlice(allReturns, m)
	if std > 0 {
		s.Sharpe = (m / std) * math.Sqrt(float64(n))
	}

	// Max consecutive losses
	maxCL, cur := 0, 0
	for _, o := range outcomes {
		if !o.Win {
			cur++
			if cur > maxCL {
				maxCL = cur
			}
		} else {
			cur = 0
		}
	}
	s.MaxConsecLosses = maxCL

	return s
}

func meanSlice(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		sum += x
	}
	return sum / float64(len(xs))
}

func stddevSlice(xs []float64, m float64) float64 {
	if len(xs) < 2 {
		return 0
	}
	sum := 0.0
	for _, x := range xs {
		d := x - m
		sum += d * d
	}
	return math.Sqrt(sum / float64(len(xs)-1))
}

func calcMaxDrawdown(pnlSeries []float64) float64 {
	equity := 100.0
	peak := equity
	mdd := 0.0
	for _, p := range pnlSeries {
		equity += p
		if equity > peak {
			peak = equity
		}
		if peak > 0 {
			dd := (peak - equity) / peak
			if dd > mdd {
				mdd = dd
			}
		}
	}
	return mdd
}
