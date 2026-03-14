// Package market provides NYSE trading hours checks.
package market

import (
	"time"
)

// nyseHolidays lists NYSE observed holidays by yyyy-mm-dd.
var nyseHolidays = map[string]bool{
	// 2026
	"2026-01-01": true, // New Year's Day
	"2026-01-19": true, // MLK Day
	"2026-02-16": true, // Presidents Day
	"2026-04-03": true, // Good Friday
	"2026-05-25": true, // Memorial Day
	"2026-06-19": true, // Juneteenth
	"2026-07-03": true, // Independence Day observed (Jul 4 = Sat)
	"2026-09-07": true, // Labor Day
	"2026-11-26": true, // Thanksgiving
	"2026-12-25": true, // Christmas
	// 2027
	"2027-01-01": true, // New Year's Day
	"2027-01-18": true, // MLK Day
	"2027-02-15": true, // Presidents Day
	"2027-03-26": true, // Good Friday
	"2027-05-31": true, // Memorial Day
	"2027-06-18": true, // Juneteenth observed (Jun 19 = Sat)
	"2027-07-05": true, // Independence Day observed (Jul 4 = Sun)
	"2027-09-06": true, // Labor Day
	"2027-11-25": true, // Thanksgiving
	"2027-12-24": true, // Christmas observed (Dec 25 = Sat)
}

var nyLoc *time.Location

func init() {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		loc = time.FixedZone("EST", -5*60*60)
	}
	nyLoc = loc
}

// IsUSMarketOpen reports whether t falls within NYSE regular session:
// Mon–Fri 09:30–16:00 America/New_York, excluding NYSE holidays.
func IsUSMarketOpen(t time.Time) bool {
	ny := t.In(nyLoc)

	wd := ny.Weekday()
	if wd == time.Saturday || wd == time.Sunday {
		return false
	}

	if nyseHolidays[ny.Format("2006-01-02")] {
		return false
	}

	minutes := ny.Hour()*60 + ny.Minute()
	return minutes >= 9*60+30 && minutes < 16*60
}
