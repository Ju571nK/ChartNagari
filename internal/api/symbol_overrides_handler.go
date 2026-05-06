package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/internal/storage"
)

// symbolOverrideRequest is the wire format for PUT.
// All fields are pointers to distinguish "not in body" from "explicit null".
// In Go's encoding/json, a JSON null decodes to a nil pointer; a missing key
// also leaves a nil pointer. We accept both as "no override / inherit".
type symbolOverrideRequest struct {
	ScoreThreshold   *float64  `json:"score_threshold"`
	CooldownHours    *int      `json:"cooldown_hours"`
	AlertLimitPerDay *int      `json:"alert_limit_per_day"`
	Timeframes       *[]string `json:"timeframes"`
	AllowedRules     *[]string `json:"allowed_rules"`
}

// fieldSource is the wire format for each field in the merged response.
type fieldSource struct {
	Value  any    `json:"value"`
	Source string `json:"source"` // "override" | "profile"
}

// effectiveResponse is the body returned by PUT and DELETE.
type effectiveResponse struct {
	Symbol           string      `json:"symbol"`
	ScoreThreshold   fieldSource `json:"score_threshold"`
	CooldownHours    fieldSource `json:"cooldown_hours"`
	AlertLimitPerDay fieldSource `json:"alert_limit_per_day"`
	Timeframes       fieldSource `json:"timeframes"`
	AllowedRules     fieldSource `json:"allowed_rules"`
}

// validTimeframes is the closed set of accepted timeframe values.
var validTimeframes = map[string]struct{}{
	"1H": {}, "4H": {}, "1D": {}, "1W": {},
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// getSymbolOverride handles GET /api/symbol-overrides/{symbol}.
func (s *Server) getSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}
	jsonOK(w, s.buildEffectiveResponse(symbol))
}

// putSymbolOverride handles PUT /api/symbol-overrides/{symbol}.
func (s *Server) putSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req symbolOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	ov, err := s.requestToOverride(symbol, req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if ov.IsEmpty() {
		if err := s.overrideStore.Delete(symbol); err != nil {
			writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("delete override: %v", err))
			return
		}
	} else {
		if err := s.overrideStore.Put(ov); err != nil {
			writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("put override: %v", err))
			return
		}
	}

	jsonOK(w, s.buildEffectiveResponse(symbol))
}

// deleteSymbolOverride handles DELETE /api/symbol-overrides/{symbol}.
func (s *Server) deleteSymbolOverride(w http.ResponseWriter, r *http.Request) {
	if !s.requireBearer(w, r) {
		return
	}
	symbol := r.PathValue("symbol")
	if symbol == "" {
		writeAPIError(w, http.StatusBadRequest, "symbol path parameter required")
		return
	}
	if err := s.overrideStore.Delete(symbol); err != nil {
		writeAPIError(w, http.StatusInternalServerError, fmt.Sprintf("delete override: %v", err))
		return
	}
	jsonOK(w, s.buildEffectiveResponse(symbol))
}

// requestToOverride validates and converts the wire request into a SymbolOverride.
// Empty arrays in the request are normalized to nil (== inherit) before storage.
func (s *Server) requestToOverride(symbol string, req symbolOverrideRequest) (storage.SymbolOverride, error) {
	out := storage.SymbolOverride{Symbol: symbol}

	if req.ScoreThreshold != nil {
		v := *req.ScoreThreshold
		if v < 0 || v > 50 {
			return out, errors.New("invalid score_threshold: must be 0~50")
		}
		out.ScoreThreshold = &v
	}
	if req.CooldownHours != nil {
		v := *req.CooldownHours
		if v < 0 || v > 168 {
			return out, errors.New("invalid cooldown_hours: must be 0~168")
		}
		out.CooldownHours = &v
	}
	if req.AlertLimitPerDay != nil {
		v := *req.AlertLimitPerDay
		if v < 0 || v > 100 {
			return out, errors.New("invalid alert_limit_per_day: must be 0~100")
		}
		out.AlertLimitPerDay = &v
	}
	if req.Timeframes != nil && len(*req.Timeframes) > 0 {
		seen := make(map[string]struct{}, len(*req.Timeframes))
		for _, tf := range *req.Timeframes {
			if _, ok := validTimeframes[tf]; !ok {
				return out, fmt.Errorf("invalid timeframe: %q (must be one of 1H,4H,1D,1W)", tf)
			}
			if _, dup := seen[tf]; dup {
				return out, fmt.Errorf("duplicate timeframe: %q", tf)
			}
			seen[tf] = struct{}{}
		}
		out.Timeframes = *req.Timeframes
	}
	if req.AllowedRules != nil && len(*req.AllowedRules) > 0 {
		if s.validRuleNames != nil {
			for _, r := range *req.AllowedRules {
				if _, ok := s.validRuleNames[r]; !ok {
					return out, fmt.Errorf("unknown rule: %q", r)
				}
			}
		}
		out.AllowedRules = *req.AllowedRules
	}
	return out, nil
}

// buildEffectiveResponse computes the merged effective config and emits it
// with per-field provenance ("override" | "profile").
func (s *Server) buildEffectiveResponse(symbol string) effectiveResponse {
	cfg := appconfig.EffectiveAlertConfig(symbol, s.profileHolder, s.overrideStore)
	ov, _ := s.overrideStore.Get(symbol)

	src := func(overrideHasIt bool) string {
		if overrideHasIt {
			return "override"
		}
		return "profile"
	}

	hasScore := ov != nil && ov.ScoreThreshold != nil
	hasCooldown := ov != nil && ov.CooldownHours != nil
	hasLimit := ov != nil && ov.AlertLimitPerDay != nil
	hasTF := ov != nil && ov.Timeframes != nil
	hasRules := ov != nil && ov.AllowedRules != nil

	return effectiveResponse{
		Symbol:           symbol,
		ScoreThreshold:   fieldSource{Value: cfg.ScoreThreshold, Source: src(hasScore)},
		CooldownHours:    fieldSource{Value: cfg.CooldownHours, Source: src(hasCooldown)},
		AlertLimitPerDay: fieldSource{Value: cfg.AlertLimitPerDay, Source: src(hasLimit)},
		Timeframes:       fieldSource{Value: cfg.Timeframes, Source: src(hasTF)},
		AllowedRules:     fieldSource{Value: cfg.AllowedRules, Source: src(hasRules)},
	}
}
