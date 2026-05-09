// Package pipeline connects the data storage, indicator engine, rule engine,
// AI interpreter, and notifier into a single periodic analysis loop.
//
// Flow (per symbol, per tick):
//
//	SQLite OHLCV → indicator.Compute → engine.Run → interpreter.Enrich → notifier.Notify
package pipeline

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/interpreter"
	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/internal/sequence"
	"github.com/Ju571nK/Chatter/internal/wyckoff"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// OHLCVReader is the storage interface the pipeline depends on.
// *storage.DB satisfies this interface.
type OHLCVReader interface {
	GetOHLCV(symbol, timeframe string, limit int) ([]models.OHLCV, error)
}

// SignalSaver persists generated signals for later retrieval (e.g., chart markers).
// *storage.DB satisfies this interface.
type SignalSaver interface {
	SaveSignal(sig models.Signal) (int64, error)
}

// PaperTrader tracks virtual paper positions driven by live signals.
// *paper.Trader satisfies this interface.
type PaperTrader interface {
	OnSignals(signals []models.Signal)
	CheckPositions(sym string, allBars map[string][]models.OHLCV)
}

// PriceAlertWatcher checks user-defined price targets on every pipeline tick.
// *pricealert.Watcher satisfies this interface.
type PriceAlertWatcher interface {
	CheckSymbol(ctx context.Context, symbol string, currentPrice float64)
}

// ExecutionDispatcher fans out TradeSignal envelopes to registered plugins.
// *execution.Dispatcher satisfies this interface. Kept as an interface so the
// pipeline has no hard dependency on the execution package (tests can stub).
type ExecutionDispatcher interface {
	Dispatch(ctx context.Context, signal models.TradeSignal)
}

// SignalBroadcaster pushes new signals to connected WebSocket clients.
// *hub.Hub satisfies this interface.
type SignalBroadcaster interface {
	Broadcast(msgType string, payload interface{})
}

// ForwardReturnStore persists forward return updates.
// *storage.DB satisfies this interface.
type ForwardReturnStore interface {
	ForwardReturnDB
	ForwardReturnOHLCVReader
}

// Config controls pipeline timing and data parameters.
type Config struct {
	Interval        time.Duration // how often to run analysis (default: 1 minute)
	Lookback        int           // bars to load per TF (default: 200)
	SignalMinScore  float64       // minimum score to persist a signal to DB (default: 5.0)
	SignalCooldown  time.Duration // minimum gap between saves for same symbol+rule (default: 4h)
	MTFConsensusMin int           // ≥2 enables direction consensus filter. 1=disabled (legacy). Default 2
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		Interval:        time.Minute,
		Lookback:        200,
		SignalMinScore:  5.0,
		SignalCooldown:  4 * time.Hour,
		MTFConsensusMin: 2,
	}
}

// Pipeline periodically reads OHLCV data, computes indicators, runs rules,
// applies AI interpretation for high-scoring signals, and dispatches notifications.
// It is safe to call Run once per instance.
type Pipeline struct {
	cfg         Config
	db          OHLCVReader
	sigSaver    SignalSaver                    // optional; set via SetSignalSaver
	paperTrader PaperTrader                    // optional; set via SetPaperTrader
	alertHolder *appconfig.AlertConfigHolder   // optional; set via SetAlertConfigHolder
	eng         *engine.RuleEngine
	interp      *interpreter.Interpreter
	notif       *notifier.Notifier
	symbols     []string
	timeframes  []string
	log         zerolog.Logger
	cryptoSyms  map[string]bool
	marketOpen   bool // tracks NYSE open/close state for transition logging

	priceAlertWatcher PriceAlertWatcher // optional; set via SetPriceAlertWatcher
	broadcaster       SignalBroadcaster  // optional; set via SetBroadcaster
	dispatcher        ExecutionDispatcher // optional; set via SetExecutionDispatcher

	seqTracker *sequence.Tracker // tracks signal sequences for bonus scoring

	profileHolder *appconfig.SymbolProfilesHolder // optional; set via SetSymbolProfiles
	tuningHolder  *appconfig.SignalTuningHolder  // optional; set via SetSignalTuningHolder
	overrideStore appconfig.OverrideGetter        // optional; set via SetOverrideStore. nil → profile-only resolution

	forwardReturnDB   ForwardReturnDB          // optional; set via SetForwardReturnStore
	forwardReturnOHLCV ForwardReturnOHLCVReader // optional; set via SetForwardReturnStore

	sigCooldownMu sync.Mutex
	sigLastSaved  map[string]time.Time // key: symbol+":"+rule
}

