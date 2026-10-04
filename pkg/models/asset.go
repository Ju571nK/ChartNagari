package models

// AssetClass identifies the market whose hours and trading conventions apply.
type AssetClass string

const (
	AssetCrypto AssetClass = "crypto"
	AssetStock  AssetClass = "stock"
	AssetIndex  AssetClass = "index"
	AssetForex  AssetClass = "forex"
)

// VolumeQuality describes whether bar volume counts trades, price updates, or
// is unavailable. An empty value preserves the pre-FX real-volume behaviour.
type VolumeQuality string

const (
	VolumeReal VolumeQuality = "real"
	VolumeTick VolumeQuality = "tick"
	VolumeNone VolumeQuality = "none"
)
