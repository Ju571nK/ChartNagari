package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/forex"
	"github.com/Ju571nK/Chatter/internal/market"
	"github.com/Ju571nK/Chatter/pkg/models"
)

func (s *Server) getForexStrength(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		http.Error(w, "chart data unavailable", http.StatusServiceUnavailable)
		return
	}
	tf := r.URL.Query().Get("tf")
	if tf == "" {
		tf = "4H"
	}
	if tf != "1H" && tf != "4H" && tf != "1D" && tf != "1W" {
		http.Error(w, "invalid timeframe", http.StatusBadRequest)
		return
	}
	lookback := 20
	if raw := r.URL.Query().Get("lookback"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			http.Error(w, "invalid lookback", http.StatusBadRequest)
			return
		}
		lookback = n
	}
	wl, err := s.readWatchlist()
	if err != nil {
		http.Error(w, "watchlist unavailable", http.StatusInternalServerError)
		return
	}
	series := map[string][]models.OHLCV{}
	for _, entry := range wl.Symbols.Forex {
		if !entry.Enabled {
			continue
		}
		bars, e := s.chartStore.GetOHLCV(entry.Symbol, tf, lookback+1)
		if e != nil {
			http.Error(w, "candle query failed", http.StatusInternalServerError)
			return
		}
		series[entry.Symbol] = bars
	}
	items := forex.Strength(series, lookback)
	if items == nil {
		items = []forex.CurrencyStrength{}
	}
	jsonOK(w, map[string]any{"currencies": items})
}

func (s *Server) getForexDXY(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		http.Error(w, "chart data unavailable", http.StatusServiceUnavailable)
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	if len(symbol) != 6 || !strings.Contains(symbol, "USD") {
		http.Error(w, "USD forex pair required", http.StatusBadRequest)
		return
	}
	wl, err := s.readWatchlist()
	if err != nil {
		http.Error(w, "watchlist unavailable", 500)
		return
	}
	found := false
	for _, entry := range wl.Symbols.Forex {
		if entry.Symbol == symbol && entry.Enabled {
			found = true
			break
		}
	}
	if !found {
		http.Error(w, "pair is not enabled", http.StatusNotFound)
		return
	}
	dxyEnabled := false
	for _, entry := range wl.Symbols.Indices {
		if entry.Symbol == "DX-Y.NYB" && entry.Enabled {
			dxyEnabled = true
			break
		}
	}
	if !dxyEnabled {
		jsonOK(w, map[string]any{"state": "disabled", "bars": []any{}, "correlation_20": nil, "correlation_60": nil})
		return
	}
	pair, err := s.chartStore.GetOHLCV(symbol, "1D", 100)
	if err != nil {
		http.Error(w, "pair candles unavailable", 500)
		return
	}
	dxy, err := s.chartStore.GetOHLCV("DX-Y.NYB", "1D", 100)
	if err != nil {
		http.Error(w, "DXY candles unavailable", 500)
		return
	}
	type point struct {
		Time  int64   `json:"time"`
		Close float64 `json:"close"`
	}
	points := make([]point, 0, len(dxy))
	for i := len(dxy) - 1; i >= 0; i-- {
		points = append(points, point{dxy[i].OpenTime.Unix(), dxy[i].Close})
	}
	jsonOK(w, map[string]any{"bars": points, "correlation_20": forex.Correlation(pair, dxy, 20), "correlation_60": forex.Correlation(pair, dxy, 60)})
}

func (s *Server) getForexSessions(w http.ResponseWriter, r *http.Request) {
	if s.chartStore == nil {
		http.Error(w, "chart data unavailable", http.StatusServiceUnavailable)
		return
	}
	symbol := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("symbol")))
	tf := r.URL.Query().Get("tf")
	if symbol == "" || (tf != "1H" && tf != "4H" && tf != "1D" && tf != "1W") {
		http.Error(w, "symbol and valid tf required", 400)
		return
	}
	type item struct {
		Name      string   `json:"name"`
		Start     int64    `json:"start"`
		End       int64    `json:"end"`
		High      float64  `json:"high"`
		Low       float64  `json:"low"`
		AsianHigh *float64 `json:"asian_high,omitempty"`
		AsianLow  *float64 `json:"asian_low,omitempty"`
	}
	out := make([]item, 0)
	if tf == "1D" || tf == "1W" {
		jsonOK(w, map[string]any{"sessions": out})
		return
	}
	from, err1 := strconv.ParseInt(r.URL.Query().Get("from"), 10, 64)
	to, err2 := strconv.ParseInt(r.URL.Query().Get("to"), 10, 64)
	if err1 != nil || err2 != nil || to <= from || to-from > 180*86400 {
		http.Error(w, "invalid time range", 400)
		return
	}
	// Session boundaries can cut through a 4H candle; derive observed range
	// from 1H candles regardless of the requested intraday chart timeframe.
	bars, err := s.chartStore.GetOHLCV(symbol, "1H", 10000)
	if err != nil {
		http.Error(w, "candles unavailable", 500)
		return
	}
	ny, _ := time.LoadLocation("America/New_York")
	first := time.Unix(from, 0).In(ny).AddDate(0, 0, -1)
	last := time.Unix(to, 0).In(ny).AddDate(0, 0, 1)
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		ctx := market.AsianRange(bars, day)
		for _, session := range []struct {
			name   string
			window models.SessionWindow
		}{{"asia", ctx.Asia}, {"london", ctx.London}, {"new_york", ctx.NewYork}} {
			if session.window.Close.Unix() < from || session.window.Open.Unix() > to {
				continue
			}
			entry := item{Name: session.name, Start: session.window.Open.Unix(), End: session.window.Close.Unix()}
			has := false
			for _, b := range bars {
				if session.window.Contains(b.OpenTime) {
					if !has {
						entry.High = b.High
						entry.Low = b.Low
						has = true
					} else {
						if b.High > entry.High {
							entry.High = b.High
						}
						if b.Low < entry.Low {
							entry.Low = b.Low
						}
					}
				}
			}
			if !has {
				continue
			}
			if ctx.AsianRangeValid {
				hi, lo := ctx.AsianHigh, ctx.AsianLow
				entry.AsianHigh = &hi
				entry.AsianLow = &lo
			}
			out = append(out, entry)
		}
	}
	jsonOK(w, map[string]any{"sessions": out})
}