// New creates a Pipeline wired to the provided components.
func New(
	cfg Config,
	db OHLCVReader,
	eng *engine.RuleEngine,
	interp *interpreter.Interpreter,
	notif *notifier.Notifier,
	symbols []string,
	timeframes []string,
	log zerolog.Logger,
) *Pipeline {
	return &Pipeline{
		cfg:          cfg,
		db:           db,
		eng:          eng,
		interp:       interp,
		notif:        notif,
		symbols:      symbols,
		timeframes:   timeframes,
		log:          log,
		seqTracker:   sequence.New(),
		sigLastSaved: make(map[string]time.Time),
	}
}

// SetSignalSaver wires an optional signal persistence store.
// Call before Run; safe to call only once.
func (p *Pipeline) SetSignalSaver(ss SignalSaver) {
	p.sigSaver = ss
}

// SetPaperTrader wires an optional paper trading simulator.
func (p *Pipeline) SetPaperTrader(pt PaperTrader) {
	p.paperTrader = pt
}

// SetPriceAlertWatcher wires an optional price alert checker.
func (p *Pipeline) SetPriceAlertWatcher(w PriceAlertWatcher) {
	p.priceAlertWatcher = w
}

// SetBroadcaster wires an optional WebSocket broadcaster.
func (p *Pipeline) SetBroadcaster(b SignalBroadcaster) {
	p.broadcaster = b
}

// SetExecutionDispatcher wires the Phase 2 trade execution dispatcher. When set,
// every enriched signal that passes notifier filtering is also converted to a
// TradeSignal envelope and fanned out to eligible plugins.
func (p *Pipeline) SetExecutionDispatcher(d ExecutionDispatcher) {
	p.dispatcher = d
}

// SetAlertConfigHolder wires an optional live-updated alert configuration holder.
func (p *Pipeline) SetAlertConfigHolder(h *appconfig.AlertConfigHolder) {
	p.alertHolder = h
}

// SetSymbolProfiles wires an optional per-symbol profile holder for rule filtering.
func (p *Pipeline) SetSymbolProfiles(h *appconfig.SymbolProfilesHolder) {
	p.profileHolder = h
}

// SetOverrideStore wires an optional per-symbol override store for hot-reload alert config.
// When set, EffectiveAlertConfig merges profile defaults with per-symbol DB rows on each tick.
func (p *Pipeline) SetOverrideStore(store appconfig.OverrideGetter) {
	p.overrideStore = store
}

// SetSignalTuningHolder wires an optional live-updated signal tuning configuration holder.
func (p *Pipeline) SetSignalTuningHolder(h *appconfig.SignalTuningHolder) {
	p.tuningHolder = h
}

// SetForwardReturnStore wires the forward return tracking store.
func (p *Pipeline) SetForwardReturnStore(frDB ForwardReturnDB, ohlcv ForwardReturnOHLCVReader) {
	p.forwardReturnDB = frDB
	p.forwardReturnOHLCV = ohlcv
}

// SetCryptoSymbols records which symbols are crypto (vs stock) for TP/SL multiplier selection.
func (p *Pipeline) SetCryptoSymbols(syms []string) {
	p.cryptoSyms = make(map[string]bool, len(syms))
	for _, s := range syms {
		p.cryptoSyms[s] = true
	}
}

// isCrypto returns true if sym is a known crypto symbol.
func (p *Pipeline) isCrypto(sym string) bool {
	return p.cryptoSyms != nil && p.cryptoSyms[sym]
}

