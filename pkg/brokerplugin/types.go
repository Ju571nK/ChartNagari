// Package brokerplugin is the public, language-neutral order-plugin protocol
// and Go server kit. It never loads third-party code into the ChartNagari process.
package brokerplugin

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
)

const ProtocolVersion = "1.0"

var (
	ErrNotFound    = errors.New("not found")
	ErrRejected    = errors.New("broker rejected request")
	ErrUnsupported = errors.New("unsupported operation")
	idPattern      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,47}$`)
	decimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,17})(\.[0-9]{1,12})?$`)
	symbolPattern  = regexp.MustCompile(`[\s\x00-\x1f\x7f]`)
)

// Amounts are decimal strings, never binary floating point. Currency is explicit.
type Account struct {
	ID          string `json:"id"`
	Mode        string `json:"mode"`
	Currency    string `json:"currency"`
	BuyingPower string `json:"buying_power"`
}

func (a Account) Validate() error {
	if !idPattern.MatchString(a.ID) || a.Mode != "paper" || len(a.Currency) < 3 || len(a.Currency) > 12 || !decimalPattern.MatchString(strings.TrimPrefix(a.BuyingPower, "-")) {
		return errors.New("invalid account response")
	}
	return nil
}

type Capabilities struct {
	Submit             bool     `json:"submit"`
	Cancel             bool     `json:"cancel"`
	Replace            bool     `json:"replace"`
	Events             bool     `json:"events"`
	OrderTypes         []string `json:"order_types"`
	TimeInForce        []string `json:"time_in_force"`
	AssetClasses       []string `json:"asset_classes"`
	FractionalQuantity bool     `json:"fractional_quantity"`
}

// SettingField describes configuration, not its values. No credentials/default
// secrets belong in a manifest. v1 does not remotely write these settings.
type SettingField struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Type     string   `json:"type"` // text, secret, select
	Required bool     `json:"required"`
	Options  []string `json:"options,omitempty"`
}

type Manifest struct {
	ProtocolVersion string         `json:"protocol_version"`
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Version         string         `json:"version"`
	Modes           []string       `json:"modes"`
	Simulation      bool           `json:"simulation"`
	Capabilities    Capabilities   `json:"capabilities"`
	Settings        []SettingField `json:"settings"`
	OrdersEnabled   bool           `json:"orders_enabled"` // injected by the host, false by default
}

func (m Manifest) Validate() error {
	if m.ProtocolVersion != ProtocolVersion || !idPattern.MatchString(m.ID) || m.Name == "" || len(m.Name) > 120 || m.Version == "" || len(m.Version) > 40 {
		return errors.New("invalid plugin identity or unsupported protocol version")
	}
	// Live support requires a separate execution/risk integration; fail closed.
	if len(m.Modes) != 1 || m.Modes[0] != "paper" {
		return errors.New("v1 kit supports paper mode only")
	}
	if m.Capabilities.Replace || m.Capabilities.Events {
		return errors.New("replace and events are reserved, not implemented in v1")
	}
	for _, list := range []struct{ values, allowed []string }{
		{m.Capabilities.OrderTypes, []string{"market"}},
		{m.Capabilities.TimeInForce, []string{"day", "gtc", "ioc", "fok"}},
		{m.Capabilities.AssetClasses, []string{"stock", "crypto"}},
	} {
		seen := map[string]bool{}
		if m.Capabilities.Submit && len(list.values) == 0 {
			return errors.New("submission capability requires supported order dimensions")
		}
		for _, value := range list.values {
			if seen[value] || !slices.Contains(list.allowed, value) {
				return errors.New("unsupported or duplicate capability")
			}
			seen[value] = true
		}
	}
	if len(m.Settings) > 32 {
		return errors.New("too many settings fields")
	}
	seen := map[string]bool{}
	for _, f := range m.Settings {
		if !idPattern.MatchString(f.Key) || seen[f.Key] || f.Label == "" || len(f.Label) > 120 || !slices.Contains([]string{"text", "secret", "select"}, f.Type) {
			return errors.New("invalid settings schema")
		}
		seen[f.Key] = true
		if f.Type == "select" && (len(f.Options) == 0 || len(f.Options) > 32) {
			return errors.New("select requires bounded options")
		}
		if f.Type != "select" && len(f.Options) > 0 {
			return errors.New("only select fields may contain options")
		}
		for _, option := range f.Options {
			if option == "" || len(option) > 120 {
				return errors.New("invalid settings option")
			}
		}
	}
	return nil
}

