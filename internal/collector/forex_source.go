package collector

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Ju571nK/Chatter/internal/config"
)

// ForexSource describes the exact feed used for a canonical FX symbol.
// Environment and provider mapping are part of the identity: changing either
// invalidates candles and prevents old paper positions from using new prices.
type ForexSource struct {
	Provider       string `json:"data_provider"`
	ProviderSymbol string `json:"provider_symbol"`
	Identity       string `json:"source_identity"`
	Proxy          bool   `json:"data_proxy"`
}

func SourceForForex(entry config.SymbolEntry, provider, environment string, token ...string) ForexSource {
	symbol := strings.ToUpper(entry.Symbol)
	mapped := entry.ProviderSymbol
	proxy := false
	if provider == "oanda" {
		if mapped == "" {
			if len(symbol) == 6 {
				mapped = symbol[:3] + "_" + symbol[3:]
			} else {
				mapped = symbol
			}
		}
		if environment != "live" {
			environment = "practice"
		}
	} else {
		provider = "yahoo"
		if mapped == "" {
			mapped, proxy = YahooFXSymbol(symbol)
		}
		proxy = proxy || symbol == "XAUUSD" || symbol == "XAGUSD"
		environment = "public"
	}
	identity := provider + ":" + environment + ":" + mapped
	if provider == "oanda" && len(token) > 0 && token[0] != "" {
		digest := sha256.Sum256([]byte(token[0]))
		identity += ":sha256=" + hex.EncodeToString(digest[:])
	}
	return ForexSource{Provider: provider, ProviderSymbol: mapped, Identity: identity, Proxy: proxy}
}
