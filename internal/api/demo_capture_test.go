package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/methodology/candlestick"
	"github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/methodology/smc"
	"github.com/Ju571nK/Chatter/internal/methodology/wyckoff"
	"github.com/Ju571nK/Chatter/internal/rule"
)

// demoTestEngine builds a rule engine with the same rule roster as
// cmd/server/main.go, every rule enabled, for demo-scan tests.
func demoTestEngine() *engine.RuleEngine {
	// Same rule roster as cmd/server/main.go. Every rule is enabled so the
	// demo surfaces the full breadth of ICT / Wyckoff / SMC / TA detections.
	allRules := []rule.AnalysisRule{
		&general_ta.RSIOverboughtOversoldRule{},
		&general_ta.RSIDivergenceRule{},
		&general_ta.EMACrossRule{},
		&general_ta.SupportResistanceBreakoutRule{},
		&general_ta.FibonacciConfluenceRule{},
		&general_ta.VolumeSpikeRule{},
		&general_ta.VSAEffortCandleRule{},
		&ict.ICTOrderBlockRule{},
		&ict.ICTFairValueGapRule{},
		&ict.ICTLiquiditySweepRule{},
		&ict.ICTBreakerBlockRule{},
		ict.NewICTKillZoneRule(),
		&ict.ICTOTERule{},
		&ict.ICTAMDSessionRule{},
		&wyckoff.WyckoffAccumulationRule{},
		&wyckoff.WyckoffDistributionRule{},
		&wyckoff.WyckoffSpringRule{},
		&wyckoff.WyckoffUpthrustRule{},
		&wyckoff.WyckoffVolumeAnomalyRule{},
		&smc.SMCBOSRule{},
		&smc.SMCChoCHRule{},
		&candlestick.DojiRule{},
		&candlestick.HammerRule{},
		&candlestick.HangingManRule{},
		&candlestick.ShootingStarRule{},
		&candlestick.InvertedHammerRule{},
		&candlestick.MarubozuRule{},
		&candlestick.BullishEngulfingRule{},
		&candlestick.BearishEngulfingRule{},
		&candlestick.BullishHaramiRule{},
		&candlestick.BearishHaramiRule{},
		&candlestick.MorningStarRule{},
		&candlestick.EveningStarRule{},
		&candlestick.ThreeWhiteSoldiersRule{},
		&candlestick.ThreeBlackCrowsRule{},
	}

	cfg := engine.RuleConfig{Rules: map[string]engine.RuleEntry{}}
	for _, r := range allRules {
		cfg.Rules[r.Name()] = engine.RuleEntry{Enabled: true, Timeframe: "ALL", Weight: 1.0}
	}
	eng := engine.New(cfg)
	for _, r := range allRules {
		eng.Register(r)
	}
	return eng
}

// demoScanResponse mirrors the frontend DemoScan contract consumed by
// web/src/demoApi.ts — keep field names in sync.
type demoScanResponse struct {
	Symbol    string `json:"symbol"`
	Timeframe string `json:"timeframe"`
	Bars      []struct {
		Time   int64   `json:"time"`
		Open   float64 `json:"open"`
		High   float64 `json:"high"`
		Low    float64 `json:"low"`
		Close  float64 `json:"close"`
		Volume float64 `json:"volume"`
	} `json:"bars"`
	Signals []struct {
		Rule      string  `json:"rule"`
		Direction string  `json:"direction"`
		Score     float64 `json:"score"`
	} `json:"signals"`
}

// TestDemoScanEndpoint asserts /api/demo/scan returns a valid DemoScan payload
// for every captured timeframe. Ungated: this is the regression guard for the
// demo endpoint and the Go↔frontend schema contract.
func TestDemoScanEndpoint(t *testing.T) {
	srv := New(t.TempDir(), "")
	srv.WithDemoEngine(demoTestEngine())

	for _, tf := range []string{"1W", "1D", "4H", "1H"} {
		req := httptest.NewRequest("GET", "/api/demo/scan?symbol=DEMO_BTC&timeframe="+tf, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("demo/scan %s: want 200, got %d (%s)", tf, w.Code, w.Body)
		}
		var resp demoScanResponse
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("demo/scan %s: invalid JSON: %v", tf, err)
		}
		if resp.Symbol != "DEMO_BTC" || resp.Timeframe != tf {
			t.Errorf("demo/scan %s: got symbol=%q timeframe=%q", tf, resp.Symbol, resp.Timeframe)
		}
		if len(resp.Bars) == 0 {
			t.Errorf("demo/scan %s: no bars", tf)
		}
		for i, b := range resp.Bars {
			if b.Low > b.High {
				t.Errorf("demo/scan %s: bar %d low %.2f > high %.2f", tf, i, b.Low, b.High)
			}
		}
		// No future candles: the last bar must land at or before ~now.
		// (Regression guard for the fixed-days baseTime bug that pushed
		// weekly bars ~10 months into the future.)
		if n := len(resp.Bars); n > 0 {
			last := time.Unix(resp.Bars[n-1].Time, 0)
			if last.After(time.Now().Add(24 * time.Hour)) {
				t.Errorf("demo/scan %s: last bar %s is in the future", tf, last)
			}
		}
		if len(resp.Signals) == 0 {
			t.Errorf("demo/scan %s: no signals — demo would show an empty scan", tf)
		}
		for i, s := range resp.Signals {
			if s.Rule == "" || (s.Direction != "LONG" && s.Direction != "SHORT" && s.Direction != "NEUTRAL") {
				t.Errorf("demo/scan %s: signal %d invalid: rule=%q direction=%q", tf, i, s.Rule, s.Direction)
			}
		}
	}
}

// TestCaptureDemoFixtures runs the real rule engine against the deterministic
// demo bars and writes the /api/demo/scan responses to web/public/demo/ as
// static JSON. These fixtures back the zero-install GitHub Pages demo.
//
// It is gated behind CAPTURE_DEMO so it never runs during `go test ./...`
// (it is a fixture generator, not an assertion — see TestDemoScanEndpoint for
// the ungated regression guard). Regenerate with:
//
//	CAPTURE_DEMO=1 go test ./internal/api -run TestCaptureDemoFixtures -count=1
func TestCaptureDemoFixtures(t *testing.T) {
	if os.Getenv("CAPTURE_DEMO") == "" {
		t.Skip("set CAPTURE_DEMO=1 to regenerate web/public/demo fixtures")
	}

	srv := New(t.TempDir(), "")
	srv.WithDemoEngine(demoTestEngine())

	outDir := filepath.Join("..", "..", "web", "public", "demo")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", outDir, err)
	}

	for _, tf := range []string{"1W", "1D", "4H", "1H"} {
		req := httptest.NewRequest("GET", "/api/demo/scan?symbol=DEMO_BTC&timeframe="+tf, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("demo/scan %s: want 200, got %d (%s)", tf, w.Code, w.Body)
		}
		out := filepath.Join(outDir, "scan-"+tf+".json")
		if err := os.WriteFile(out, w.Body.Bytes(), 0o644); err != nil {
			t.Fatalf("write %s: %v", out, err)
		}
		t.Logf("wrote %s (%d bytes)", out, w.Body.Len())
	}
}
