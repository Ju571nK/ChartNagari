package main

// Offline research only: reads a Yahoo snapshot and rule configuration;
// never opens the application DB, changes settings or sends notifications.
import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/Ju571nK/Chatter/internal/backtest"
	"github.com/Ju571nK/Chatter/internal/collector"
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/methodology/candlestick"
	"github.com/Ju571nK/Chatter/internal/methodology/general_ta"
	"github.com/Ju571nK/Chatter/internal/methodology/ict"
	"github.com/Ju571nK/Chatter/internal/methodology/smc"
	"github.com/Ju571nK/Chatter/internal/methodology/wyckoff"
	"github.com/Ju571nK/Chatter/internal/rule"
	"github.com/Ju571nK/Chatter/pkg/models"
	"gopkg.in/yaml.v3"
	"os"
	"time"
)

type yahooResult struct {
	Meta struct {
		Symbol   string `json:"symbol"`
		Currency string `json:"currency"`
	} `json:"meta"`
	Timestamp  []int64 `json:"timestamp"`
	Indicators struct {
		Quote []struct{ Open, High, Low, Close []*float64 } `json:"quote"`
	} `json:"indicators"`
}

func main() {
	input := flag.String("input", "", "legacy Yahoo hourly JSON input")
	input15m := flag.String("input-15m", "tmp/usdjpy-15m-research/yahoo-15m.json", "Yahoo native 15-minute JSON snapshot")
	input1h := flag.String("input-1h", "tmp/usdjpy-15m-research/yahoo-1y.json", "Yahoo hourly JSON snapshot")
	output := flag.String("output", "tmp/usdjpy-15m-research/replay.json", "Replay JSON output")
	rulesPath := flag.String("rules", "config/rules.yaml", "Rules configuration (no secrets)")
	flag.Parse()
	now := time.Now().UTC()
	inputs := map[string]string{"15M": *input15m, "1H": *input1h}
	if *input != "" {
		inputs["1H"] = *input
	}
	bars := map[string][]models.OHLCV{}
	for _, tf := range []string{"15M", "1H"} {
		bars[tf] = readYahooBars(inputs[tf], tf, now)
	}
	bars["4H"] = collector.RebuildFX4H("USDJPY", bars["1H"])
	rb, err := os.ReadFile(*rulesPath)
	must(err)
	var rc appconfig.RulesConfig
	must(yaml.Unmarshal(rb, &rc))
	cfg := toEngineConfig(rc)
	// Match the built-in Forex methodology restrictions; do not modify live config.
	profile := appconfig.DefaultForexProfile()
	for name, entry := range cfg.Rules {
		if !profile.IsSignalAllowed(name) {
			entry.Enabled = false
			cfg.Rules[name] = entry
		}
	}
	allRules := []rule.AnalysisRule{
		// General TA
		&general_ta.RSIOverboughtOversoldRule{},
		&general_ta.RSIDivergenceRule{},
		&general_ta.EMACrossRule{},
		&general_ta.SupportResistanceBreakoutRule{},
		&general_ta.FibonacciConfluenceRule{},
		&general_ta.VolumeSpikeRule{},
		&general_ta.VSAEffortCandleRule{},
		// ICT
		&ict.ICTOrderBlockRule{},
		&ict.ICTFairValueGapRule{},
		&ict.ICTLiquiditySweepRule{},
		&ict.ICTBreakerBlockRule{},
		ict.NewICTKillZoneRule(),
		&ict.ICTOTERule{},
		&ict.ICTAMDSessionRule{},
		// Wyckoff
		&wyckoff.WyckoffAccumulationRule{},
		&wyckoff.WyckoffDistributionRule{},
		&wyckoff.WyckoffSpringRule{},
		&wyckoff.WyckoffUpthrustRule{},
		&wyckoff.WyckoffVolumeAnomalyRule{},
		// SMC
		&smc.SMCBOSRule{},
		&smc.SMCChoCHRule{},
		// Candlestick
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
	results := map[string]backtest.BacktestResult{}
	horizons := map[string]int{"15M": 20, "1H": 5, "4H": 2} // five hours, rounded up to whole bars
	for _, tf := range []string{"15M", "1H", "4H"} {
		bc := backtest.DefaultConfig()
		bc.AssetClass = models.AssetForex
		bc.VolumeQuality = models.VolumeNone
		bc.TPATRMultiplier = 1.5
		bc.SLATRMultiplier = 1
		bc.SpreadPips = 1
		bc.MaxExitBars = horizons[tf]
		results[tf] = backtest.New(allRules, cfg, bc).Run("USDJPY", tf, "", bars[tf])
		fmt.Fprintf(os.Stderr, "%s: %d bars, %d raw outcomes\n", tf, len(bars[tf]), results[tf].Trades)
	}
	out := map[string]interface{}{"generated_at": now, "bars": bars, "results": results, "rules_config": rc, "warmup_bars": 200, "max_exit_bars": horizons, "max_exit_hours": 5, "tp_atr": 1.5, "sl_atr": 1, "spread_pips": 1}
	b, err := json.Marshal(out)
	must(err)
	must(os.WriteFile(*output, b, 0644))
}
func readYahooBars(path, tf string, now time.Time) []models.OHLCV {
	blob, err := os.ReadFile(path)
	must(err)
	var raw struct {
		Chart struct {
			Result []yahooResult
			Error  interface{}
		}
	}
	must(json.Unmarshal(blob, &raw))
	if len(raw.Chart.Result) != 1 || raw.Chart.Error != nil {
		panic("Yahoo returned no usable result for " + tf)
	}
	y := raw.Chart.Result[0]
	if (y.Meta.Symbol != "USDJPY=X" && y.Meta.Symbol != "JPY=X") || y.Meta.Currency != "JPY" {
		panic("expected USDJPY=X quoted in JPY")
	}
	if len(y.Indicators.Quote) != 1 {
		panic("missing OHLC for " + tf)
	}
	q := y.Indicators.Quote[0]
	duration := time.Hour
	if tf == "15M" {
		duration = 15 * time.Minute
	}
	bars := make([]models.OHLCV, 0, len(y.Timestamp))
	for i, ts := range y.Timestamp {
		if i >= len(q.Open) || i >= len(q.High) || i >= len(q.Low) || i >= len(q.Close) {
			panic("mismatched OHLC arrays")
		}
		if q.Open[i] == nil || q.High[i] == nil || q.Low[i] == nil || q.Close[i] == nil {
			continue
		}
		t := time.Unix(ts, 0).UTC()
		if t.Add(duration).After(now) {
			continue
		}
		o, h, l, c := *q.Open[i], *q.High[i], *q.Low[i], *q.Close[i]
		if o <= 0 || c <= 0 || l <= 0 || h < l || h < o || h < c || l > o || l > c {
			panic("invalid OHLC")
		}
		if len(bars) > 0 && !t.After(bars[len(bars)-1].OpenTime) {
			panic("timestamps not strictly ascending")
		}
		bars = append(bars, models.OHLCV{Symbol: "USDJPY", Timeframe: tf, OpenTime: t, Open: o, High: h, Low: l, Close: c, VolumeQuality: models.VolumeNone})
	}
	if len(bars) < 1000 {
		panic("not enough history for train/holdout research: " + tf)
	}
	return bars
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func toEngineConfig(rc appconfig.RulesConfig) engine.RuleConfig {
	rules := make(map[string]engine.RuleEntry, len(rc.Rules))
	for _, r := range rc.Rules {
		weight := 1.0
		if s, ok := r.Params["strength"].(float64); ok {
			weight = s
		}
		rules[r.Name] = engine.RuleEntry{
			Enabled:      r.Enabled,
			Timeframe:    "ALL",
			Weight:       weight,
			AssetClasses: r.AssetClasses,
		}
	}
	return engine.RuleConfig{Rules: rules}
}
