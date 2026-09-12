package collector

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Ju571nK/Chatter/internal/storage"
)

type yahooTransport func(*http.Request) (*http.Response, error)

func (f yahooTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYahooVIXDailyPersistence(t *testing.T) {
	db, err := storage.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := NewYahooCollector(db, []string{"^VIX"}, []string{"1D"}, time.Hour)
	c.httpClient.Transport = yahooTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v8/finance/chart/^VIX" || r.URL.Query().Get("interval") != "1d" {
			t.Fatalf("unexpected index request: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatal("public index request contains credentials")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"chart":{"result":[{"timestamp":[1789084800],"indicators":{"quote":[{"open":[15],"high":[17],"low":[14],"close":[16],"volume":[0]}]}}],"error":null}}`))}, nil
	})
	c.fetchAll()
	bars, err := db.GetOHLCV("^VIX", "1D", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].Close != 16 {
		t.Fatalf("VIX daily bars: %+v", bars)
	}
}

func TestYahooOffHoursRespectsConfiguredTimeframes(t *testing.T) {
	for _, tc := range []struct{ input, want []string }{
		{[]string{"1D"}, []string{"1D"}},
		{[]string{"1H", "4H"}, nil},
		{[]string{"1H", "1D", "1W"}, []string{"1D", "1W"}},
	} {
		c := NewYahooCollector(nil, nil, tc.input, time.Hour)
		if got := c.offHoursTimeframes(); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %v, want %v", tc.input, got, tc.want)
		}
	}
}

// Explicit opt-in; ordinary test runs never depend on Yahoo availability.
func TestYahooVIXLive(t *testing.T) {
	if os.Getenv("CHARTTER_TEST_YAHOO_LIVE") != "1" {
		t.Skip("set CHARTTER_TEST_YAHOO_LIVE=1 for live provider verification")
	}
	c := NewYahooCollector(nil, nil, []string{"1D"}, time.Hour)
	bars, err := c.fetchOHLCV("^VIX", "1D")
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) < 20 {
		t.Fatalf("need 20 daily bars for VIX average; got %d", len(bars))
	}
	for _, bar := range bars {
		if bar.Symbol != "^VIX" || bar.Timeframe != "1D" || bar.Close <= 0 {
			t.Fatalf("invalid VIX bar: %+v", bar)
		}
	}
	t.Logf("received %d VIX daily bars; latest %s", len(bars), bars[len(bars)-1].OpenTime)
}
