package statement

import (
	"sort"
	"time"
)

// PanelBooking is what the panel holds for one locator, as the reconciliation sees it.
type PanelBooking struct {
	IssuanceID     string
	Locator        string
	IssuedOn       time.Time
	Points         int
	RefundedPoints int
}

// PanelBatch is one acquisition already recorded, as the reconciliation sees it.
type PanelBatch struct {
	BatchID    string
	AcquiredOn time.Time
	Points     int
	// Lines are the statement credits this batch was built from, when it was built from a
	// statement at all. Empty on a batch entered by hand, which is then matched by date
	// and amount the way it always was.
	Lines []BatchLine
}

// Verdict says what a comparison found. The wording is the operator's: each one names what
// is wrong rather than which side is missing a row.
type Verdict string

const (
	// VerdictMatched is the same figure on both sides. Reported so the screen can say how
	// much of the statement is already right, which is most of it.
	VerdictMatched Verdict = "matched"
	// VerdictMissingInPanel is on the statement and not in the panel.
	VerdictMissingInPanel Verdict = "missing_in_panel"
	// VerdictMissingInStatement is in the panel and not on the statement.
	VerdictMissingInStatement Verdict = "missing_in_statement"
	// VerdictAmountDiffers is on both sides for different points.
	VerdictAmountDiffers Verdict = "amount_differs"
)

// BookingLine compares one locator.
type BookingLine struct {
	Locator string
	Verdict Verdict
	// StatementPoints is what the statement says left the account, as a positive number.
	// Several legs of one booking are already summed.
	StatementPoints int
	PanelPoints     int
	IssuanceID      string
	OccurredOn      time.Time
}

// RefundLine compares one booking's refund.
type RefundLine struct {
	Locator         string
	Verdict         Verdict
	StatementPoints int
	PanelPoints     int
	IssuanceID      string
	OccurredOn      time.Time
}

// CreditLine is one credit on the statement, matched to a batch or not.
//
// This is the finding the whole package exists for: a credit with no batch is stock that
// arrived and was never priced, and the FIFO is spending it at whatever the neighbouring
// batch cost. Nothing in the panel says so today.
type CreditLine struct {
	OccurredOn  time.Time
	Points      int
	Description string
	Verdict     Verdict
	BatchID     string
}

// Reconciliation is the whole comparison.
type Reconciliation struct {
	Bookings []BookingLine
	Refunds  []RefundLine
	Credits  []CreditLine

	// StatementNetPoints and PanelNetPoints are the change in balance each side claims for
	// the period. Neither export states a balance, so agreeing on the change is the only
	// check available - and disagreeing is the signal that something above was missed.
	StatementNetPoints int
	PanelNetPoints     int
}

// Reconcile compares a statement with what the panel holds for the same account and period.
//
// Pure: everything it needs is in its arguments, so the comparison is testable against the
// real exports without a database.
//
// The panel side is filtered by the caller to the statement's own period. Comparing a
// statement of one month against every booking the account ever had would report the whole
// history as missing.
func Reconcile(movements []Movement, bookings []PanelBooking, batches []PanelBatch) Reconciliation {
	result := Reconciliation{}

	// Several statement lines share one locator - Azul writes one per leg - so they are
	// summed before anything is compared. Comparing leg by leg would report a booking of
	// five legs as five mismatches.
	statementBookings := map[string]int{}
	statementRefunds := map[string]int{}
	var credits []Movement

	for _, movement := range movements {
		result.StatementNetPoints += movement.Points
		switch movement.Kind {
		case KindRedemption:
			statementBookings[movement.Locator] -= movement.Points // stored positive
		case KindRefund:
			statementRefunds[movement.Locator] += movement.Points
		case KindAccrual:
			credits = append(credits, movement)
		}
	}

	byLocator := map[string]PanelBooking{}
	for _, booking := range bookings {
		byLocator[booking.Locator] = booking
		result.PanelNetPoints += booking.RefundedPoints - booking.Points
	}
	for _, batch := range batches {
		result.PanelNetPoints += batch.Points
	}

	result.Bookings = reconcileBookings(statementBookings, byLocator)
	result.Refunds = reconcileRefunds(statementRefunds, byLocator)
	result.Credits = reconcileCredits(credits, batches)
	return result
}

func reconcileBookings(statement map[string]int, panel map[string]PanelBooking) []BookingLine {
	lines := make([]BookingLine, 0, len(statement))
	seen := map[string]bool{}

	for locator, points := range statement {
		seen[locator] = true
		line := BookingLine{Locator: locator, StatementPoints: points}
		booking, found := panel[locator]
		switch {
		case !found:
			line.Verdict = VerdictMissingInPanel
		case booking.Points == points:
			line.Verdict = VerdictMatched
			line.PanelPoints = booking.Points
			line.IssuanceID = booking.IssuanceID
			line.OccurredOn = booking.IssuedOn
		default:
			line.Verdict = VerdictAmountDiffers
			line.PanelPoints = booking.Points
			line.IssuanceID = booking.IssuanceID
			line.OccurredOn = booking.IssuedOn
		}
		lines = append(lines, line)
	}

	for locator, booking := range panel {
		if seen[locator] {
			continue
		}
		lines = append(lines, BookingLine{
			Locator:     locator,
			Verdict:     VerdictMissingInStatement,
			PanelPoints: booking.Points,
			IssuanceID:  booking.IssuanceID,
			OccurredOn:  booking.IssuedOn,
		})
	}

	sortBookings(lines)
	return lines
}

