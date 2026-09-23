package models

import "time"

// HTFOpportunityVersion identifies the collection semantics. Version 1 keeps
// the first live observation per rule/direction/bar, not a closed-bar replay.
const HTFOpportunityVersion = 1

// HTFOpportunity is a rule-engine output before profile, score, MTF and HTF
// filtering. Score includes rule/timeframe weights, but no pipeline adjustments.
// It is an observation, not a completed trade or calibration-ready sample.
type HTFOpportunity struct {
	Version           int
	Signal            Signal
	BarOpenTime       time.Time
	ObservedAt        time.Time
	RawHTFTrend       string
	EffectiveHTFTrend string
	ATRPercentile     float64
	Snapshot          string // JSON of indicators, latest bars and Wyckoff phase
}
