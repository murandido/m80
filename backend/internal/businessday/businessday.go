// Package businessday is the one place the M80 decides which calendar day an instant
// belongs to.
//
// Timestamps are stored as instants (timestamptz), but the business runs on São Paulo's
// calendar: a booking made at 22:30 on the 31st belongs to the 31st, to that month's costing
// and to that month's closing. The process zone cannot be trusted for this - the production
// container runs in UTC and a developer's machine in BRT - so anything that turns an
// instant into a day asks this package instead of reading moment.Year()/Month()/Day().
package businessday

import (
	"time"

	// Embedded so the zone resolves even on an image without /usr/share/zoneinfo: a silent
	// fallback to UTC would be exactly the bug this package exists to prevent.
	_ "time/tzdata"
)

// Timezone is the business calendar's zone, by name, for the places that need the string
// (SQL's AT TIME ZONE, time.LoadLocation callers).
const Timezone = "America/Sao_Paulo"

// Location is Timezone loaded. Brazil has kept a fixed UTC-3 since 2019, so the fallback
// (unreachable with tzdata embedded) is that offset rather than UTC.
var Location = func() *time.Location {
	loc, err := time.LoadLocation(Timezone)
	if err != nil {
		return time.FixedZone("BRT", -3*60*60)
	}
	return loc
}()

// Of is the São Paulo calendar day of an instant, as a midnight-UTC date: the same shape a
// DATE column comes back from the database in, so the result compares directly against
// ends_on, occurred_on and friends.
//
// Only for instants. A value that already is a date (a DATE column, midnight UTC) must not
// go through here: read in São Paulo it is 21:00 of the day before.
func Of(moment time.Time) time.Time {
	local := moment.In(Location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

// Start is the instant a São Paulo calendar day begins. The day's year, month and day are
// read as they are, whatever location the value carries, so it accepts a DATE column.
func Start(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, Location)
}
