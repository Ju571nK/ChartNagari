package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// FeedbackSender POSTs OrderFeedback to ChartNagari's
// /api/execution/feedback with an HMAC-SHA256 signature produced by the
// shared internal/execution.Sign helper — exact same canonical string format
// the dispatcher uses in the outbound direction. This guarantees we never
// drift from the ChartNagari side's Verify expectations.
type FeedbackSender struct {
	url      string
	pluginID string
	secret   string
	client   *http.Client
	nowFn    func() time.Time
	path     string // cached URL path (for canonical string)
}

// NewFeedbackSender builds a sender. urlStr must be the full URL including
// path; e.g. http://127.0.0.1:8080/api/execution/feedback.
func NewFeedbackSender(urlStr, pluginID, secret string) (*FeedbackSender, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid feedback url: %w", err)
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	return &FeedbackSender{
		url:      urlStr,
		pluginID: pluginID,
		secret:   secret,
		client:   &http.Client{Timeout: HTTPTimeout},
		nowFn:    time.Now,
		path:     path,
	}, nil
}

// WithNow swaps the clock — unit tests freeze time here so canonical strings
// are reproducible.
func (f *FeedbackSender) WithNow(now func() time.Time) *FeedbackSender {
	f.nowFn = now
	return f
}

// Send posts an OrderFeedback JSON body with the HMAC signature headers. It
// returns an error for any non-2xx response or transport failure so the
// caller can log it; the adapter intentionally does not retry feedback here
// — ChartNagari's auto-release window covers silent feedback loss.
func (f *FeedbackSender) Send(ctx context.Context, fb models.OrderFeedback) error {
	fb.PluginName = f.pluginID
	if fb.Timestamp.IsZero() {
		fb.Timestamp = f.nowFn().UTC()
	}
	body, err := json.Marshal(fb)
	if err != nil {
		return fmt.Errorf("marshal feedback: %w", err)
	}

	ts := f.nowFn().Unix()
	sig := execution.Sign(f.secret, f.pluginID, ts, http.MethodPost, f.path, body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("new feedback request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(execution.SignatureHeader, sig)
	req.Header.Set(execution.TimestampHeader, strconv.FormatInt(ts, 10))
	req.Header.Set(execution.PluginIDHeader, f.pluginID)

	resp, err := f.client.Do(req)
	if err != nil {
		return fmt.Errorf("feedback do: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("feedback non-2xx: %d", resp.StatusCode)
	}
	return nil
}
