package handlers

import (
	"testing"
	"time"
)

// aggregateDashboard is a pure fold over rows the SQL layer already narrowed, so these
// tests need no database - which is the reason the period rule lives in Go at all.

func spLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(listTimezone)
	if err != nil {
		t.Skipf("skipping dashboard tests: tzdata unavailable: %v", err)
	}
	return loc
}

// spInstant builds a UTC instant from a São Paulo wall clock, the way rows come back from
// the database (stored UTC, meaningful in local time).
func spInstant(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04", value, spLocation(t))
	if err != nil {
		t.Fatalf("bad fixture instant %q: %v", value, err)
	}
	return parsed.UTC()
}

// spBounds mirrors parseDateRange: date_from is the start of that local day, date_to the
// start of the day after, making the interval half-open.
func spBounds(t *testing.T, fromYmd, toYmd string) (*time.Time, *time.Time) {
	t.Helper()
	loc := spLocation(t)

	fromLocal, err := time.ParseInLocation("2006-01-02", fromYmd, loc)
	if err != nil {
		t.Fatalf("bad fixture date %q: %v", fromYmd, err)
	}
	toLocal, err := time.ParseInLocation("2006-01-02", toYmd, loc)
	if err != nil {
		t.Fatalf("bad fixture date %q: %v", toYmd, err)
	}

	from := fromLocal.UTC()
	to := toLocal.AddDate(0, 0, 1).UTC()
	return &from, &to
}

func dashboardRow(created time.Time, points, valueOfTheThousand int) dashboardIssuanceRow {
	return dashboardIssuanceRow{
		Points:              points,
		ValueOfTheThousand:  valueOfTheThousand,
		CreatedAt:           created,
		LoyaltyProgramID:    "program-a",
		LoyaltyProgramName:  "Smiles",
		LoyaltyProgramColor: "#7C3AED",
		IssuerID:            "issuer-a",
		IssuerName:          "Ana",
	}
}

func amountOrFail(t *testing.T, value *int, what string) int {
	t.Helper()
	if value == nil {
		t.Fatalf("expected %s to carry an amount, got nil", what)
	}
	return *value
}

func TestAggregateDashboard_CountsCreationAndCancellationIndependently(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	cancelledAt := spInstant(t, "2026-07-03 09:00")
	refunded := 300

	rows := []dashboardIssuanceRow{
		dashboardRow(spInstant(t, "2026-07-01 10:00"), 1000, 2000),
		dashboardRow(spInstant(t, "2026-07-02 10:00"), 500, 2000),
		func() dashboardIssuanceRow {
			row := dashboardRow(spInstant(t, "2026-07-01 11:00"), 200, 1000)
			row.CancelledAt = &cancelledAt
			row.RefundedPoints = &refunded
			return row
		}(),
	}

	got := aggregateDashboard(rows, from, to, "day", loc, true)

	if got.Totals.IssuanceCount != 3 {
		t.Errorf("issuance_count = %d, want 3", got.Totals.IssuanceCount)
	}
	if got.Totals.Points != 1700 {
		t.Errorf("points = %d, want 1700", got.Totals.Points)
	}
	// 1000 miles @ 2000/mil = 2000c, 500 @ 2000 = 1000c, 200 @ 1000 = 200c.
	if amount := amountOrFail(t, got.Totals.AmountCents, "totals"); amount != 3200 {
		t.Errorf("amount_cents = %d, want 3200", amount)
	}

	// The cancelled issuance was created inside the window too, so it counts on both
	// sides rather than being netted out of the volume.
	if got.Totals.CancelledCount != 1 {
		t.Errorf("cancelled_count = %d, want 1", got.Totals.CancelledCount)
	}
	if got.Totals.RefundedPoints != 300 {
		t.Errorf("refunded_points = %d, want 300", got.Totals.RefundedPoints)
	}
}

