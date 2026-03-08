// Package pipeline connects the data storage, indicator engine, rule engine,
// AI interpreter, and notifier into a single periodic analysis loop.
//
// Flow (per symbol, per tick):
//
//	SQLite OHLCV → indicator.Compute → engine.Run → interpreter.Enrich → notifier.Notify
package pipeline

import (
	"context"
	"time"

	"github.com/rs/zerolog"

	"github.com/Ju571nK/Chatter/internal/engine"
	"github.com/Ju571nK/Chatter/internal/indicator"
	"github.com/Ju571nK/Chatter/internal/interpreter"
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

// Config controls pipeline timing and data parameters.
type Config struct {
	Interval time.Duration // how often to run analysis (default: 1 minute)
	Lookback int           // bars to load per TF (default: 200)
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		Interval: time.Minute,
		Lookback: 200,
	}
}

// Pipeline periodically reads OHLCV data, computes indicators, runs rules,
// applies AI interpretation for high-scoring signals, and dispatches notifications.
// It is safe to call Run once per instance.
type Pipeline struct {
	cfg        Config
	db         OHLCVReader
	sigSaver   SignalSaver // optional; set via SetSignalSaver
	eng        *engine.RuleEngine
	interp     *interpreter.Interpreter
	notif      *notifier.Notifier
	symbols    []string
	timeframes []string
	log        zerolog.Logger
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
		cfg:        cfg,
		db:         db,
		eng:        eng,
		interp:     interp,
		notif:      notif,
		symbols:    symbols,
		timeframes: timeframes,
		log:        log,
	}
}

// SetSignalSaver wires an optional signal persistence store.
// Call before Run; safe to call only once.
func (p *Pipeline) SetSignalSaver(ss SignalSaver) {
	p.sigSaver = ss
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
	for _, sym := range p.symbols {
		p.analyzeSymbol(ctx, sym)
	}
}

func (p *Pipeline) analyzeSymbol(ctx context.Context, sym string) {
	// Load OHLCV bars for each configured timeframe.
	allBars := make(map[string][]models.OHLCV, len(p.timeframes))
	for _, tf := range p.timeframes {
		bars, err := p.db.GetOHLCV(sym, tf, p.cfg.Lookback)
		if err != nil {
			p.log.Error().Err(err).Str("symbol", sym).Str("tf", tf).Msg("OHLCV 로드 실패")
			continue
		}
		if len(bars) > 0 {
			allBars[tf] = bars
		}
	}

	if len(allBars) == 0 {
		p.log.Debug().Str("symbol", sym).Msg("OHLCV 데이터 없음 — 분석 스킵")
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

	// Persist signals for chart dashboard markers.
	if p.sigSaver != nil {
		for _, sig := range signals {
			if err := p.sigSaver.SaveSignal(sig); err != nil {
				p.log.Warn().Err(err).Str("symbol", sym).Msg("신호 저장 실패")
			}
		}
	}

	if len(signals) == 0 {
		p.log.Debug().Str("symbol", sym).Msg("신호 없음")
		return
	}

	p.log.Info().
		Str("symbol", sym).
		Int("signals", len(signals)).
		Float64("top_score", signals[0].Score).
		Msg("신호 감지 — AI 해석 시작")

	// AI enrichment: Claude interprets high-scoring signal groups.
	group := interpreter.SignalGroup{
		Symbol:     sym,
		Signals:    signals,
		Indicators: indicators,
	}
	enriched := p.interp.Enrich(ctx, []interpreter.SignalGroup{group})

	// Notify: filters by score threshold and cooldown.
	p.notif.Notify(ctx, enriched)
}
