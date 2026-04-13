package alpaca

import (
	"strings"
	"testing"

	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestMapTradeSignalToOrder(t *testing.T) {
	t.Parallel()
	base := models.TradeSignal{
		ID:         "sig-1",
		Symbol:     "aapl",
		Direction:  "LONG",
		EntryPrice: 200,
		AssetClass: "stock",
	}
	cases := []struct {
		name     string
		mutate   func(*models.TradeSignal)
		notional float64
		wantSide string
		wantQty  string
		wantErr  string
	}{
		{"long sizing", func(*models.TradeSignal) {}, 1000, "buy", "5", ""},
		{"short sizing", func(s *models.TradeSignal) { s.Direction = "SHORT"; s.EntryPrice = 100 }, 1000, "sell", "10", ""},
		{"empty symbol", func(s *models.TradeSignal) { s.Symbol = "" }, 1000, "", "", "empty symbol"},
		{"crypto rejected", func(s *models.TradeSignal) { s.AssetClass = "crypto" }, 1000, "", "", "crypto asset_class"},
		{"bad direction", func(s *models.TradeSignal) { s.Direction = "FLAT" }, 1000, "", "", "unsupported direction"},
		{"zero price", func(s *models.TradeSignal) { s.EntryPrice = 0 }, 1000, "", "", "entry_price"},
		{"zero notional", func(*models.TradeSignal) {}, 0, "", "", "notional"},
		{"qty below 1", func(s *models.TradeSignal) { s.EntryPrice = 5000 }, 1000, "", "", "qty < 1"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sig := base
			tc.mutate(&sig)
			got, err := MapTradeSignalToOrder(sig, tc.notional)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Side != tc.wantSide {
				t.Errorf("side = %q want %q", got.Side, tc.wantSide)
			}
			if got.Qty != tc.wantQty {
				t.Errorf("qty = %q want %q", got.Qty, tc.wantQty)
			}
			if got.Symbol != strings.ToUpper(sig.Symbol) {
				t.Errorf("symbol = %q want %q", got.Symbol, strings.ToUpper(sig.Symbol))
			}
			if got.Type != "market" || got.TimeInForce != "day" {
				t.Errorf("unexpected type/tif: %+v", got)
			}
			if got.ClientOrderID != sig.ID {
				t.Errorf("client_order_id = %q want %q", got.ClientOrderID, sig.ID)
			}
		})
	}
}