func TestAggregateDashboard_CountsCancellationFromOutsideTheWindow(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	// Created well before the window, cancelled inside it: the cancellation belongs to
	// this period, the volume does not.
	cancelledAt := spInstant(t, "2026-07-02 09:00")
	refunded := 900
	row := dashboardRow(spInstant(t, "2026-05-10 10:00"), 5000, 2000)
	row.CancelledAt = &cancelledAt
	row.RefundedPoints = &refunded

	got := aggregateDashboard([]dashboardIssuanceRow{row}, from, to, "day", loc, true)

	if got.Totals.IssuanceCount != 0 {
		t.Errorf("issuance_count = %d, want 0", got.Totals.IssuanceCount)
	}
	if got.Totals.Points != 0 {
		t.Errorf("points = %d, want 0", got.Totals.Points)
	}
	if got.Totals.CancelledCount != 1 {
		t.Errorf("cancelled_count = %d, want 1", got.Totals.CancelledCount)
	}
	if got.Totals.RefundedPoints != 900 {
		t.Errorf("refunded_points = %d, want 900", got.Totals.RefundedPoints)
	}
}

func TestAggregateDashboard_PreviousTotalsUseTheWindowShiftedBack(t *testing.T) {
	loc := spLocation(t)
	// A three-day window, so the previous period is 28-30 June.
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	rows := []dashboardIssuanceRow{
		dashboardRow(spInstant(t, "2026-07-02 10:00"), 1000, 2000),
		dashboardRow(spInstant(t, "2026-06-29 10:00"), 400, 2000),
		// Older than the previous window - counts nowhere.
		dashboardRow(spInstant(t, "2026-06-20 10:00"), 9999, 2000),
	}

	got := aggregateDashboard(rows, from, to, "day", loc, true)

	if got.Totals.Points != 1000 {
		t.Errorf("totals points = %d, want 1000", got.Totals.Points)
	}
	if got.PreviousTotals == nil {
		t.Fatal("expected previous_totals to be present for a bounded window")
	}
	if got.PreviousTotals.Points != 400 {
		t.Errorf("previous points = %d, want 400", got.PreviousTotals.Points)
	}
	if got.PreviousTotals.IssuanceCount != 1 {
		t.Errorf("previous issuance_count = %d, want 1", got.PreviousTotals.IssuanceCount)
	}
}

func TestAggregateDashboard_OmitsPreviousTotalsWhenUnbounded(t *testing.T) {
	loc := spLocation(t)
	rows := []dashboardIssuanceRow{dashboardRow(spInstant(t, "2026-07-02 10:00"), 1000, 2000)}

	got := aggregateDashboard(rows, nil, nil, "day", loc, true)

	if got.PreviousTotals != nil {
		t.Error("expected previous_totals to be nil without both bounds")
	}
}

func TestAggregateDashboard_FillsEmptyBuckets(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-05")

	rows := []dashboardIssuanceRow{
		dashboardRow(spInstant(t, "2026-07-01 10:00"), 1000, 2000),
		dashboardRow(spInstant(t, "2026-07-05 10:00"), 500, 2000),
	}

	got := aggregateDashboard(rows, from, to, "day", loc, true)

	want := []string{"2026-07-01", "2026-07-02", "2026-07-03", "2026-07-04", "2026-07-05"}
	if len(got.Series) != len(want) {
		t.Fatalf("series has %d buckets, want %d: %+v", len(got.Series), len(want), got.Series)
	}
	for i, key := range want {
		if got.Series[i].Bucket != key {
			t.Errorf("series[%d].bucket = %q, want %q", i, got.Series[i].Bucket, key)
		}
	}

	if got.Series[0].Points != 1000 {
		t.Errorf("series[0].points = %d, want 1000", got.Series[0].Points)
	}
	if got.Series[2].Points != 0 {
		t.Errorf("series[2].points = %d, want 0 for a quiet day", got.Series[2].Points)
	}
	if got.Series[4].Points != 500 {
		t.Errorf("series[4].points = %d, want 500", got.Series[4].Points)
	}
}

