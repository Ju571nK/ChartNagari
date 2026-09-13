package brokerplugin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	SignatureHeader = "X-ChartNagari-Signature-256"
	TimestampHeader = "X-ChartNagari-Timestamp"
	PluginIDHeader  = "X-ChartNagari-Plugin-Id"
	maxBody         = 1 << 20
)

// Sign covers the escaped path AND query for v1 (URL.RequestURI()). Legacy
// /webhook signatures use just the path; do not change that legacy contract.
func Sign(secret, pluginID string, timestamp int64, method, target string, body []byte) string {
	sum := sha256.Sum256(body)
	canonical := pluginID + "\n" + strconv.FormatInt(timestamp, 10) + "\n" + method + "\n" + target + "\n" + hex.EncodeToString(sum[:])
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil))
}

// Client performs exactly one HTTP attempt. It does not retry orders or follow
// redirects (which could forward authentication to another endpoint).
type Client struct {
	base, id, secret string
	http             *http.Client
}

func (c *Client) PluginID() string { return c.id }

func NewClient(base, id, secret string) (*Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("plugin URL must be an origin")
	}
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && loopback) {
		return nil, errors.New("remote plugins require HTTPS")
	}
	if !idPattern.MatchString(id) || secret == "" {
		return nil, errors.New("plugin ID and secret required")
	}
	return &Client{base: strings.TrimRight(base, "/"), id: id, secret: secret, http: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// Do returns the HTTP status even for protocol errors. A transport/decode error
// after a write is not evidence that the broker did not receive the order.
func (c *Client) Do(ctx context.Context, method, target string, payload, output any) (int, error) {
	u, err := url.Parse(target)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(target, "/v1/") || u.Fragment != "" {
		return 0, errors.New("invalid v1 request target")
	}
	var body []byte
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return 0, err
		}
	}
	r, err := http.NewRequestWithContext(ctx, method, c.base+target, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	ts := time.Now().Unix()
	r.Header.Set(PluginIDHeader, c.id)
	r.Header.Set(TimestampHeader, strconv.FormatInt(ts, 10))
	r.Header.Set(SignatureHeader, Sign(c.secret, c.id, ts, method, r.URL.RequestURI(), body))
	r.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(r)
	if err != nil {
		return 0, errors.New("plugin transport failed; reconcile writes by client_order_id")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil || len(b) > maxBody {
		return resp.StatusCode, errors.New("invalid plugin response size")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("plugin returned HTTP %d", resp.StatusCode)
	}
	if output != nil && json.Unmarshal(b, output) != nil {
		return resp.StatusCode, errors.New("invalid plugin JSON response")
	}
	return resp.StatusCode, nil
}
