package handlers

import (
	"time"

	"github.com/murandido/m-80-backend/carriers/azul"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/statement"
)

// sameOperationWindow is how far apart two movements of one locator and kind may be and still be
// one operation. The airline stamps each leg and passenger of an issuance separately, a second or
// so apart, so a round trip for four is eight movements within a couple of seconds; a remarking
// or a second issuance on the same locator comes minutes later. A window rather than truncating
// to the minute, so an issuance that straddles hh:mm:59 -> hh:mm+1:00 is not split in two.
const sameOperationWindow = 60 * time.Second

// groupStatementEntries folds the statement lines into operations for the page: movements of one
// locator and kind no more than sameOperationWindow from the group's previous movement. A
// redemption and its refund never share a group (the kind differs), and a line without a locator
// (a purchase, a correction, a campaign enrolment) stands alone.
//
// rows and entries are parallel and already in display order (day, then instant). Rows stored
// before the clock was kept have no instant; those join a group only on the same day.
//
// Each group of a linked issuance is also checked against it: the locator's redemptions together
// must equal the issuance's points, and its refunds its refunded points (measured on a live
// account: every linked locator matched). The check is per locator, not per group, so a remarking
// that lands minutes later - a second group of the same issuance - is not flagged on its own.
func groupStatementEntries(rows []data.AzulStatementLine, entries []schemas.AzulStatementEntry,
	resolved map[string]statement.ResolvedBooking) []schemas.AzulStatementGroup {
	groups := make([]schemas.AzulStatementGroup, 0, len(entries))
	lastAt := make([]*time.Time, 0, len(entries)) // per group: its latest movement's instant
	openByKey := make(map[string]int)             // locator+kind -> index of its latest group
	totals := make(map[string]int)                // locator+kind -> signed points, all groups

	for i, entry := range entries {
		instant := rows[i].OccurredAt
		operationKey := entry.Locator + "|" + entry.Kind
		if entry.Locator != "" {
			totals[operationKey] += entry.Points
			if g, open := openByKey[operationKey]; open && sameOperation(groups[g], lastAt[g], entry, instant) {
				groups[g].Points += entry.Points
				groups[g].LineCount++
				groups[g].Routes = appendRoute(groups[g].Routes, entry)
				groups[g].Lines = append(groups[g].Lines, entry)
				if instant != nil {
					lastAt[g] = instant
				}
				continue
			}
		}

		groups = append(groups, schemas.AzulStatementGroup{
			Key:            entry.Key,
			OccurredOn:     entry.OccurredOn,
			OccurredAt:     entry.OccurredAt,
			Kind:           entry.Kind,
			Locator:        entry.Locator,
			Points:         entry.Points,
			LineCount:      1,
			Routes:         appendRoute(nil, entry),
			DayBalance:     entry.DayBalance,
			LinkedIssuance: entry.LinkedIssuance,
			LinkedBatch:    entry.LinkedBatch,
			Lines:          []schemas.AzulStatementEntry{entry},
		})
		lastAt = append(lastAt, instant)
		if entry.Locator != "" {
			openByKey[operationKey] = len(groups) - 1
		}
	}

	for g := range groups {
		groups[g].PointsMismatch = pointsMismatch(groups[g], totals, resolved)
	}
	return groups
}

// sameOperation reports whether a movement continues a group: within sameOperationWindow of the
// group's latest movement, or - for rows without an instant - on the same day.
func sameOperation(group schemas.AzulStatementGroup, groupLast *time.Time, entry schemas.AzulStatementEntry, instant *time.Time) bool {
	if groupLast == nil || instant == nil {
		return groupLast == nil && instant == nil && group.OccurredOn == entry.OccurredOn
	}
	gap := instant.Sub(*groupLast)
	if gap < 0 {
		gap = -gap
	}
	return gap <= sameOperationWindow
}

// appendRoute adds the line's leg ("CNF→SSA") once, keeping the order the legs were flown.
func appendRoute(routes []string, entry schemas.AzulStatementEntry) []string {
	if entry.Origin == "" || entry.Destination == "" {
		return routes
	}
	leg := entry.Origin + "→" + entry.Destination
	for _, existing := range routes {
		if existing == leg {
			return routes
		}
	}
	return append(routes, leg)
}

// pointsMismatch compares a linked redemption or refund against its issuance, by the locator's
// total across every group of that kind.
func pointsMismatch(group schemas.AzulStatementGroup, totals map[string]int, resolved map[string]statement.ResolvedBooking) bool {
	if group.Locator == "" || group.LinkedIssuance == nil {
		return false
	}
	booking, found := resolved[group.Key]
	if !found {
		return false
	}
	total := totals[group.Locator+"|"+group.Kind]
	switch group.Kind {
	case data.AzulLineRedemption:
		return -total != booking.RedeemedPoints()
	case data.AzulLineRefund:
		return total != booking.RefundedPoints()
	default:
		return false
	}
}

// brazilInstant renders a movement's instant in São Paulo time, the zone every screen reads, or
// nil for a line synced before the clock was kept.
func brazilInstant(instant *time.Time) *string {
	if instant == nil {
		return nil
	}
	formatted := instant.In(azul.BrazilLocation).Format(time.RFC3339)
	return &formatted
}
