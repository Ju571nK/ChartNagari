package calendar

import "time"

// CollectionStatus is process-local; restart clears its history. EventCount is
// the last successful batch after US/date filtering, not the visible date range.
type CollectionStatus struct {
	Provider    string    `json:"provider"`
	State       string    `json:"state"`
	Failure     string    `json:"failure,omitempty"`
	LastAttempt time.Time `json:"last_attempt"`
	LastSuccess time.Time `json:"last_success"`
	EventCount  int       `json:"event_count"`
}

func (f *Fetcher) Status() CollectionStatus {
	f.mu.RLock()
	defer f.mu.RUnlock()
	s := f.status
	s.Provider = "none"
	if f.fmpKey != "" {
		s.Provider = "FMP"
	} else if f.finnhubKey != "" {
		s.Provider = "Finnhub"
	}
	if s.State == "" {
		if s.Provider == "none" {
			s.State = "disabled"
		} else {
			s.State = "waiting"
		}
	}
	return s
}

type collectionError string

func (e collectionError) Error() string { return string(e) }

func httpFailure(status int) error {
	switch status {
	case 401:
		return collectionError("authentication")
	case 402, 403:
		return collectionError("permission")
	case 429:
		return collectionError("rate_limit")
	default:
		return collectionError("provider")
	}
}