// Run starts the periodic analysis loop. It blocks until ctx is cancelled.
func (p *Pipeline) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.runOnce(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// RunOnce executes one analysis cycle across all configured symbols.
// It is exported for testing purposes.
func (p *Pipeline) RunOnce(ctx context.Context) {
	p.runOnce(ctx)
}

func (p *Pipeline) runOnce(ctx context.Context) {
	isOpen := market.IsUSMarketOpen(time.Now())
	if isOpen != p.marketOpen {
		if isOpen {
			p.log.Info().Msg("US market opened — stock analysis started")
		} else {
			p.log.Info().Msg("US market closed — stock analysis paused")
		}
		p.marketOpen = isOpen
	}

	for _, sym := range p.symbols {
		if !p.isCrypto(sym) && !isOpen {
			continue
		}
		p.analyzeSymbol(ctx, sym)
	}

	// Forward return tracking: update historical signals with actual returns.
	if p.forwardReturnDB != nil && p.forwardReturnOHLCV != nil {
		UpdateForwardReturns(p.forwardReturnDB, p.forwardReturnOHLCV, p.log)
	}
}

func (p *Pipeline) analyzeSymbol(ctx context.Context, sym string) {
	// Load OHLCV bars for each configured timeframe.
	allBars := make(map[string][]models.OHLCV, len(p.timeframes))
	for _, tf := range p.timeframes {
		bars, err := p.db.GetOHLCV(sym, tf, p.cfg.Lookback)
		if err != nil {
			p.log.Error().Err(err).Str("symbol", sym).Str("tf", tf).Msg("OHLCV load failed")
			continue
		}
		if len(bars) > 0 {
			allBars[tf] = bars
		}
	}

	if len(allBars) == 0 {
		p.log.Debug().Str("symbol", sym).Msg("no OHLCV data — skipping analysis")
		return
	}

	// Check price alerts for this symbol using the latest 1D (or any available) close.
	if p.priceAlertWatcher != nil {
		var latestClose float64
		for _, tf := range []string{"1H", "4H", "1D", "1W"} {
			if bars, ok := allBars[tf]; ok && len(bars) > 0 {
				latestClose = bars[0].Close
				break
			}
		}
		if latestClose > 0 {
			p.priceAlertWatcher.CheckSymbol(ctx, sym, latestClose)
		}
	}

	// Compute all indicators across all loaded timeframes.
	indicators := indicator.Compute(allBars)

	// Inject benchmark (SPY) return for relative strength calculation.
	// Only if SPY bars are available (SPY must be in the watchlist).
	if sym != "SPY" {
		for _, tf := range p.timeframes {
			spyBars, err := p.db.GetOHLCV("SPY", tf, p.cfg.Lookback)
			if err != nil || len(spyBars) < 20 {
				continue
			}
			// spyBars are in DESC order; [0]=newest, [19]=20 bars ago
			spyReturn := (spyBars[0].Close - spyBars[19].Close) / spyBars[19].Close
			indicators[tf+":BENCHMARK_RETURN_20"] = spyReturn
		}
	}

	// Run the rule engine.
	analysisCtx := models.AnalysisContext{
		Symbol:     sym,
		Timeframes: allBars,
		Indicators: indicators,
	}
	signals := p.eng.Run(analysisCtx)

	// Effective alert config (profile + per-symbol override).
	effCfg := appconfig.EffectiveAlertConfig(sym, p.profileHolder, p.overrideStore)

	// Profile/override filter: remove signals not allowed by methodology/rule.
	// Uses the merged effective config so that an override can widen the
	// profile's allowed_rules list (not just narrow it).
	if p.profileHolder != nil {
		beforeProfile := len(signals)
		profile := p.profileHolder.GetProfile(sym)
		signals = filterByProfileEffective(signals, profile, effCfg)
		if filtered := beforeProfile - len(signals); filtered > 0 {
			p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Msg("profile/override filter removed disallowed signals")
		}
	}

	// Score threshold gate (per-symbol effective; 0 = no minimum).
	// Note: this filters at pipeline level. The Notifier currently also has
	// a global ScoreThreshold check; the per-symbol value here can be
	// stricter than the global, but the global remains a floor.
	if effCfg.ScoreThreshold > 0 {
		beforeScore := len(signals)
		kept := signals[:0]
		for _, sig := range signals {
			if sig.Score >= effCfg.ScoreThreshold {
				kept = append(kept, sig)
			}
		}
		signals = kept
		if filtered := beforeScore - len(signals); filtered > 0 {
			p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Float64("threshold", effCfg.ScoreThreshold).Msg("override score threshold filter removed signals")
		}
	}

	// Timeframe filter (override-driven; empty list = allow all).
	if len(effCfg.Timeframes) > 0 {
		beforeTF := len(signals)
		signals = filterByTimeframe(signals, effCfg.Timeframes)
		if filtered := beforeTF - len(signals); filtered > 0 {
			p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Strs("allowed_tf", effCfg.Timeframes).Msg("timeframe filter removed signals")
		}
	}

	// Enrich each signal with ATR-based entry/TP/SL levels (used in notifications).
	// Determine TP/SL multipliers (crypto vs stock)
	tpMult, slMult := 2.0, 1.0
	if p.alertHolder != nil {
		ac := p.alertHolder.Get()
		if p.isCrypto(sym) {
			tpMult, slMult = ac.CryptoTPMult, ac.CryptoSLMult
		} else {
			tpMult, slMult = ac.StockTPMult, ac.StockSLMult
		}
	}
	if tpMult == 0 {
		tpMult = 2.0
	}
	if slMult == 0 {
		slMult = 1.0
	}
	for i := range signals {
		enrichSignalLevels(&signals[i], allBars, indicators, tpMult, slMult)
	}

	// MTF 합의 필터: 동적 설정 우선, 없으면 정적 Config 사용
	mtfMin := p.cfg.MTFConsensusMin
	if p.alertHolder != nil {
		mtfMin = p.alertHolder.Get().MTFConsensusMin
	}
	if mtfMin > 1 {
		signals = filterMTFConsensus(signals, mtfMin)
		if len(signals) == 0 {
			p.log.Debug().Str("symbol", sym).Int("mtf_min", mtfMin).Msg("MTF consensus not met — signals filtered")
			return
		}
	}

	// Detect Wyckoff phase early so it can inform the HTF context filter.
	// During accumulation/markup, even if EMA trend is bearish, we want to allow
	// LONG signals through (the Wyckoff phase overrides pure trend reading).
	var wyckoffPhase wyckoff.Phase
	if bars1D, ok := allBars["1D"]; ok && len(bars1D) >= 50 {
		reversed := make([]models.OHLCV, len(bars1D))
		for i, b := range bars1D {
			reversed[len(bars1D)-1-i] = b
		}
		wa := wyckoff.Analyze(sym, "1D", reversed)
		wyckoffPhase = wa.Phase
	}

	// HTF context filter: penalize (or suppress) lower-TF signals that contradict higher-TF trend.
	// Penalty percentage varies by volatility regime if per-regime overrides are set.
	// Wyckoff phase can override: accumulation/markup → allow LONG even if EMA says bearish.
	htfPenaltyPct := 100 // legacy default: full suppress
	if p.tuningHolder != nil {
		tc := p.tuningHolder.Get()
		htfPenaltyPct = tc.HTFFilter.CounterTrendPenaltyPct

		if tc.HTFFilter.UseGradient {
			// Continuous gradient: penalty decreases as volatility increases.
			// penalty = gradient_base * (1 - gradient_scaling/100 * atr_percentile/100)
			if bars1D, ok := allBars["1D"]; ok {
				if currentATR, hasATR := indicators["1D:ATR_14"]; hasATR && currentATR > 0 {
					atrPctl := atrPercentile(bars1D, currentATR, 14, 90)
					if atrPctl >= 0 {
						htfPenaltyPct = int(float64(tc.HTFFilter.GradientBase) * (1.0 - float64(tc.HTFFilter.GradientScaling)/100.0*atrPctl/100.0))
						if htfPenaltyPct < 0 {
							htfPenaltyPct = 0
						}
						if htfPenaltyPct > 100 {
							htfPenaltyPct = 100
						}
					}
				}
			}
		} else {
			// Legacy 3-bucket mode: per-regime override from 1D ATR percentile
			if bars1D, ok := allBars["1D"]; ok {
				if currentATR, hasATR := indicators["1D:ATR_14"]; hasATR && currentATR > 0 {
					pctl := atrPercentile(bars1D, currentATR, 14, 90)
					if pctl >= 0 {
						switch {
						case pctl < float64(tc.VolatilityRegime.LowVolPercentile) && tc.HTFFilter.LowVolPenaltyPct > 0:
							htfPenaltyPct = tc.HTFFilter.LowVolPenaltyPct
						case pctl > float64(tc.VolatilityRegime.HighVolPercentile) && tc.HTFFilter.HighVolPenaltyPct > 0:
							htfPenaltyPct = tc.HTFFilter.HighVolPenaltyPct
						default:
							if tc.HTFFilter.NormalPenaltyPct > 0 {
								htfPenaltyPct = tc.HTFFilter.NormalPenaltyPct
							}
						}
					}
				}
			}
		}
	}
	beforeHTF := len(signals)
	signals = penalizeHTFContext(signals, indicators, allBars, wyckoffPhase, htfPenaltyPct)
	if filtered := beforeHTF - len(signals); filtered > 0 {
		p.log.Debug().Str("symbol", sym).Int("filtered", filtered).Str("wyckoff_phase", string(wyckoffPhase)).Msg("HTF context filter removed counter-trend signals")
	}

	// Signal sequence tracking: record each signal and apply bonus for completed sequences
	for i := range signals {
		matches := p.seqTracker.Record(signals[i])
		for _, m := range matches {
			signals[i].Score *= (1.0 + m.Bonus)
			p.log.Debug().
				Str("symbol", sym).
				Str("sequence", m.Name).
				Float64("bonus", m.Bonus).
				Float64("new_score", signals[i].Score).
				Msg("sequence bonus applied")
		}
	}

	// Wyckoff phase context boost: reuse the phase detected above for HTF filter.
	switch wyckoffPhase {
	case wyckoff.PhaseAccumulation, wyckoff.PhaseMarkup:
		for i := range signals {
			if signals[i].Direction == "LONG" {
				signals[i].Score *= 1.2
			}
		}
		p.log.Debug().Str("symbol", sym).Str("phase", string(wyckoffPhase)).Msg("Wyckoff phase boost: LONG +20%")
	case wyckoff.PhaseDistribution, wyckoff.PhaseMarkdown:
		for i := range signals {
			if signals[i].Direction == "SHORT" {
				signals[i].Score *= 1.2
			}
		}
		p.log.Debug().Str("symbol", sym).Str("phase", string(wyckoffPhase)).Msg("Wyckoff phase boost: SHORT +20%")
	}

	// Coiled market detection: boost signals when realized vol << implied vol (VIX).
	if p.tuningHolder != nil {
		tc := p.tuningHolder.Get()
		if tc.CoiledMarket.Enabled {
			if rv, hasRV := indicators["1D:REALIZED_VOL_20"]; hasRV {
				coiled := detectCoiledMarket(rv, p.db, float64(tc.CoiledMarket.RatioThreshold)/100.0)
				if coiled.IsCoiled {
					bonus := 1.0 + float64(tc.CoiledMarket.BonusPct)/100.0
					for i := range signals {
						signals[i].Score *= bonus
					}
					p.log.Debug().Float64("ratio", coiled.Ratio).Msg("coiled market bonus applied")
				}
			}
		}
	}

	// Volume Profile boost: adjust scores based on proximity to HVN/LVN/POC levels.
	applyVolumeProfileBoost(signals, indicators)

	// Volatility regime scoring: adjust scores based on ATR percentile classification.
	if p.tuningHolder != nil {
		tc := p.tuningHolder.Get()
		applyVolatilityRegime(signals, allBars, indicators, tc.VolatilityRegime)
		applyATRSlopeBonus(signals, allBars, indicators, tc.ATRSlope)
	}

	// Compute HTF trend and ATR percentile for signal metadata (#32 Phase 1).
	// These values are already partially computed above; extract them for persistence.
	computedHTFTrend := htfContext(indicators, "1D", allBars)
	if computedHTFTrend == "" {
		computedHTFTrend = htfContext(indicators, "1W", allBars)
	}
	var computedATRPctl float64 = -1
	if bars1D, ok := allBars["1D"]; ok {
		if currentATR, hasATR := indicators["1D:ATR_14"]; hasATR && currentATR > 0 {
			computedATRPctl = atrPercentile(bars1D, currentATR, 14, 90)
		}
	}
	for i := range signals {
		signals[i].HTFTrend = computedHTFTrend
		signals[i].ATRPercentile = computedATRPctl
	}

	// Paper trading: open new positions and check existing TP/SL.
	if p.paperTrader != nil {
		p.paperTrader.OnSignals(signals)
		p.paperTrader.CheckPositions(sym, allBars)
	}

	if len(signals) == 0 {
		p.log.Debug().Str("symbol", sym).Msg("no signals")
		return
	}

	p.log.Info().
		Str("symbol", sym).
		Int("signals", len(signals)).
		Float64("top_score", signals[0].Score).
		Msg("signals detected")

	// AI enrichment: Claude interprets high-scoring signal groups.
	group := interpreter.SignalGroup{
		Symbol:     sym,
		Signals:    signals,
		Indicators: indicators,
	}
	enriched := p.interp.Enrich(ctx, []interpreter.SignalGroup{group})

	// Persist signals for chart dashboard markers (after AI enrichment).
	// Only save signals above MinScore and not within the cooldown window.
	if p.sigSaver != nil {
		for i := range enriched {
			if enriched[i].Score < p.cfg.SignalMinScore {
				continue
			}
			key := enriched[i].Symbol + ":" + enriched[i].Rule
			p.sigCooldownMu.Lock()
			last := p.sigLastSaved[key]
			canSave := time.Since(last) >= p.cfg.SignalCooldown
			if canSave {
				p.sigLastSaved[key] = time.Now()
			}
			p.sigCooldownMu.Unlock()
			if !canSave {
				continue
			}
			id, err := p.sigSaver.SaveSignal(enriched[i])
			if err != nil {
				p.log.Warn().Err(err).Str("symbol", sym).Msg("signal save failed")
				continue
			}
			enriched[i].ID = id
		}
	}

	// Broadcast enriched signals to connected WebSocket clients.
	if p.broadcaster != nil {
		for i := range enriched {
			p.broadcaster.Broadcast("signal", enriched[i])
		}
	}

	// Notify: filters by score threshold and cooldown.
	p.notif.Notify(ctx, enriched)

	// Execution dispatch (Phase 2): after notifier handling, fan out every
	// enriched signal as a TradeSignal envelope. The dispatcher is responsible
	// for kill-switch checks, dedup, per-plugin filtering, and HMAC signing —
	// the pipeline simply converts + calls Dispatch in a fire-and-forget loop.
	if p.dispatcher != nil {
		for i := range enriched {
			p.dispatcher.Dispatch(ctx, models.ToTradeSignal(enriched[i]))
		}
	}
}

// filterMTFConsensus returns only signals whose direction has signals
// from at least minTFs distinct timeframes. NEUTRAL signals are always kept.
func filterMTFConsensus(signals []models.Signal, minTFs int) []models.Signal {
	dirTFs := make(map[string]map[string]struct{})
	for _, sig := range signals {
		if sig.Direction == "NEUTRAL" {
			continue
		}
		if dirTFs[sig.Direction] == nil {
			dirTFs[sig.Direction] = make(map[string]struct{})
		}
		dirTFs[sig.Direction][sig.Timeframe] = struct{}{}
	}
	out := signals[:0]
	for _, sig := range signals {
		if sig.Direction == "NEUTRAL" || len(dirTFs[sig.Direction]) >= minTFs {
			out = append(out, sig)
		}
	}
	return out
}

// htfContext determines the higher-timeframe trend direction from EMA_50/EMA_200,
// current price position, and ADX trend strength.
//
// Returns "LONG" (uptrend), "SHORT" (downtrend), or "" (ranging/unknown).
//
// ADX integration:
//   - ADX < 20 → ranging regardless of EMA position (weak/no trend)
//   - ADX >= 20 → use EMA_50 vs EMA_200 + price position for direction
//
// Without ADX data, falls back to pure EMA logic.
func htfContext(indicators map[string]float64, tf string, bars map[string][]models.OHLCV) string {
	ema50, has50 := indicators[tf+":EMA_50"]
	ema200, has200 := indicators[tf+":EMA_200"]
	if !has50 || !has200 {
		return "" // insufficient data
	}

	b, ok := bars[tf]
	if !ok || len(b) == 0 {
		return ""
	}
	price := b[0].Close // most recent bar (bars are in DESC order in pipeline)

	// ADX check: if trend strength is weak, classify as ranging
	if adxVal, hasADX := indicators[tf+":ADX_14"]; hasADX && adxVal < 20 {
		return "" // ADX below 20 = no meaningful trend
	}

	if ema50 > ema200 && price > ema50 {
		return "LONG"
	}
	if ema50 < ema200 && price < ema50 {
		return "SHORT"
	}
	return "" // ranging
}

// penalizeHTFContext penalizes (or removes) lower-timeframe (1H, 4H) signals
// that contradict the higher-timeframe (1D, 1W) trend direction.
//
// penaltyPct controls behavior:
//   - 0   = no penalty (all signals pass through unchanged)
//   - 100 = full suppress (counter-trend signals removed entirely, legacy behavior)
//   - 50  = reduce counter-trend signal scores by 50%
//
// HTF signals (1D, 1W) and NEUTRAL signals are never penalized.
// If no clear HTF trend is detected, all signals pass.
//
// Wyckoff phase override: during accumulation/markup, even if EMA says bearish,
// the filter allows LONG signals through (trend overridden to ranging).
// During distribution/markdown, SHORT signals are allowed even if EMA says bullish.
func penalizeHTFContext(signals []models.Signal, indicators map[string]float64, bars map[string][]models.OHLCV, phase wyckoff.Phase, penaltyPct int) []models.Signal {
	if penaltyPct == 0 {
		return signals // no penalty — pass all signals through
	}

	// Determine HTF trend: prefer 1D, fall back to 1W
	trend := htfContext(indicators, "1D", bars)
	if trend == "" {
		trend = htfContext(indicators, "1W", bars)
	}

	// Wyckoff phase override: if phase contradicts or qualifies the EMA trend,
	// relax the filter to allow aligned signals through.
	switch phase {
	case wyckoff.PhaseAccumulation, wyckoff.PhaseMarkup:
		if trend == "SHORT" {
			trend = "" // override: accumulation in a bearish EMA = early reversal, allow LONG
		}
	case wyckoff.PhaseDistribution, wyckoff.PhaseMarkdown:
		if trend == "LONG" {
			trend = "" // override: distribution in a bullish EMA = early reversal, allow SHORT
		}
	}

	if trend == "" {
		return signals // ranging, no data, or Wyckoff override — pass everything through
	}

	out := make([]models.Signal, 0, len(signals))
	for _, sig := range signals {
		// Always keep NEUTRAL, 1D, and 1W signals unchanged
		if sig.Direction == "NEUTRAL" || sig.Timeframe == "1D" || sig.Timeframe == "1W" {
			out = append(out, sig)
			continue
		}
		// Aligned with HTF trend → keep unchanged
		if sig.Direction == trend {
			out = append(out, sig)
			continue
		}
		// Counter-trend LTF signal: apply penalty
		if penaltyPct >= 100 {
			continue // full suppress — remove signal entirely
		}
		sig.Score *= (1.0 - float64(penaltyPct)/100.0)
		out = append(out, sig)
	}
	return out
}

// filterHTFContext is the legacy wrapper that fully suppresses counter-trend signals.
// It is kept for backward compatibility with tests.
func filterHTFContext(signals []models.Signal, indicators map[string]float64, bars map[string][]models.OHLCV, phase wyckoff.Phase) []models.Signal {
	return penalizeHTFContext(signals, indicators, bars, phase, 100)
}

// enrichSignalLevels fills sig.EntryPrice, sig.TP, and sig.SL using ATR(14).
//
//	TP = entry ± ATR × tpMult
//	SL = entry ∓ ATR × slMult
//
// allBars is expected in DESC order (index 0 = most recent bar).
// A signal with Direction == "NEUTRAL" or an unavailable ATR is left unchanged.
func enrichSignalLevels(sig *models.Signal, allBars map[string][]models.OHLCV, indicators map[string]float64, tpMult, slMult float64) {
	if sig.Direction == "NEUTRAL" {
		return
	}
	bars, ok := allBars[sig.Timeframe]
	if !ok || len(bars) == 0 {
		return
	}
	atr := indicators[sig.Timeframe+":ATR_14"]
	if atr <= 0 {
		return
	}
	entry := bars[0].Close
	sig.EntryPrice = entry
	if sig.Direction == "LONG" {
		sig.TP = entry + atr*tpMult
		sig.SL = entry - atr*slMult
	} else {
		sig.TP = entry - atr*tpMult
		sig.SL = entry + atr*slMult
	}
}

