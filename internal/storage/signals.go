package storage

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/pkg/models"
)

// SaveSignal persists a generated signal to the database and returns the new row id.
func (db *DB) SaveSignal(sig models.Signal) (int64, error) {
	meta, _ := json.Marshal(signalFXMeta{AssetClass: sig.AssetClass, VolumeQuality: sig.VolumeQuality, DataProxy: sig.DataProxy, DataProvider: sig.DataProvider, ProviderSymbol: sig.ProviderSymbol, SourceIdentity: sig.SourceIdentity, VolumeUnconfirmed: sig.VolumeUnconfirmed, MessageKey: sig.MessageKey, MessageParams: sig.MessageParams, SpreadPips: sig.SpreadPips, SpreadPipsSet: sig.SpreadPipsSet, TPPips: sig.TPPips, SLPips: sig.SLPips, EntryPrice: sig.EntryPrice, TP: sig.TP, SL: sig.SL})
	res, err := db.conn.Exec(`
		INSERT INTO signals (symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high, htf_trend, atr_percentile, created_at, fx_meta)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sig.Symbol,
		sig.Timeframe,
		sig.Rule,
		sig.Direction,
		sig.Score,
		sig.Message,
		sig.AIInterpretation,
		sig.ZoneLow,
		sig.ZoneHigh,
		sig.HTFTrend,
		sig.ATRPercentile,
		sig.CreatedAt.Unix(),
		string(meta),
	)
	if err != nil {
		return 0, fmt.Errorf("신호 저장 실패 [%s %s]: %w", sig.Symbol, sig.Rule, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("LastInsertId 실패 [%s %s]: %w", sig.Symbol, sig.Rule, err)
	}
	return id, nil
}

type signalFXMeta struct {
	AssetClass        models.AssetClass    `json:"asset_class,omitempty"`
	VolumeQuality     models.VolumeQuality `json:"volume_quality,omitempty"`
	DataProxy         bool                 `json:"data_proxy,omitempty"`
	DataProvider      string               `json:"data_provider,omitempty"`
	ProviderSymbol    string               `json:"provider_symbol,omitempty"`
	SourceIdentity    string               `json:"source_identity,omitempty"`
	VolumeUnconfirmed bool                 `json:"volume_unconfirmed,omitempty"`
	MessageKey        string               `json:"message_key,omitempty"`
	MessageParams     map[string]string    `json:"message_params,omitempty"`
	SpreadPips        float64              `json:"spread_pips,omitempty"`
	SpreadPipsSet     bool                 `json:"spread_pips_set,omitempty"`
	TPPips            float64              `json:"tp_pips,omitempty"`
	SLPips            float64              `json:"sl_pips,omitempty"`
	EntryPrice        float64              `json:"entry_price,omitempty"`
	TP                float64              `json:"tp,omitempty"`
	SL                float64              `json:"sl,omitempty"`
}

func applySignalMeta(s *models.Signal, raw string) {
	var m signalFXMeta
	if json.Unmarshal([]byte(raw), &m) != nil {
		return
	}
	s.AssetClass = m.AssetClass
	s.VolumeQuality = m.VolumeQuality
	s.DataProxy = m.DataProxy
	s.DataProvider = m.DataProvider
	s.ProviderSymbol = m.ProviderSymbol
	s.SourceIdentity = m.SourceIdentity
	s.VolumeUnconfirmed = m.VolumeUnconfirmed
	s.MessageKey = m.MessageKey
	s.MessageParams = m.MessageParams
	s.SpreadPips = m.SpreadPips
	s.SpreadPipsSet = m.SpreadPipsSet
	s.TPPips = m.TPPips
	s.SLPips = m.SLPips
	s.EntryPrice = m.EntryPrice
	s.TP = m.TP
	s.SL = m.SL
}

// GetLatestSignalTime returns the Unix-second timestamp of the most recent signal,
// or 0 if no signals have been saved yet.
func (db *DB) GetLatestSignalTime() (int64, error) {
	var ts int64
	err := db.conn.QueryRow(`SELECT COALESCE(MAX(created_at), 0) FROM signals`).Scan(&ts)
	return ts, err
}

// GetSignalsByDate returns all signals for a symbol on the given calendar date (UTC).
func (db *DB) GetSignalsByDate(symbol string, date time.Time) ([]models.Signal, error) {
	start := time.Date(date.UTC().Year(), date.UTC().Month(), date.UTC().Day(), 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)

	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high,
		       forward_return_5d, forward_return_10d, forward_return_20d, forward_return_40d,
		       htf_trend, atr_percentile, created_at, fx_meta
		FROM signals
		WHERE symbol = ? AND created_at >= ? AND created_at < ?
		ORDER BY created_at DESC`,
		symbol, start.Unix(), end.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []models.Signal
	for rows.Next() {
		var s models.Signal
		var createdAtUnix int64
		var fxMeta string
		if err := rows.Scan(
			&s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation,
			&s.ZoneLow, &s.ZoneHigh,
			&s.ForwardReturn5d, &s.ForwardReturn10d, &s.ForwardReturn20d, &s.ForwardReturn40d,
			&s.HTFTrend, &s.ATRPercentile,
			&createdAtUnix, &fxMeta,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		applySignalMeta(&s, fxMeta)
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// GetSignalsFiltered returns signals with optional filters.
// symbol="ALL" → all symbols. direction="ALL" → all directions.
func (db *DB) GetSignalsFiltered(symbol, direction string, limit int) ([]models.Signal, error) {
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high,
		       forward_return_5d, forward_return_10d, forward_return_20d, forward_return_40d,
		       htf_trend, atr_percentile, created_at, fx_meta
		FROM signals
		WHERE (? = 'ALL' OR symbol = ?)
		  AND (? = 'ALL' OR direction = ?)
		ORDER BY created_at DESC
		LIMIT ?`,
		symbol, symbol, direction, direction, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []models.Signal
	for rows.Next() {
		var s models.Signal
		var id int64
		var createdAtUnix int64
		var fxMeta string
		if err := rows.Scan(
			&id, &s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation,
			&s.ZoneLow, &s.ZoneHigh,
			&s.ForwardReturn5d, &s.ForwardReturn10d, &s.ForwardReturn20d, &s.ForwardReturn40d,
			&s.HTFTrend, &s.ATRPercentile,
			&createdAtUnix, &fxMeta,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		applySignalMeta(&s, fxMeta)
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// GetSignals retrieves the N most recent signals for a given symbol.
func (db *DB) GetSignals(symbol string, limit int) ([]models.Signal, error) {
	rows, err := db.conn.Query(`
		SELECT symbol, timeframe, rule, direction, score, message, ai_interpretation, zone_low, zone_high,
		       forward_return_5d, forward_return_10d, forward_return_20d, forward_return_40d,
		       htf_trend, atr_percentile, created_at, fx_meta
		FROM signals
		WHERE symbol = ?
		ORDER BY created_at DESC
		LIMIT ?`,
		symbol, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sigs []models.Signal
	for rows.Next() {
		var s models.Signal
		var createdAtUnix int64
		var fxMeta string
		if err := rows.Scan(
			&s.Symbol, &s.Timeframe, &s.Rule, &s.Direction, &s.Score, &s.Message, &s.AIInterpretation,
			&s.ZoneLow, &s.ZoneHigh,
			&s.ForwardReturn5d, &s.ForwardReturn10d, &s.ForwardReturn20d, &s.ForwardReturn40d,
			&s.HTFTrend, &s.ATRPercentile,
			&createdAtUnix, &fxMeta,
		); err != nil {
			return nil, err
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		applySignalMeta(&s, fxMeta)
		sigs = append(sigs, s)
	}
	return sigs, rows.Err()
}

// SignalForForwardReturn is a lightweight struct for forward return processing.
type SignalForForwardReturn struct {
	ID             int64
	Symbol         string
	Timeframe      string
	EntryPrice     float64
	AssetClass     models.AssetClass
	DataProvider   string
	ProviderSymbol string
	SourceIdentity string
	DataProxy      bool
	CreatedAt      time.Time
	FR5d           float64
	FR10d          float64
	FR20d          float64
	FR40d          float64
}

// GetSignalsNeedingForwardReturn returns signals where forward_return_40d == 0
// and created_at is at least minAgeDays old. The pipeline caps eligible work;
// limiting this query would let unverifiable old FX rows starve later signals.
func (db *DB) GetSignalsNeedingForwardReturn(minAgeDays int) ([]SignalForForwardReturn, error) {
	cutoff := time.Now().Add(-time.Duration(minAgeDays) * 24 * time.Hour).Unix()
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe,
		       COALESCE((SELECT close FROM ohlcv WHERE ohlcv.symbol = signals.symbol AND ohlcv.timeframe = '1D'
		                 AND ohlcv.open_time <= signals.created_at * 1000 ORDER BY ohlcv.open_time DESC LIMIT 1), 0) AS legacy_entry_price,
		       forward_return_5d, forward_return_10d, forward_return_20d, forward_return_40d,
		       created_at, fx_meta
		FROM signals
		WHERE forward_return_40d = 0 AND created_at <= ?
		ORDER BY created_at ASC`,
		cutoff,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SignalForForwardReturn
	for rows.Next() {
		var s SignalForForwardReturn
		var createdAtUnix int64
		var legacyEntryPrice float64
		var fxMeta string
		if err := rows.Scan(&s.ID, &s.Symbol, &s.Timeframe, &legacyEntryPrice,
			&s.FR5d, &s.FR10d, &s.FR20d, &s.FR40d,
			&createdAtUnix, &fxMeta); err != nil {
			return nil, err
		}
		var persisted models.Signal
		applySignalMeta(&persisted, fxMeta)
		s.AssetClass = persisted.AssetClass
		s.DataProvider = persisted.DataProvider
		s.ProviderSymbol = persisted.ProviderSymbol
		s.SourceIdentity = persisted.SourceIdentity
		s.DataProxy = persisted.DataProxy
		// Stock and crypto retain the historical 1D-close entry convention.
		// FX must use the immutable signal entry, even if it was missing.
		s.EntryPrice = legacyEntryPrice
		if s.AssetClass == models.AssetForex || s.SourceIdentity != "" || s.DataProvider != "" {
			s.EntryPrice = persisted.EntryPrice
		}
		s.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		out = append(out, s)
	}
	return out, rows.Err()
}

// UpdateForwardReturns updates the forward return columns for a signal.
func (db *DB) UpdateForwardReturns(signalID int64, r5, r10, r20, r40 float64) error {
	_, err := db.conn.Exec(`
		UPDATE signals SET forward_return_5d = ?, forward_return_10d = ?, forward_return_20d = ?, forward_return_40d = ?
		WHERE id = ?`,
		r5, r10, r20, r40, signalID,
	)
	return err
}
