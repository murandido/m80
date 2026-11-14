package businessday

import (
	"testing"
	"time"
)

func TestOfReadsTheSaoPauloCalendarWhateverTheProcessZone(t *testing.T) {
	cases := []struct {
		name   string
		moment time.Time
		want   string
	}{
		// 23:30 in São Paulo on the last day of the month is 02:30 UTC of the next month.
		{"late evening, last day of the month", time.Date(2026, 11, 1, 2, 30, 0, 0, time.UTC), "2026-10-31"},
		{"last second before São Paulo midnight", time.Date(2026, 11, 1, 2, 59, 59, 0, time.UTC), "2026-10-31"},
		{"São Paulo midnight", time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC), "2026-11-01"},
		{"same instant carried in another zone", time.Date(2026, 10, 31, 23, 30, 0, 0, Location), "2026-10-31"},
		{"UTC midnight is still the previous day", time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC), "2026-09-03"},
	}
	for _, c := range cases {
		got := Of(c.moment)
		if got.Format(time.DateOnly) != c.want || got.Location() != time.UTC || got.Hour() != 0 {
			t.Errorf("%s: Of(%s) = %s, want %s at midnight UTC", c.name, c.moment, got, c.want)
		}
	}
}

func TestStartIsSaoPauloMidnightOfTheDate(t *testing.T) {
	date := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC) // as a DATE column scans
	got := Start(date)
	if want := time.Date(2026, 10, 31, 3, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Start(%s) = %s, want %s", date, got.UTC(), want)
	}
}
