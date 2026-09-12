package backtest

import (
	"github.com/Ju571nK/Chatter/pkg/models"
	"testing"
	"time"
)

type preparationStore []models.OHLCV

func (s preparationStore) GetOHLCVAll(string, string) ([]models.OHLCV, error) { return s, nil }

func TestPreparationMatchesWarmupBoundary(t *testing.T) {
	for _, count := range []int{0, 200, 201, 202, 500} {
		bars := make(preparationStore, count)
		for i := range bars {
			bars[i].OpenTime = time.Unix(int64(i)*3600, 0)
		}
		r := NewRunner(bars, &Engine{cfg: DefaultConfig()})
		p, err := r.Prepare("SPCX", "1H")
		if err != nil || p.Bars != count || p.MinimumBars != 202 || p.Ready != (count >= 202) {
			t.Fatalf("count %d: %+v %v", count, p, err)
		}
		if count == 0 && (p.From != nil || p.To != nil) {
			t.Fatal("empty range invented")
		}
		if count > 0 && (*p.From != 0 || *p.To != int64(count-1)*3600) {
			t.Fatal("wrong full range")
		}
	}
}
