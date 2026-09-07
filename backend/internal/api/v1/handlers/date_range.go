package handlers

import (
	"net/http"
	"time"
)

// listTimezone is the timezone used to interpret the date_from/date_to filters of every
// listing endpoint. Timestamps are stored in UTC, but operators think in local (Brazil)
// calendar days, so "today" must be resolved against this zone, not UTC.
const listTimezone = "America/Sao_Paulo"

// exportFilenameDate resolves "today" for a download name in the operator's calendar day.
// time.Now() alone is UTC inside the container, so a file exported at 22h in São Paulo came
// down named with tomorrow's date - the sheet's own contents were already local.
func exportFilenameDate(now time.Time) string {
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}
	return now.In(loc).Format("2006-01-02")
}

// parseDateRange reads the date_from/date_to query params (both optional, YYYY-MM-DD) and
// resolves them into UTC bounds for a half-open [from, to) interval: date_from is the
// start of that local day, date_to the start of the day *after* it, so filtering with
// `>= from AND < to` includes the whole "to" day.
//
// A nil return on either side means that end is unbounded. Unparseable values are treated
// as absent rather than as an error: a listing with a typo'd date degrades to a wider
// result set instead of a 400, which is what every caller wants here.
//
// Shared by the issuance and sales listings, and by whatever period-based aggregation
// comes next - the calendar-day semantics only need to be right in one place.
func parseDateRange(r *http.Request) (from, to *time.Time) {
	q := r.URL.Query()

	// Fall back to UTC if tzdata is missing rather than dropping the filter entirely.
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		loc = time.UTC
	}

	if v := q.Get("date_from"); v != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			t := parsed.UTC()
			from = &t
		}
	}
	if v := q.Get("date_to"); v != "" {
		if parsed, err := time.ParseInLocation("2006-01-02", v, loc); err == nil {
			t := parsed.AddDate(0, 0, 1).UTC()
			to = &t
		}
	}

	return from, to
}

// inRange reports whether an instant falls inside the half-open interval produced by
// parseDateRange. A nil bound is unbounded on that side; a nil instant is never in range
// (a never-cancelled issuance has no cancelled_at to compare against).
func inRange(instant *time.Time, from, to *time.Time) bool {
	if instant == nil {
		return false
	}
	if from != nil && instant.Before(*from) {
		return false
	}
	if to != nil && !instant.Before(*to) {
		return false
	}
	return true
}
