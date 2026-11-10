// Package rakutenfx connects a Rakuten Securities MT4 DEMO terminal to the
// order-plugin v1 protocol. Rakuten offers no public FX API, so the MQL4 EA in
// cmd/plugin-rakutenfx pushes account/order snapshots to this bridge.
//
// Read-only by design: nothing here can place, modify or cancel an MT4 order.
package rakutenfx

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

const (
	DefaultDemoServer = "RakutenSecurities-Demo"
	TokenHeader       = "X-ChartNagari-Bridge-Token"
	SnapshotPath      = "/bridge/v1/snapshot"
	maxSnapshotAge    = 60 * time.Second // EA pushes every few seconds; older data means the terminal is gone.
	maxOrders         = 200
)

var errOffline = errors.New("no recent MT4 snapshot")

// snapshot is the EA payload. Amounts are decimal strings, as in the v1 protocol.
type snapshot struct {
	Login      string `json:"login"`
	Server     string `json:"server"`
	TradeMode  string `json:"trade_mode"` // "demo" or "live", from MQL4 IsDemo()
	Currency   string `json:"currency"`
	FreeMargin string `json:"free_margin"`
	Orders     []struct {
		Ticket string `json:"ticket"`
		Symbol string `json:"symbol"`
		Status string `json:"status"` // filled, submitted, cancelled, expired
		Units  string `json:"units"`  // lots × contract size
	} `json:"orders"`
}

type Bridge struct {
	id, token, server string
	now               func() time.Time

	mu       sync.RWMutex
	account  plugin.Account
	orders   []plugin.Order // replaced, never mutated, per snapshot
	received time.Time
}

func New(pluginID, bridgeToken, demoServer string) (*Bridge, error) {
	if len(bridgeToken) < 32 {
		return nil, errors.New("bridge_token must contain at least 32 characters")
	}
	if demoServer == "" {
		demoServer = DefaultDemoServer
	}
	return &Bridge{id: pluginID, token: bridgeToken, server: demoServer, now: time.Now}, nil
}

// ServeHTTP accepts EA snapshots on SnapshotPath. Live accounts are refused and
// also drop any stored demo snapshot, so a terminal switched to live goes dark.
func (b *Bridge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost || r.URL.Path != SnapshotPath {
		http.Error(w, "not_found", http.StatusNotFound)
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(TokenHeader)), []byte(b.token)) != 1 {
		http.Error(w, "authentication_failed", http.StatusUnauthorized)
		return
	}
	var s snapshot
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if d.Decode(&s) != nil {
		http.Error(w, "invalid_snapshot", http.StatusBadRequest)
		return
	}
	if s.TradeMode != "demo" || s.Server != b.server {
		b.store(plugin.Account{}, nil, time.Time{})
		http.Error(w, "demo_account_required", http.StatusForbidden)
		return
	}
	account, orders, err := convert(s)
	if err != nil {
		http.Error(w, "invalid_snapshot", http.StatusBadRequest)
		return
	}
	b.store(account, orders, b.now())
	w.WriteHeader(http.StatusNoContent)
}

func (b *Bridge) store(a plugin.Account, o []plugin.Order, at time.Time) {
	b.mu.Lock()
	b.account, b.orders, b.received = a, o, at
	b.mu.Unlock()
}

func convert(s snapshot) (plugin.Account, []plugin.Order, error) {
	// MT4 demo maps to protocol "paper"; v1 has no live mode.
	a := plugin.Account{ID: "mt4-" + s.Login, Mode: "paper", Currency: s.Currency, BuyingPower: s.FreeMargin}
	if a.Validate() != nil || len(s.Orders) > maxOrders {
		return a, nil, errors.New("invalid account")
	}
	orders := make([]plugin.Order, 0, len(s.Orders))
	seen := map[string]bool{}
	for _, in := range s.Orders {
		o := plugin.Order{ClientOrderID: "mt4-" + in.Ticket, BrokerOrderID: in.Ticket, AccountID: a.ID, Symbol: in.Symbol, Status: in.Status, Quantity: in.Units, FilledQuantity: "0"}
		switch in.Status {
		case "filled":
			o.FilledQuantity = in.Units
		case "submitted", "cancelled", "expired":
		default:
			return a, nil, errors.New("invalid order status")
		}
		badSymbol := in.Symbol == "" || len(in.Symbol) > 32 || strings.ContainsFunc(in.Symbol, func(r rune) bool { return r <= ' ' || r == 0x7f })
		if badSymbol || seen[o.ClientOrderID] || o.Validate() != nil {
			return a, nil, errors.New("invalid order")
		}
		seen[o.ClientOrderID] = true
		orders = append(orders, o)
	}
	return a, orders, nil
}

func (b *Bridge) current() (plugin.Account, []plugin.Order, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.received.IsZero() || b.now().Sub(b.received) > maxSnapshotAge {
		return plugin.Account{}, nil, errOffline
	}
	return b.account, b.orders, nil
}

func (b *Bridge) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ProtocolVersion: plugin.ProtocolVersion, ID: b.id, Name: "Rakuten Securities MT4 demo bridge (read-only)", Version: "0.1.0", Modes: []string{"paper"},
		Capabilities: plugin.Capabilities{OrderTypes: []string{}, TimeInForce: []string{}, AssetClasses: []string{}},
		Settings: []plugin.SettingField{
			{Key: "plugin_secret", Label: "Shared HMAC secret", Type: "secret", Required: true},
			{Key: "bridge_token", Label: "MT4 EA bridge token", Type: "secret", Required: true},
			{Key: "listen", Label: "Listener address", Type: "text", Required: true},
			{Key: "demo_server", Label: "MT4 demo server name", Type: "text", Required: false},
		},
	}
}

func (b *Bridge) Accounts(context.Context) ([]plugin.Account, error) {
	a, _, err := b.current()
	if err != nil {
		return nil, err
	}
	return []plugin.Account{a}, nil
}

// Orders is never complete: the EA sends open orders plus a bounded slice of
// the terminal's loaded history.
func (b *Bridge) Orders(_ context.Context, account string) (plugin.OrderPage, error) {
	a, orders, err := b.current()
	if err != nil {
		return plugin.OrderPage{}, err
	}
	if account != a.ID {
		return plugin.OrderPage{}, plugin.ErrNotFound
	}
	return plugin.OrderPage{Orders: orders, Complete: false}, nil
}

func (b *Bridge) Order(_ context.Context, account, id string) (plugin.Order, error) {
	a, orders, err := b.current()
	if err != nil {
		return plugin.Order{}, err
	}
	for _, o := range orders {
		if account == a.ID && o.ClientOrderID == id {
			return o, nil
		}
	}
	return plugin.Order{}, plugin.ErrNotFound
}

func (b *Bridge) Submit(context.Context, plugin.OrderRequest) (plugin.Order, error) {
	return plugin.Order{}, plugin.ErrUnsupported
}
