package general_ta

import (
	"fmt"
	"math"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

const fibTolerancePct = 0.005

// FibonacciConfluenceRule signals when current price is within 0.5% of a key Fibonacci level.
//
// Direction: price < FIB_500 → LONG (lower half, support), price > FIB_500 → SHORT (upper half, resistance)
// Checks levels: FIB_236, FIB_382, FIB_500, FIB_618, FIB_786
// Score = 1.0 - (proximity / tolerance), clamped [0.1, 1.0]
type FibonacciConfluenceRule struct{}

func (r *FibonacciConfluenceRule) Name() string                 { return "fibonacci_confluence" }
func (r *FibonacciConfluenceRule) RequiredIndicators() []string { return nil }

func (r *FibonacciConfluenceRule) Analyze(ctx models.AnalysisContext) (*models.Signal, error) {
	tfs := []string{"1W", "1D", "4H", "1H"}
	tfW := map[string]float64{"1W": 2.0, "1D": 1.5, "4H": 1.2, "1H": 1.0}

	fibKeys := []string{"FIB_236", "FIB_382", "FIB_500", "FIB_618", "FIB_786"}

	var bestSig *models.Signal
	var bestWeighted float64

	for _, tf := range tfs {
		fib500, ok := ctx.Indicators[tf+":FIB_500"]
		if !ok {
			continue
		}

		bars, ok := ctx.Timeframes[tf]
		if !ok || len(bars) == 0 {
			continue
		}

		price := bars[len(bars)-1].Close

		var dir string
		if price < fib500 {
			dir = "LONG"
		} else {
			dir = "SHORT"
		}

		for _, key := range fibKeys {
			level, ok := ctx.Indicators[tf+":"+key]
			if !ok {
				continue
			}

			proximity := math.Abs(price-level) / level
			if proximity > fibTolerancePct {
				continue
			}

			rawScore := 1.0 - proximity/fibTolerancePct
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
					Message:   fmt.Sprintf("[%s] 가격(%.4f)이 %s(%.4f) 근처 → %s", tf, price, key, level, dir),
					CreatedAt: time.Now(),
				}
			}
		}
	}

	return bestSig, nil
}
