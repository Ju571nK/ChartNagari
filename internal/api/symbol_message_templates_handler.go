package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Ju571nK/Chatter/internal/notifier"
	"github.com/Ju571nK/Chatter/internal/storage"
	"github.com/Ju571nK/Chatter/pkg/models"
)

type messageTemplatesRequest struct {
	Long  *string `json:"long"`
	Short *string `json:"short"`
}

type messageTemplatePreviewRequest struct {
	Direction string  `json:"direction"`
	Template  *string `json:"template"`
}

func decodeTemplateRequest(w http.ResponseWriter, r *http.Request, out any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body: expected one object")
		return false
	}
	return true
}

func (s *Server) getSymbolMessageTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol required")
		return
	}
	value, err := s.messageTemplateStore.Get(symbol)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, value)
}

func (s *Server) putSymbolMessageTemplates(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol required")
		return
	}
	var req messageTemplatesRequest
	if !decodeTemplateRequest(w, r, &req) {
		return
	}
	if req.Long == nil || req.Short == nil {
		writeAPIError(w, http.StatusBadRequest, "long and short strings are required")
		return
	}
	if err := notifier.ValidateTelegramTemplate(*req.Long); err != nil {
		writeAPIError(w, http.StatusBadRequest, "long: "+err.Error())
		return
	}
	if err := notifier.ValidateTelegramTemplate(*req.Short); err != nil {
		writeAPIError(w, http.StatusBadRequest, "short: "+err.Error())
		return
	}
	value := storage.SymbolMessageTemplates{Symbol: symbol, Long: *req.Long, Short: *req.Short}
	if err := s.messageTemplateStore.Put(value); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err.Error())
		return
	}
	jsonOK(w, value)
}

func (s *Server) previewSymbolMessageTemplate(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol required")
		return
	}
	var req messageTemplatePreviewRequest
	if !decodeTemplateRequest(w, r, &req) {
		return
	}
	if req.Direction != "LONG" && req.Direction != "SHORT" {
		writeAPIError(w, http.StatusBadRequest, "direction must be LONG or SHORT")
		return
	}
	if req.Template == nil {
		writeAPIError(w, http.StatusBadRequest, "template string is required")
		return
	}
	if err := notifier.ValidateTelegramTemplate(*req.Template); err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	sig := models.Signal{
		Symbol: symbol, Timeframe: "1H", Direction: req.Direction,
		Rule: "sample_rule", Score: 12.5, Message: "Sample setup",
		EntryPrice: 100, TP: 105, SL: 98,
		CreatedAt:  time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC),
		AssetClass: models.AssetStock,
	}
	if wl, err := s.readWatchlist(); err == nil {
		for _, item := range wl.Symbols.Forex {
			if item.Symbol == symbol {
				sig.AssetClass = models.AssetForex
				switch {
				case symbol == "JPYUSD":
					sig.EntryPrice, sig.TP, sig.SL = 0.0067, 0.0068, 0.0066
				case strings.HasSuffix(symbol, "JPY"):
					sig.EntryPrice, sig.TP, sig.SL = 150, 151, 149
				case symbol == "XAUUSD":
					sig.EntryPrice, sig.TP, sig.SL = 2000, 2010, 1990
				case symbol == "XAGUSD":
					sig.EntryPrice, sig.TP, sig.SL = 25, 26, 24
				default:
					sig.EntryPrice, sig.TP, sig.SL = 1.1, 1.11, 1.09
				}
				break
			}
		}
	}
	if sig.Direction == "SHORT" {
		tpDistance := sig.TP - sig.EntryPrice
		slDistance := sig.EntryPrice - sig.SL
		sig.TP, sig.SL = sig.EntryPrice-tpDistance, sig.EntryPrice+slDistance
	}
	message, included := notifier.RenderTelegramAlertWithStatus(sig, *req.Template)
	jsonOK(w, map[string]any{
		"html": message, "custom_included": included,
		"sample": notifier.TelegramSampleSummary(sig),
	})
}
