package alpaca

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/execution"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func TestFeedbackSender_Send_SignsCorrectly(t *testing.T) {
	t.Parallel()

	const (
		pluginID = "alpaca-paper"
		secret   = "super-secret"
	)
	fixedNow := time.Unix(1_700_000_000, 0).UTC()

	var gotBody []byte
	var gotHeaders http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotHeaders = r.Header.Clone()

		pid := r.Header.Get(execution.PluginIDHeader)
		tsRaw := r.Header.Get(execution.TimestampHeader)
		sig := r.Header.Get(execution.SignatureHeader)
		ts, err := execution.ParseTimestamp(tsRaw)
		if err != nil {
			t.Errorf("parse ts: %v", err)
		}
		if !execution.Verify(secret, pid, ts, r.Method, r.URL.Path, gotBody, sig) {
			t.Errorf("HMAC verify failed")
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender, err := NewFeedbackSender(srv.URL+"/api/execution/feedback", pluginID, secret)
	if err != nil {
		t.Fatalf("NewFeedbackSender: %v", err)
	}
	sender.WithNow(func() time.Time { return fixedNow })

	fb := models.OrderFeedback{SignalID: "sig-1", OrderID: "ord-1", Status: models.OrderStatusSubmitted}
	if err := sender.Send(context.Background(), fb); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotHeaders.Get(execution.PluginIDHeader) != pluginID {
		t.Errorf("plugin header = %q", gotHeaders.Get(execution.PluginIDHeader))
	}
	if gotHeaders.Get(execution.TimestampHeader) != strconv.FormatInt(fixedNow.Unix(), 10) {
		t.Errorf("timestamp header mismatch")
	}
	var decoded models.OrderFeedback
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode feedback: %v", err)
	}
	if decoded.SignalID != "sig-1" || decoded.OrderID != "ord-1" || decoded.PluginName != pluginID {
		t.Errorf("unexpected feedback: %+v", decoded)
	}
}

func TestFeedbackSender_Send_NonSuccess(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	sender, err := NewFeedbackSender(srv.URL+"/api/execution/feedback", "pid", "s")
	if err != nil {
		t.Fatalf("NewFeedbackSender: %v", err)
	}
	err = sender.Send(context.Background(), models.OrderFeedback{SignalID: "s", Status: "X"})
	if err == nil {
		t.Fatal("expected non-2xx error")
	}
}

func TestFeedbackSender_BadURL(t *testing.T) {
	t.Parallel()
	if _, err := NewFeedbackSender("://bad", "p", "s"); err == nil {
		t.Fatal("expected parse error")
	}
}

func TestFeedbackSender_Send_IncludesSymbol(t *testing.T) {
	t.Parallel()

	const (
		pluginID = "alpaca-paper"
		secret   = "super-secret"
	)

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	sender, err := NewFeedbackSender(srv.URL+"/api/execution/feedback", pluginID, secret)
	if err != nil {
		t.Fatalf("NewFeedbackSender: %v", err)
	}

	fb := models.OrderFeedback{
		SignalID: "sig-2",
		OrderID:  "ord-2",
		Status:   models.OrderStatusSubmitted,
		Symbol:   "AAPL",
	}
	if err := sender.Send(context.Background(), fb); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if !bytes.Contains(gotBody, []byte(`"symbol":"AAPL"`)) {
		t.Errorf("outbound feedback body missing symbol; got: %s", gotBody)
	}

	var decoded models.OrderFeedback
	if err := json.Unmarshal(gotBody, &decoded); err != nil {
		t.Fatalf("decode feedback: %v", err)
	}
	if decoded.Symbol != "AAPL" {
		t.Errorf("decoded Symbol = %q, want AAPL", decoded.Symbol)
	}
}
