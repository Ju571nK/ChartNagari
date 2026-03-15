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
	SaveSignal(sig models.Signal) error
}

// PaperTrader tracks virtual paper positions driven by live signals.
// *paper.Trader satisfies this interface.
type PaperTrader interface {
	OnSignals(signals []models.Signal)
	CheckPositions(sym string, allBars map[string][]models.OHLCV)
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

// SetAlertConfigHolder wires an optional live-updated alert configuration holder.
func (p *Pipeline) SetAlertConfigHolder(h *appconfig.AlertConfigHolder) {
	p.alertHolder = h
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

	// Compute all indicators across all loaded timeframes.
	indicators := indicator.Compute(allBars)

	// Run the rule engine.
	analysisCtx := models.AnalysisContext{
		Symbol:     sym,
		Timeframes: allBars,
		Indicators: indicators,
	}
	signals := p.eng.Run(analysisCtx)

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
		for _, sig := range enriched {
			if sig.Score < p.cfg.SignalMinScore {
				continue
			}
			key := sig.Symbol + ":" + sig.Rule
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
			if err := p.sigSaver.SaveSignal(sig); err != nil {
				p.log.Warn().Err(err).Str("symbol", sym).Msg("signal save failed")
			}
		}
	}

	// Notify: filters by score threshold and cooldown.
	p.notif.Notify(ctx, enriched)
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
