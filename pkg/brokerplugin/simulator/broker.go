// Package simulator is a deterministic fake exchange. It has no network client
// or credentials and cannot place a real or Alpaca paper order.
package simulator

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"sync/atomic"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
	_ "modernc.org/sqlite"
)

type Broker struct {
	id          string
	db          *sql.DB
	submissions atomic.Int64
}

func Open(id, path string) (*Broker, error) {
	if path == "" {
		return nil, errors.New("simulator database path required")
	}
	if path != ":memory:" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return nil, err
		}
		_ = f.Close()
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, q := range []string{`PRAGMA busy_timeout=5000`, `PRAGMA journal_mode=WAL`, `PRAGMA synchronous=FULL`, `CREATE TABLE IF NOT EXISTS simulated_orders (id TEXT PRIMARY KEY, body BLOB NOT NULL)`} {
		if _, err = db.Exec(q); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	return &Broker{id: id, db: db}, nil
}
func (b *Broker) Close() error           { return b.db.Close() }
func (b *Broker) SubmissionCount() int64 { return b.submissions.Load() }
func (b *Broker) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ProtocolVersion: plugin.ProtocolVersion, ID: b.id, Name: "ChartNagari simulated exchange", Version: "1.0.0", Modes: []string{"paper"}, Simulation: true,
		Capabilities: plugin.Capabilities{Submit: true, Cancel: true, OrderTypes: []string{"market"}, TimeInForce: []string{"day"}, AssetClasses: []string{"stock"}},
		Settings:     []plugin.SettingField{{Key: "plugin_secret", Label: "Shared HMAC secret", Type: "secret", Required: true}, {Key: "listen", Label: "Listener address", Type: "text", Required: true}},
	}
}
func (b *Broker) Accounts(context.Context) ([]plugin.Account, error) {
	return []plugin.Account{{ID: "sim-account", Mode: "paper", Currency: "USD", BuyingPower: "100000.00"}}, nil
}
func (b *Broker) Orders(ctx context.Context, account string) (plugin.OrderPage, error) {
	if account != "sim-account" {
		return plugin.OrderPage{}, plugin.ErrNotFound
	}
	rows, err := b.db.QueryContext(ctx, `SELECT body FROM simulated_orders ORDER BY id LIMIT 101`)
	if err != nil {
		return plugin.OrderPage{}, err
	}
	defer rows.Close()
	page := plugin.OrderPage{Orders: []plugin.Order{}, Complete: true}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			return page, err
		}
		var o plugin.Order
		if err = json.Unmarshal(raw, &o); err != nil {
			return page, err
		}
		page.Orders = append(page.Orders, o)
	}
	if len(page.Orders) > 100 {
		page.Orders = page.Orders[:100]
		page.Complete = false
	}
	return page, rows.Err()
}
func (b *Broker) Order(ctx context.Context, account, id string) (plugin.Order, error) {
	if account != "sim-account" {
		return plugin.Order{}, plugin.ErrNotFound
	}
	var raw []byte
	err := b.db.QueryRowContext(ctx, `SELECT body FROM simulated_orders WHERE id=?`, id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return plugin.Order{}, plugin.ErrNotFound
	}
	if err != nil {
		return plugin.Order{}, err
	}
	var o plugin.Order
	err = json.Unmarshal(raw, &o)
	return o, err
}
func (b *Broker) Submit(ctx context.Context, r plugin.OrderRequest) (plugin.Order, error) {
	b.submissions.Add(1)
	if r.AccountID != "sim-account" || r.Validate(b.Manifest().Capabilities) != nil {
		return plugin.Order{}, plugin.ErrRejected
	}
	o := plugin.Order{ClientOrderID: r.ClientOrderID, BrokerOrderID: "sim-" + r.ClientOrderID, AccountID: r.AccountID, Symbol: r.Symbol, Quantity: r.Quantity, FilledQuantity: "0", Status: "submitted"}
	switch r.Symbol {
	case "SIM-FILL", "SIM-TIMEOUT":
		o.Status = "filled"
		o.FilledQuantity = r.Quantity
	case "SIM-PARTIAL":
		o.Status = "partial_fill"
		q, _ := new(big.Rat).SetString(r.Quantity)
		o.FilledQuantity = new(big.Rat).Quo(q, big.NewRat(2, 1)).FloatString(1)
	case "SIM-REJECT":
		o.Status = "rejected"
	case "SIM-PENDING":
	default:
		return plugin.Order{}, plugin.ErrRejected
	}
	raw, _ := json.Marshal(o)
	if _, err := b.db.ExecContext(ctx, `INSERT INTO simulated_orders(id,body) VALUES(?,?)`, r.ClientOrderID, raw); err != nil {
		return plugin.Order{}, err
	}
	if r.Symbol == "SIM-TIMEOUT" {
		return plugin.Order{}, errors.New("simulated response lost after broker accepted order")
	}
	return o, nil
}
func (b *Broker) Cancel(ctx context.Context, account, id string) (plugin.Order, error) {
	o, err := b.Order(ctx, account, id)
	if err != nil {
		return o, err
	}
	if o.Status == "filled" || o.Status == "rejected" {
		return plugin.Order{}, plugin.ErrRejected
	}
	o.Status = "cancelled" // Already filled quantity remains intact.
	raw, _ := json.Marshal(o)
	_, err = b.db.ExecContext(ctx, `UPDATE simulated_orders SET body=? WHERE id=?`, raw, id)
	return o, err
}
