package handlers

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/statement"
)

// groupingLine builds a parallel row/entry pair as the entries handler would. An empty instant
// is a row synced before the clock was kept.
func groupingLine(day, instant, kind, locator string, points int, route ...string) (data.AzulStatementLine, schemas.AzulStatementEntry) {
	row := data.AzulStatementLine{Points: points}
	if instant != "" {
		t, err := time.Parse("2006-01-02T15:04:05", instant)
		if err != nil {
			panic(err)
		}
		row.OccurredAt = &t
	}
	entry := schemas.AzulStatementEntry{Key: locator, OccurredOn: day, Kind: kind, Locator: locator, Points: points}
	if len(route) == 2 {
		entry.Origin, entry.Destination = route[0], route[1]
	}
	return row, entry
}

func TestGroupStatementEntries(t *testing.T) {
	var rows []data.AzulStatementLine
	var entries []schemas.AzulStatementEntry
	add := func(row data.AzulStatementLine, entry schemas.AzulStatementEntry) {
		rows = append(rows, row)
		entries = append(entries, entry)
	}
	const day = "2026-09-26"
	redemption, refund := data.AzulLineRedemption, data.AzulLineRefund

	add(groupingLine(day, "2026-09-26T12:57:59", redemption, "MN2YGG", -18000, "GRU", "REC")) // straddles the minute
	add(groupingLine(day, "2026-09-26T12:58:00", redemption, "MN2YGG", -18000, "GRU", "REC"))
	add(groupingLine(day, "2026-09-26T13:52:07", redemption, "RQR7HK", -32000, "CNF", "SSA"))
	add(groupingLine(day, "2026-09-26T13:52:07", redemption, "OTHER1", -1000)) // interleaved
	add(groupingLine(day, "2026-09-26T13:52:08", redemption, "RQR7HK", -28000, "SSA", "CNF"))
	add(groupingLine(day, "2026-09-26T15:17:07", redemption, "MTJFFB", -8000))
	add(groupingLine(day, "2026-09-26T15:25:00", redemption, "MTJFFB", -12000)) // a remarking minutes later
	add(groupingLine(day, "2026-09-26T17:32:16", refund, "MTJFFB", 20000))      // its refund
	add(groupingLine(day, "2026-09-26T18:00:00", data.AzulLinePurchase, "", 5000))
	add(groupingLine(day, "2026-09-26T18:00:00", data.AzulLinePurchase, "", 5000)) // no locator: alone
	add(groupingLine("2026-08-20", "", redemption, "OLDROW", -32000))              // clockless, same day
	add(groupingLine("2026-08-20", "", redemption, "OLDROW", -32000))

	refunded := 20000
	linked := func(points int, refundedPoints *int) statement.ResolvedBooking {
		return statement.ResolvedBooking{Issuance: data.Issuance{Points: points, RefundedPoints: refundedPoints}}
	}
	resolved := map[string]statement.ResolvedBooking{
		"RQR7HK": linked(50000, nil), // the statement says 60000: flagged
		"MTJFFB": linked(20000, &refunded),
	}
	for i := range entries {
		if _, ok := resolved[entries[i].Key]; ok {
			entries[i].LinkedIssuance = &schemas.AzulStatementLinkedIssuance{}
		}
	}

	groups := groupStatementEntries(rows, entries, resolved)

	type want struct {
		locator, kind string
		points, lines int
		mismatch      bool
	}
	wants := []want{
		{"MN2YGG", redemption, -36000, 2, false},
		{"RQR7HK", redemption, -60000, 2, true},
		{"OTHER1", redemption, -1000, 1, false},
		{"MTJFFB", redemption, -8000, 1, false}, // 8000 + 12000 across both groups = 20000
		{"MTJFFB", redemption, -12000, 1, false},
		{"MTJFFB", refund, 20000, 1, false},
		{"", data.AzulLinePurchase, 5000, 1, false},
		{"", data.AzulLinePurchase, 5000, 1, false},
		{"OLDROW", redemption, -64000, 2, false},
	}
	if len(groups) != len(wants) {
		t.Fatalf("got %d groups, want %d: %+v", len(groups), len(wants), groups)
	}
	for i, w := range wants {
		g := groups[i]
		if g.Locator != w.locator || g.Kind != w.kind || g.Points != w.points || g.LineCount != w.lines || g.PointsMismatch != w.mismatch {
			t.Errorf("group %d = %s %s %d pts %d lines mismatch=%v, want %+v",
				i, g.Locator, g.Kind, g.Points, g.LineCount, g.PointsMismatch, w)
		}
	}
	if routes := groups[1].Routes; len(routes) != 2 || routes[0] != "CNF→SSA" || routes[1] != "SSA→CNF" {
		t.Errorf("round trip routes = %v, want [CNF→SSA SSA→CNF]", routes)
	}
}