func TestAggregateDashboard_OmitsAmountsForNonAdmin(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	rows := []dashboardIssuanceRow{
		dashboardRow(spInstant(t, "2026-07-01 10:00"), 1000, 2000),
		dashboardRow(spInstant(t, "2026-06-29 10:00"), 400, 2000),
	}

	got := aggregateDashboard(rows, from, to, "day", loc, false)

	if got.Totals.AmountCents != nil {
		t.Error("totals.amount_cents leaked to a non-admin")
	}
	if got.PreviousTotals != nil && got.PreviousTotals.AmountCents != nil {
		t.Error("previous_totals.amount_cents leaked to a non-admin")
	}
	for i, bucket := range got.Series {
		if bucket.AmountCents != nil {
			t.Errorf("series[%d].amount_cents leaked to a non-admin", i)
		}
	}
	for i, slice := range got.ByProgram {
		if slice.AmountCents != nil {
			t.Errorf("by_program[%d].amount_cents leaked to a non-admin", i)
		}
	}
	for i, slice := range got.ByIssuer {
		if slice.AmountCents != nil {
			t.Errorf("by_issuer[%d].amount_cents leaked to a non-admin", i)
		}
	}

	// The miles are still there - only the money is cut.
	if got.Totals.Points != 1000 {
		t.Errorf("points = %d, want 1000 for a non-admin", got.Totals.Points)
	}
}

func TestAggregateDashboard_AlwaysCarriesAmountForAdmin(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	// Nothing at all in either window. The amounts still have to be present-and-zero for
	// an admin: absent is the signal for "withheld", and overloading it with "quiet
	// period" would make the frontend drop the delta on every stat tile.
	got := aggregateDashboard(nil, from, to, "day", loc, true)

	if amount := amountOrFail(t, got.Totals.AmountCents, "totals"); amount != 0 {
		t.Errorf("totals amount_cents = %d, want 0", amount)
	}
	if got.PreviousTotals == nil {
		t.Fatal("expected previous_totals for a bounded window")
	}
	if amount := amountOrFail(t, got.PreviousTotals.AmountCents, "previous_totals"); amount != 0 {
		t.Errorf("previous amount_cents = %d, want 0", amount)
	}
}

func TestAggregateDashboard_RanksBreakdownsByPoints(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-03")

	small := dashboardRow(spInstant(t, "2026-07-01 10:00"), 100, 2000)
	small.LoyaltyProgramID = "program-b"
	small.LoyaltyProgramName = "TudoAzul"
	small.IssuerID = "issuer-b"
	small.IssuerName = "Bruno"

	big := dashboardRow(spInstant(t, "2026-07-02 10:00"), 900, 2000)

	got := aggregateDashboard([]dashboardIssuanceRow{small, big}, from, to, "day", loc, true)

	if len(got.ByProgram) != 2 {
		t.Fatalf("by_program has %d slices, want 2", len(got.ByProgram))
	}
	if got.ByProgram[0].Name != "Smiles" {
		t.Errorf("by_program[0] = %q, want the biggest (Smiles)", got.ByProgram[0].Name)
	}
	if got.ByProgram[0].Color != "#7C3AED" {
		t.Errorf("by_program[0].color = %q, want the program's own hex", got.ByProgram[0].Color)
	}
	if got.ByIssuer[0].Name != "Ana" {
		t.Errorf("by_issuer[0] = %q, want the biggest (Ana)", got.ByIssuer[0].Name)
	}
	// Issuers have no colour of their own; the field must stay empty rather than inherit
	// the program's.
	if got.ByIssuer[0].Color != "" {
		t.Errorf("by_issuer[0].color = %q, want empty", got.ByIssuer[0].Color)
	}
}

func TestAggregateDashboard_AmountIncludesTaxes(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-01")

	taxes := 4550
	row := dashboardRow(spInstant(t, "2026-07-01 10:00"), 1000, 2000)
	row.Taxes = &taxes

	got := aggregateDashboard([]dashboardIssuanceRow{row}, from, to, "day", loc, true)

	if amount := amountOrFail(t, got.Totals.AmountCents, "totals"); amount != 6550 {
		t.Errorf("amount_cents = %d, want 6550 (2000 + 4550 taxes)", amount)
	}
}

