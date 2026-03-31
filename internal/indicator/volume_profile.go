package indicator

import (
	"sort"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// volumeProfile computes a volume profile from OHLCV bars by dividing the
// overall high-low price range into numBins equal-width bins and distributing
// each bar's volume proportionally across the bins it spans.
//
// Returns:
//   - poc:  Point of Control — midpoint price of the bin with the highest volume.
//   - hvns: High Volume Nodes — midpoint prices of the top 3 bins by volume,
//     excluding the POC bin, sorted by volume descending.
//   - lvns: Low Volume Nodes — midpoint prices of the bottom 3 bins by volume,
//     sorted by volume ascending.
//   - ok:   false when there are fewer than 10 bars, numBins < 1, or the entire
//     price range is zero (all prices identical).
func volumeProfile(candles []models.OHLCV, numBins int) (poc float64, hvns, lvns []float64, ok bool) {
	if len(candles) < 10 || numBins < 1 {
		return 0, nil, nil, false
	}

	// Find the overall price range across all bars.
	priceMin := candles[0].Low
	priceMax := candles[0].High
	for _, c := range candles[1:] {
		if c.Low < priceMin {
			priceMin = c.Low
		}
		if c.High > priceMax {
			priceMax = c.High
		}
	}

	if priceMax <= priceMin {
		return 0, nil, nil, false
	}

	binSize := (priceMax - priceMin) / float64(numBins)
	bins := make([]float64, numBins)

	// Distribute each bar's volume proportionally across the bins it spans.
	for _, c := range candles {
		if c.Volume <= 0 {
			continue
		}

		candleLow := c.Low
		candleHigh := c.High

		// Zero-range bar (doji): assign all volume to the single containing bin.
		if candleHigh <= candleLow {
			idx := int((candleLow - priceMin) / binSize)
			if idx >= numBins {
				idx = numBins - 1
			}
			if idx < 0 {
				idx = 0
			}
			bins[idx] += c.Volume
			continue
		}

		startBin := int((candleLow - priceMin) / binSize)
		endBin := int((candleHigh - priceMin) / binSize)
		if startBin < 0 {
			startBin = 0
		}
		if endBin >= numBins {
			endBin = numBins - 1
		}

		candleRange := candleHigh - candleLow
		for b := startBin; b <= endBin; b++ {
			binLow := priceMin + float64(b)*binSize
			binHigh := binLow + binSize

			overlapLow := candleLow
			if binLow > overlapLow {
				overlapLow = binLow
			}
			overlapHigh := candleHigh
			if binHigh < overlapHigh {
				overlapHigh = binHigh
			}
			if overlapHigh <= overlapLow {
				continue
			}
			fraction := (overlapHigh - overlapLow) / candleRange
			bins[b] += c.Volume * fraction
		}
	}

	// Find the POC bin (highest volume).
	pocIdx := 0
	for i := 1; i < numBins; i++ {
		if bins[i] > bins[pocIdx] {
			pocIdx = i
		}
	}
	poc = priceMin + (float64(pocIdx)+0.5)*binSize

	// Build an index slice for sorting bins by volume.
	type binEntry struct {
		idx    int
		volume float64
	}
	entries := make([]binEntry, numBins)
	for i := range entries {
		entries[i] = binEntry{idx: i, volume: bins[i]}
	}

	// Sort descending by volume to find HVNs.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].volume > entries[j].volume
	})

	// Top 3 bins excluding the POC bin → HVNs.
	for _, e := range entries {
		if len(hvns) == 3 {
			break
		}
		if e.idx == pocIdx {
			continue
		}
		hvns = append(hvns, priceMin+(float64(e.idx)+0.5)*binSize)
	}

	// Sort ascending by volume to find LVNs.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].volume < entries[j].volume
	})

	// Bottom 3 bins → LVNs.
	for _, e := range entries {
		if len(lvns) == 3 {
			break
		}
		lvns = append(lvns, priceMin+(float64(e.idx)+0.5)*binSize)
	}

	return poc, hvns, lvns, true
}
