package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/Ju571nK/Chatter/internal/paper"
)

// SavePaperPosition inserts a new OPEN paper position and returns its ID.
func (db *DB) SavePaperPosition(pos paper.PaperPosition) (int64, error) {
	res, err := db.conn.Exec(`
		INSERT INTO paper_positions
			(symbol, timeframe, rule, direction, entry_price, tp, sl, entry_time, asset_class, pip_size, spread_pips, data_provider, provider_symbol, source_identity, data_proxy)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pos.Symbol, pos.Timeframe, pos.Rule, pos.Direction,
		pos.EntryPrice, pos.TP, pos.SL, pos.EntryTime.Unix(), pos.AssetClass, pos.PipSize, pos.SpreadPips, pos.DataProvider, pos.ProviderSymbol, pos.SourceIdentity, pos.DataProxy,
	)
	if err != nil {
		return 0, fmt.Errorf("페이퍼 포지션 저장 실패: %w", err)
	}
	return res.LastInsertId()
}

// GetOpenPositions returns all OPEN positions for a symbol.
func (db *DB) GetOpenPositions(symbol string) ([]paper.PaperPosition, error) {
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe, rule, direction, entry_price, tp, sl, entry_time, asset_class, pip_size, spread_pips, data_provider, provider_symbol, source_identity, data_proxy, suspended, suspension_reason
		FROM paper_positions
		WHERE symbol = ? AND status = 'OPEN'
		ORDER BY entry_time DESC`,
		symbol,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOpenPositions(rows)
}

// GetAllOpenPositions returns all OPEN positions across all symbols.
func (db *DB) GetAllOpenPositions() ([]paper.PaperPosition, error) {
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe, rule, direction, entry_price, tp, sl, entry_time, asset_class, pip_size, spread_pips, data_provider, provider_symbol, source_identity, data_proxy, suspended, suspension_reason
		FROM paper_positions
		WHERE status = 'OPEN'
		ORDER BY entry_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanOpenPositions(rows)
}

// ClosePaperPosition updates a position to closed with exit details.
func (db *DB) ClosePaperPosition(id int64, exitPrice float64, status string, pnlPct float64) error {
	_, err := db.conn.Exec(`
		UPDATE paper_positions
		SET exit_price = ?, exit_time = ?, status = ?, pnl_pct = ?
		WHERE id = ?`,
		exitPrice, time.Now().Unix(), status, pnlPct, id,
	)
	if err != nil {
		return fmt.Errorf("페이퍼 포지션 청산 실패 [id=%d]: %w", id, err)
	}
	return nil
}

func (db *DB) ClosePaperPositionWithPips(id int64, exitPrice float64, status string, pnlPct, pnlPips float64) error {
	_, err := db.conn.Exec(`UPDATE paper_positions SET exit_price=?,exit_time=?,status=?,pnl_pct=?,pnl_pips=? WHERE id=?`, exitPrice, time.Now().Unix(), status, pnlPct, pnlPips, id)
	return err
}

// GetClosedPositions returns the N most recent closed positions.
func (db *DB) GetClosedPositions(limit int) ([]paper.PaperPosition, error) {
	rows, err := db.conn.Query(`
		SELECT id, symbol, timeframe, rule, direction, entry_price, tp, sl,
		       entry_time, exit_price, exit_time, status, pnl_pct, asset_class, pip_size, spread_pips, pnl_pips, data_provider, provider_symbol, source_identity, data_proxy, suspended, suspension_reason
		FROM paper_positions
		WHERE status != 'OPEN'
		ORDER BY exit_time DESC
		LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAllPositions(rows)
}

func scanOpenPositions(rows *sql.Rows) ([]paper.PaperPosition, error) {
	var positions []paper.PaperPosition
	for rows.Next() {
		var p paper.PaperPosition
		var entryUnix int64
		if err := rows.Scan(
			&p.ID, &p.Symbol, &p.Timeframe, &p.Rule, &p.Direction,
			&p.EntryPrice, &p.TP, &p.SL, &entryUnix, &p.AssetClass, &p.PipSize, &p.SpreadPips, &p.DataProvider, &p.ProviderSymbol, &p.SourceIdentity, &p.DataProxy, &p.Suspended, &p.SuspensionReason,
		); err != nil {
			return nil, err
		}
		p.EntryTime = time.Unix(entryUnix, 0).UTC()
		p.Status = "OPEN"
		paperExecutionPrices(&p)
		positions = append(positions, p)
	}
	return positions, rows.Err()
}

func scanAllPositions(rows *sql.Rows) ([]paper.PaperPosition, error) {
	var positions []paper.PaperPosition
	for rows.Next() {
		var p paper.PaperPosition
		var entryUnix, exitUnix int64
		if err := rows.Scan(
			&p.ID, &p.Symbol, &p.Timeframe, &p.Rule, &p.Direction,
			&p.EntryPrice, &p.TP, &p.SL,
			&entryUnix, &p.ExitPrice, &exitUnix, &p.Status, &p.PnLPct, &p.AssetClass, &p.PipSize, &p.SpreadPips, &p.PnLPips, &p.DataProvider, &p.ProviderSymbol, &p.SourceIdentity, &p.DataProxy, &p.Suspended, &p.SuspensionReason,
		); err != nil {
			return nil, err
		}
		p.EntryTime = time.Unix(entryUnix, 0).UTC()
		if exitUnix > 0 {
			t := time.Unix(exitUnix, 0).UTC()
			p.ExitTime = &t
		}
		paperExecutionPrices(&p)
		positions = append(positions, p)
	}
	return positions, rows.Err()
}

func paperExecutionPrices(p *paper.PaperPosition) {
	p.PriceBasis = "mid"
	if p.Status == "OPEN" && p.AssetClass == "forex" && p.SourceIdentity == "" {
		p.Suspended = true
		p.SuspensionReason = "unknown_origin"
	}
	if p.PipSize <= 0 {
		return
	}
	half := p.SpreadPips * p.PipSize / 2
	p.EntryExecutionPrice = p.EntryPrice + half
	p.ExitExecutionPrice = p.ExitPrice - half
	if p.Direction == "SHORT" {
		p.EntryExecutionPrice = p.EntryPrice - half
		p.ExitExecutionPrice = p.ExitPrice + half
	}
	if p.Status == "OPEN" {
		p.ExitExecutionPrice = 0
	}
}
