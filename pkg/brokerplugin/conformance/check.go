// Package conformance provides read-only checks for external plugins. Passing
// checks is not a security audit or a guarantee of correct order execution.
package conformance

import (
	"context"
	"errors"
	"net/http"
	"net/url"

	plugin "github.com/Ju571nK/Chatter/pkg/brokerplugin"
)

type Report struct {
	Protocol string   `json:"protocol"`
	PluginID string   `json:"plugin_id"`
	Accounts int      `json:"accounts"`
	Checks   []string `json:"checks"`
	ReadOnly bool     `json:"read_only"`
}

// Inspect only calls GET manifest/accounts/orders. It never probes submission,
// cancellation or a broker's paper/live declaration by sending an order.
func Inspect(ctx context.Context, c *plugin.Client) (Report, error) {
	r := Report{ReadOnly: true, Checks: []string{}}
	var m plugin.Manifest
	if _, err := c.Do(ctx, http.MethodGet, "/v1/manifest", nil, &m); err != nil {
		return r, err
	}
	if err := m.Validate(); err != nil {
		return r, err
	}
	if m.ID != c.PluginID() {
		return r, errors.New("manifest identity mismatch")
	}
	r.Protocol = m.ProtocolVersion
	r.PluginID = m.ID
	r.Checks = append(r.Checks, "manifest")
	var accounts []plugin.Account
	if _, err := c.Do(ctx, http.MethodGet, "/v1/accounts", nil, &accounts); err != nil {
		return r, err
	}
	r.Accounts = len(accounts)
	if accounts == nil {
		return r, errors.New("accounts must be an array")
	}
	seen := map[string]bool{}
	for _, a := range accounts {
		if a.Validate() != nil || seen[a.ID] {
			return r, errors.New("invalid or duplicate account")
		}
		seen[a.ID] = true
		var page plugin.OrderPage
		if _, err := c.Do(ctx, http.MethodGet, "/v1/orders?account_id="+url.QueryEscape(a.ID), nil, &page); err != nil {
			return r, err
		}
		for _, o := range page.Orders {
			if o.AccountID != a.ID || o.Validate() != nil {
				return r, errors.New("invalid order identity")
			}
		}
		if page.Orders == nil {
			return r, errors.New("orders must be an array")
		}
	}
	r.Checks = append(r.Checks, "accounts", "order snapshots")
	return r, nil
}
