package pipeline

import (
	appconfig "github.com/Ju571nK/Chatter/internal/config"
	"github.com/Ju571nK/Chatter/pkg/models"
)

// filterByProfile removes signals that are not allowed by the symbol's profile.
// If the holder is nil or the profile has no restrictions, all signals pass through.
func filterByProfile(signals []models.Signal, holder *appconfig.SymbolProfilesHolder, symbol string) []models.Signal {
	if holder == nil {
		return signals
	}
	profile := holder.GetProfile(symbol)

	// Fast path: if profile has no restrictions, allow everything.
	if len(profile.AllowedMethodologies) == 0 &&
		len(profile.BlockedMethodologies) == 0 &&
		len(profile.AllowedRules) == 0 {
		return signals
	}

	out := make([]models.Signal, 0, len(signals))
	for _, sig := range signals {
		if profile.IsSignalAllowed(sig.Rule) {
			out = append(out, sig)
		}
	}
	return out
}

// profileScoreThreshold returns the profile's score threshold for a symbol.
// Returns 0 if the holder is nil or threshold is not set (caller should use default).
func profileScoreThreshold(holder *appconfig.SymbolProfilesHolder, symbol string) float64 {
	if holder == nil {
		return 0
	}
	return holder.GetProfile(symbol).ScoreThreshold
}

// profileCooldownHours returns the profile's cooldown hours for a symbol.
// Returns 0 if the holder is nil or not set (caller should use default).
func profileCooldownHours(holder *appconfig.SymbolProfilesHolder, symbol string) int {
	if holder == nil {
		return 0
	}
	return holder.GetProfile(symbol).CooldownHours
}