func TestAggregateDashboard_BucketsByLocalDayNotUTC(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-02")

	// 22:00 in São Paulo is already 01:00 the next day in UTC. Bucketing on the raw UTC
	// timestamp would file this under the 2nd and shift a whole evening's work into the
	// wrong day.
	row := dashboardRow(spInstant(t, "2026-07-01 22:00"), 1000, 2000)

	got := aggregateDashboard([]dashboardIssuanceRow{row}, from, to, "day", loc, true)

	if len(got.Series) != 2 {
		t.Fatalf("series has %d buckets, want 2", len(got.Series))
	}
	if got.Series[0].Points != 1000 {
		t.Errorf("2026-07-01 points = %d, want 1000", got.Series[0].Points)
	}
	if got.Series[1].Points != 0 {
		t.Errorf("2026-07-02 points = %d, want 0", got.Series[1].Points)
	}
}

func TestDashboardBucketKey_Granularities(t *testing.T) {
	loc := spLocation(t)

	tests := []struct {
		name        string
		instant     string
		granularity string
		want        string
	}{
		{"hour truncates minutes", "2026-07-01 14:37", "hour", "2026-07-01T14:00"},
		{"day drops the clock", "2026-07-01 14:37", "day", "2026-07-01"},
		{"month keeps year and month", "2026-07-17 14:37", "month", "2026-07"},
		// Weeks start on Monday: a Wednesday and the Sunday that closes the same week
		// both belong to Monday the 29th of June.
		{"week from a Wednesday", "2026-07-01 14:37", "week", "2026-06-29"},
		{"week from a Sunday", "2026-07-05 23:59", "week", "2026-06-29"},
		{"week from a Monday", "2026-07-06 00:00", "week", "2026-07-06"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := dashboardBucketKey(spInstant(t, tc.instant), tc.granularity, loc)
			if got != tc.want {
				t.Errorf("dashboardBucketKey(%s, %s) = %q, want %q", tc.instant, tc.granularity, got, tc.want)
			}
		})
	}
}

func TestAggregateDashboard_MonthlyBucketsWalkTheCalendar(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-01-01", "2026-12-31")

	rows := []dashboardIssuanceRow{
		dashboardRow(spInstant(t, "2026-02-14 10:00"), 1000, 2000),
		dashboardRow(spInstant(t, "2026-11-30 10:00"), 500, 2000),
	}

	got := aggregateDashboard(rows, from, to, "month", loc, true)

	// Twelve months, not 365 days - stepping by AddDate(0,1,0) rather than a fixed
	// duration is what keeps February from dragging the rest of the year out of phase.
	if len(got.Series) != 12 {
		t.Fatalf("series has %d buckets, want 12: %+v", len(got.Series), got.Series)
	}
	if got.Series[0].Bucket != "2026-01" {
		t.Errorf("first bucket = %q, want 2026-01", got.Series[0].Bucket)
	}
	if got.Series[11].Bucket != "2026-12" {
		t.Errorf("last bucket = %q, want 2026-12", got.Series[11].Bucket)
	}
	if got.Series[1].Points != 1000 {
		t.Errorf("february points = %d, want 1000", got.Series[1].Points)
	}
	if got.Series[10].Points != 500 {
		t.Errorf("november points = %d, want 500", got.Series[10].Points)
	}
}

func TestFormatDashboardBound_EchoesTheInclusiveDate(t *testing.T) {
	loc := spLocation(t)
	from, to := spBounds(t, "2026-07-01", "2026-07-31")

	if got := formatDashboardBound(from, loc, false); got != "2026-07-01" {
		t.Errorf("from = %q, want 2026-07-01", got)
	}
	// parseDateRange advanced the upper bound to 1 August; the response has to say the
	// 31st, which is the date the caller actually asked for.
	if got := formatDashboardBound(to, loc, true); got != "2026-07-31" {
		t.Errorf("to = %q, want 2026-07-31", got)
	}
	if got := formatDashboardBound(nil, loc, false); got != "" {
		t.Errorf("nil bound = %q, want empty", got)
	}
}