type OrderRequest struct {
	ClientOrderID string `json:"client_order_id"`
	AccountID     string `json:"account_id"`
	Mode          string `json:"mode"`
	Symbol        string `json:"symbol"`
	AssetClass    string `json:"asset_class"`
	Side          string `json:"side"`
	Type          string `json:"type"`
	TimeInForce   string `json:"time_in_force"`
	Quantity      string `json:"quantity"`
}

func (r OrderRequest) Validate(c Capabilities) error {
	if !idPattern.MatchString(r.ClientOrderID) || !idPattern.MatchString(r.AccountID) || r.Mode != "paper" {
		return errors.New("valid client_order_id, account_id and paper mode required")
	}
	if r.Symbol == "" || len(r.Symbol) > 64 || symbolPattern.MatchString(r.Symbol) {
		return errors.New("invalid symbol")
	}
	if r.Side != "buy" && r.Side != "sell" {
		return errors.New("side must be buy or sell")
	}
	if !slices.Contains(c.OrderTypes, r.Type) || r.Type != "market" || !slices.Contains(c.TimeInForce, r.TimeInForce) || !slices.Contains(c.AssetClasses, r.AssetClass) {
		return ErrUnsupported
	}
	if !decimalPattern.MatchString(r.Quantity) {
		return errors.New("invalid quantity")
	}
	q, ok := new(big.Rat).SetString(r.Quantity)
	if !ok || q.Sign() <= 0 || (!c.FractionalQuantity && !q.IsInt()) {
		return errors.New("invalid quantity")
	}
	return nil
}

type Order struct {
	ClientOrderID  string `json:"client_order_id"`
	BrokerOrderID  string `json:"broker_order_id,omitempty"`
	AccountID      string `json:"account_id"`
	Symbol         string `json:"symbol,omitempty"`
	Status         string `json:"status"` // submitted, partial_fill, filled, rejected, cancelled, expired, pending_cancel, unknown
	Quantity       string `json:"quantity,omitempty"`
	FilledQuantity string `json:"filled_quantity,omitempty"`
}

// OrderPage is explicitly bounded. A plugin must not claim complete history
// when its broker only returns a recent window.
type OrderPage struct {
	Orders   []Order `json:"orders"`
	Complete bool    `json:"complete"`
}

type CancelRequest struct {
	RequestID string `json:"request_id"`
	AccountID string `json:"account_id"`
}

// Broker implementations must verify account ownership and validate broker-
// specific instruments/constraints. Returning an error from Submit means the
// result may be UNKNOWN unless errors.Is(err, ErrRejected) is true.
type Broker interface {
	Manifest() Manifest
	Accounts(context.Context) ([]Account, error)
	Orders(context.Context, string) (OrderPage, error)
	Order(context.Context, string, string) (Order, error)
	Submit(context.Context, OrderRequest) (Order, error)
}

type Canceller interface {
	Cancel(context.Context, string, string) (Order, error)
}

func validateOrder(o Order, accountID, clientID string) error {
	if o.AccountID != accountID || o.ClientOrderID == "" || o.ClientOrderID != clientID || !slices.Contains([]string{"submitted", "partial_fill", "filled", "rejected", "cancelled", "expired", "pending_cancel", "unknown"}, o.Status) {
		return fmt.Errorf("invalid broker order response")
	}
	return o.Validate()
}

func (o Order) Validate() error {
	if !idPattern.MatchString(o.AccountID) || !idPattern.MatchString(o.ClientOrderID) || !slices.Contains([]string{"submitted", "partial_fill", "filled", "rejected", "cancelled", "expired", "pending_cancel", "unknown"}, o.Status) {
		return errors.New("invalid order identity or status")
	}
	for _, amount := range []string{o.Quantity, o.FilledQuantity} {
		if amount != "" && !decimalPattern.MatchString(amount) {
			return errors.New("invalid order quantity")
		}
	}
	if o.Status != "unknown" && o.Status != "rejected" && (o.Quantity == "" || o.FilledQuantity == "") {
		return errors.New("order quantities required")
	}
	if o.Quantity != "" && o.FilledQuantity != "" {
		q, _ := new(big.Rat).SetString(o.Quantity)
		f, _ := new(big.Rat).SetString(o.FilledQuantity)
		if q.Sign() <= 0 || f.Cmp(q) > 0 || (o.Status == "filled" && f.Cmp(q) != 0) || (o.Status == "partial_fill" && (f.Sign() <= 0 || f.Cmp(q) >= 0)) {
			return errors.New("inconsistent fill quantities")
		}
	}
	return nil
}
