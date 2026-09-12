package calendar

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/rs/zerolog"
)

type brokenStore struct{ mockStore }

func (*brokenStore) UpsertEconomicEvents([]storage.EconomicEvent) error {
	return errors.New("private database path")
}

func TestCollectionDiagnostics(t *testing.T) {
	if New("", "", &mockStore{}, zerolog.Nop()).Status().State != "disabled" {
		t.Fatal("missing key should be disabled")
	}
	for _, tc := range []struct {
		code          int
		body, failure string
	}{
		{200, "[]", ""}, {401, "private response", "authentication"}, {403, "private response", "permission"},
		{402, "private response", "permission"}, {429, "private response", "rate_limit"}, {500, "private response", "provider"},
		{200, "{", "invalid_response"}, {200, "null", "invalid_response"},
	} {
		t.Run(tc.failure+tc.body, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.code); w.Write([]byte(tc.body)) }))
			defer srv.Close()
			f := newFMPFetcher(srv, &mockStore{})
			if f.Status().State != "waiting" {
				t.Fatal("not yet collected")
			}
			ok := f.tryFetch(context.Background())
			s := f.Status()
			if ok != (tc.failure == "") || s.Failure != tc.failure || s.LastAttempt.IsZero() {
				t.Fatalf("unexpected status: %+v", s)
			}
			if ok && (s.State != "success" || s.LastSuccess.IsZero() || s.EventCount != 0) {
				t.Fatalf("empty success: %+v", s)
			}
			if !ok && (!s.LastSuccess.IsZero() || s.State != "error") {
				t.Fatalf("false success: %+v", s)
			}
		})
	}
}

func TestCollectionStorageFailureAndRecovery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`[{"date":"2026-09-12","country":"US","event":"CPI"}]`))
	}))
	defer srv.Close()
	f := newFMPFetcher(srv, &brokenStore{})
	if f.tryFetch(context.Background()) || f.Status().Failure != "storage" {
		t.Fatal("storage failure must fail collection")
	}
	f.store = &mockStore{}
	if !f.tryFetch(context.Background()) || f.Status().EventCount != 1 || f.Status().Failure != "" {
		t.Fatal("recovery failed")
	}
	last := f.Status().LastSuccess
	srv.Close()
	if f.tryFetch(context.Background()) || f.Status().Failure != "network" || f.Status().LastSuccess != last {
		t.Fatal("keep last success on later failure")
	}
}
