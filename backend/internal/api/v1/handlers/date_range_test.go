package handlers

import (
	"testing"
	"time"
)

// The instant that used to name the file wrong: still the 5th in São Paulo, already the 6th
// in UTC, which is where the container's clock lives.
func TestExportFilenameDateUsesTheLocalCalendarDay(t *testing.T) {
	got := exportFilenameDate(time.Date(2026, 9, 6, 2, 0, 0, 0, time.UTC))
	if want := "2026-09-05"; got != want {
		t.Fatalf("exportFilenameDate = %q, want %q", got, want)
	}
}
