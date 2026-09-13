package alpaca

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

// protocolBroker exposes the existing paper adapter through the public v1
// discovery/read contract. Submission stays on /webhook so v1 cannot bypass
// ChartNagari's dispatcher filters, deduplication and kill switch.
type protocolBroker struct {
	id     string
	client *AlpacaClient
}

func (b *protocolBroker) Manifest() plugin.Manifest {
	return plugin.Manifest{
		ProtocolVersion: plugin.ProtocolVersion, ID: b.id, Name: "Alpaca paper adapter", Version: "1.0.0", Modes: []string{"paper"},
		Capabilities: plugin.Capabilities{OrderTypes: []string{"market"}, TimeInForce: []string{"day"}, AssetClasses: []string{"stock"}},
		Settings: []plugin.SettingField{
			{Key: "ALPACA_API_KEY", Label: "Alpaca paper API key", Type: "secret", Required: true},
			{Key: "ALPACA_API_SECRET", Label: "Alpaca paper API secret", Type: "secret", Required: true},
			{Key: "ALPACA_NOTIONAL_PER_TRADE", Label: "Legacy webhook notional per trade (USD)", Type: "text", Required: true},
		},
	}
}

func (c *AlpacaClient) readJSON(ctx context.Context, path string, out any) error {
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	r.Header.Set("Accept", "application/json")
	r.Header.Set("APCA-API-KEY-ID", c.apiKey)
	r.Header.Set("APCA-API-SECRET-KEY", c.apiSecret)
	resp, err := c.http.Do(r)
	if err != nil {
		return errors.New("alpaca read failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return plugin.ErrNotFound
	}
	if resp.StatusCode != 200 {
		return errors.New("alpaca read rejected")
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return errors.New("invalid alpaca response size")
	}
	if json.Unmarshal(raw, out) != nil {
		return errors.New("invalid alpaca JSON")
	}
	return nil
}

func (b *protocolBroker) Accounts(ctx context.Context) ([]plugin.Account, error) {
	var account struct {
		ID          string `json:"id"`
		Currency    string `json:"currency"`
		BuyingPower string `json:"buying_power"`
	}
	if err := b.client.readJSON(ctx, "/v2/account", &account); err != nil {
		return nil, err
	}
	if account.ID == "" || account.Currency == "" {
		return nil, errors.New("invalid alpaca account")
	}
	return []plugin.Account{{ID: account.ID, Mode: "paper", Currency: account.Currency, BuyingPower: account.BuyingPower}}, nil
}

func (b *protocolBroker) checkAccount(ctx context.Context, id string) error {
	a, err := b.Accounts(ctx)
	if err != nil {
		return err
	}
	if a[0].ID != id {
		return plugin.ErrNotFound
	}
	return nil
}

func (b *protocolBroker) Orders(ctx context.Context, account string) (plugin.OrderPage, error) {
	if err := b.checkAccount(ctx, account); err != nil {
		return plugin.OrderPage{}, err
	}
	var orders []OrderResponse
	if err := b.client.readJSON(ctx, "/v2/orders?status=all&limit=100", &orders); err != nil {
		return plugin.OrderPage{}, err
	}
	page := plugin.OrderPage{Orders: []plugin.Order{}, Complete: false}
	for _, o := range orders {
		page.Orders = append(page.Orders, protocolOrder(o, account))
	}
	return page, nil
}

func (b *protocolBroker) Order(ctx context.Context, account, id string) (plugin.Order, error) {
	if err := b.checkAccount(ctx, account); err != nil {
		return plugin.Order{}, err
	}
	var o OrderResponse
	if err := b.client.readJSON(ctx, "/v2/orders:by_client_order_id?client_order_id="+url.QueryEscape(id), &o); err != nil {
		return plugin.Order{}, err
	}
	return protocolOrder(o, account), nil
}
func (b *protocolBroker) Submit(context.Context, plugin.OrderRequest) (plugin.Order, error) {
	return plugin.Order{}, plugin.ErrUnsupported
}

func protocolOrder(o OrderResponse, account string) plugin.Order {
	status := "unknown"
	switch o.Status {
	case "accepted", "pending_new", "new", "accepted_for_bidding":
		status = "submitted"
	case "partially_filled":
		status = "partial_fill"
	case "filled":
		status = "filled"
	case "canceled":
		status = "cancelled"
	case "expired":
		status = "expired"
	case "pending_cancel":
		status = "pending_cancel"
	case "rejected":
		status = "rejected"
	}
	return plugin.Order{ClientOrderID: o.ClientOrderID, BrokerOrderID: o.ID, AccountID: account, Symbol: o.Symbol, Status: status, Quantity: o.Qty, FilledQuantity: o.FilledQty}
}
