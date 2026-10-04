package forex

import (
	"math"
	"sort"

	"github.com/Ju571nK/Chatter/pkg/models"
)

type CurrencyStrength struct {
	Currency string   `json:"currency"`
	Value    *float64 `json:"value"`
	Coverage string   `json:"coverage"`
	Pairs    int      `json:"pairs"`
}

// Strength uses one lookback return per enabled pair, adjusting sign for the
// quote currency. Series may be ascending or descending, but must be complete.
func Strength(series map[string][]models.OHLCV, lookback int) []CurrencyStrength {
	if lookback < 1 {
		lookback = 20
	}
	sums := map[string]float64{}
	counts := map[string]int{}
	for pair, bars := range series {
		if len(pair) != 6 || len(bars) <= lookback {
			continue
		}
		ordered := append([]models.OHLCV(nil), bars...)
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].OpenTime.Before(ordered[j].OpenTime) })
		old, newest := ordered[len(ordered)-1-lookback].Close, ordered[len(ordered)-1].Close
		if old <= 0 || newest <= 0 {
			continue
		}
		change := (newest/old - 1) * 100
		base, quote := pair[:3], pair[3:]
		sums[base] += change
		counts[base]++
		sums[quote] -= change
		counts[quote]++
	}
	maxAbs := 0.0
	for currency, n := range counts {
		if n >= 2 {
			v := math.Abs(sums[currency] / float64(n))
			if v > maxAbs {
				maxAbs = v
			}
		}
	}
	out := make([]CurrencyStrength, 0, len(counts))
	for currency, n := range counts {
		entry := CurrencyStrength{Currency: currency, Pairs: n, Coverage: "low"}
		if n >= 2 {
			entry.Coverage = "ok"
			v := 0.0
			if maxAbs > 0 {
				v = sums[currency] / float64(n) / maxAbs * 100
			}
			entry.Value = &v
		}
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value == nil {
			return false
		}
		if out[j].Value == nil {
			return true
		}
		return *out[i].Value > *out[j].Value
	})
	return out
}