func reconcileRefunds(statement map[string]int, panel map[string]PanelBooking) []RefundLine {
	lines := make([]RefundLine, 0, len(statement))
	seen := map[string]bool{}

	for locator, points := range statement {
		seen[locator] = true
		line := RefundLine{Locator: locator, StatementPoints: points}
		booking, found := panel[locator]
		switch {
		case !found:
			line.Verdict = VerdictMissingInPanel
		case booking.RefundedPoints == points:
			line.Verdict = VerdictMatched
			line.PanelPoints = booking.RefundedPoints
			line.IssuanceID = booking.IssuanceID
		default:
			// Includes the panel recording no refund at all, which is the case that
			// matters: the costing is still charging points the airline gave back.
			line.Verdict = VerdictAmountDiffers
			line.PanelPoints = booking.RefundedPoints
			line.IssuanceID = booking.IssuanceID
		}
		lines = append(lines, line)
	}

	// A refund the panel recorded and the statement does not mention. Rarer than the other
	// direction, and worth saying: it means the panel gave back points the airline did not.
	for locator, booking := range panel {
		if seen[locator] || booking.RefundedPoints == 0 {
			continue
		}
		lines = append(lines, RefundLine{
			Locator:     locator,
			Verdict:     VerdictMissingInStatement,
			PanelPoints: booking.RefundedPoints,
			IssuanceID:  booking.IssuanceID,
		})
	}

	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Locator < lines[j].Locator })
	return lines
}

// reconcileCredits pairs each credit with a batch of the same day and size.
//
// Day and size, not day alone: a purchase arrives in instalments - the INTER batch of
// 632.700 landed as four credits of 158.175 - and each instalment is its own batch. Pairing
// by day would match the first instalment to the first batch and call the other three
// missing.
//
// Matching is one to one and greedy over a sorted list, so four identical credits on one
// day pair with four identical batches rather than all with the first.
func reconcileCredits(credits []Movement, batches []PanelBatch) []CreditLine {
	sorted := append([]Movement(nil), credits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if !sorted[i].OccurredOn.Equal(sorted[j].OccurredOn) {
			return sorted[i].OccurredOn.Before(sorted[j].OccurredOn)
		}
		return sorted[i].Points < sorted[j].Points
	})

	used := make([]bool, len(batches))
	lines := make([]CreditLine, 0, len(sorted))
	for _, credit := range sorted {
		lines = append(lines, CreditLine{
			OccurredOn:  credit.OccurredOn,
			Points:      credit.Points,
			Description: credit.Description,
			Verdict:     VerdictMissingInPanel,
		})
	}

	// Batches that say which credits they came from are claimed first. A batch that summed
	// four credits matches none of them by date and amount - that is what summing did - so
	// without this pass the same file reconciled twice would offer to record all four
	// again.
	for i, batch := range batches {
		if len(batch.Lines) == 0 {
			continue
		}
		for _, recorded := range batch.Lines {
			for index := range lines {
				if lines[index].Verdict != VerdictMissingInPanel {
					continue
				}
				if lines[index].Points != recorded.Points || !lines[index].OccurredOn.Equal(recorded.OccurredOn) {
					continue
				}
				lines[index].Verdict = VerdictMatched
				lines[index].BatchID = batch.BatchID
				used[i] = true
				break
			}
		}
	}

	// Then the batches entered by hand, one credit to one batch by date and amount.
	for index := range lines {
		if lines[index].Verdict != VerdictMissingInPanel {
			continue
		}
		for i, batch := range batches {
			if used[i] || len(batch.Lines) > 0 {
				continue
			}
			if batch.Points != lines[index].Points || !batch.AcquiredOn.Equal(lines[index].OccurredOn) {
				continue
			}
			used[i] = true
			lines[index].Verdict = VerdictMatched
			lines[index].BatchID = batch.BatchID
			break
		}
	}

	// Batches nobody claimed: recorded in the panel and absent from the statement. Either
	// the batch is wrong or it belongs to another period.
	for i, batch := range batches {
		if used[i] {
			continue
		}
		lines = append(lines, CreditLine{
			OccurredOn: batch.AcquiredOn,
			Points:     batch.Points,
			Verdict:    VerdictMissingInStatement,
			BatchID:    batch.BatchID,
		})
	}

	sort.SliceStable(lines, func(i, j int) bool {
		if !lines[i].OccurredOn.Equal(lines[j].OccurredOn) {
			return lines[i].OccurredOn.Before(lines[j].OccurredOn)
		}
		return lines[i].Points < lines[j].Points
	})
	return lines
}

func sortBookings(lines []BookingLine) {
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].Locator < lines[j].Locator })
}
