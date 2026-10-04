package config

import (
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// InitializeDXY handles USD pairs already present in a YAML watchlist at boot.
// The persisted marker also preserves an intentional later DXY removal.
func InitializeDXY(path string, wl *WatchlistConfig) (bool, error) {
	if wl.DXYAutoInitialized {
		return false, nil
	}
	hasUSD := false
	for _, pair := range wl.Symbols.Forex {
		if pair.Enabled && strings.Contains(pair.Symbol, "USD") {
			hasUSD = true
			break
		}
	}
	if !hasUSD {
		return false, nil
	}
	wl.DXYAutoInitialized = true
	found := false
	for _, index := range wl.Symbols.Indices {
		if index.Symbol == "DX-Y.NYB" {
			found = true
			break
		}
	}
	if !found {
		wl.Symbols.Indices = append(wl.Symbols.Indices, SymbolEntry{Symbol: "DX-Y.NYB", Exchange: "nyb", Enabled: true})
	}
	data, err := yaml.Marshal(wl)
	if err != nil {
		return false, err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return false, err
	}
	return true, nil
}
