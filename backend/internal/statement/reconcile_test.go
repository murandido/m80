package statement

import (
	"testing"
	"time"
)

func day(value string) time.Time {
	parsed, err := time.Parse(time.DateOnly, value)
	if err != nil {
		panic(err)
	}
	return parsed
}

func bookingLine(lines []BookingLine, locator string) (BookingLine, bool) {
	for _, line := range lines {
		if line.Locator == locator {
			return line, true
		}
	}
	return BookingLine{}, false
}

// Azul writes one line per leg, so a booking of five legs arrives as five movements of
// -500. Comparing them one by one would report a correct booking as five mismatches.
func TestReconcileSumsTheLegsOfOneBooking(t *testing.T) {
	t.Parallel()

	movements := make([]Movement, 5)
	for i := range movements {
		movements[i] = Movement{
			OccurredOn: day("2026-09-01"), Kind: KindRedemption, Locator: "RLGHNM", Points: -500,
		}
	}
	panel := []PanelBooking{{IssuanceID: "i1", Locator: "RLGHNM", IssuedOn: day("2026-09-01"), Points: 2500}}

	result := Reconcile(movements, panel, nil)

	line, found := bookingLine(result.Bookings, "RLGHNM")
	if !found {
		t.Fatal("RLGHNM missing from the comparison")
	}
	if line.Verdict != VerdictMatched {
		t.Errorf("verdict = %s, want matched (2.500 on both sides)", line.Verdict)
	}
	if line.StatementPoints != 2500 {
		t.Errorf("statement points = %d, want the five legs summed to 2500", line.StatementPoints)
	}
}

func TestReconcileFindsABookingOnlyOnTheStatement(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-15"), Kind: KindRedemption, Locator: "NEWPNR", Points: -120000},
	}, nil, nil)

	line, _ := bookingLine(result.Bookings, "NEWPNR")
	if line.Verdict != VerdictMissingInPanel {
		t.Errorf("verdict = %s, want missing_in_panel", line.Verdict)
	}
}

func TestReconcileFindsABookingOnlyInThePanel(t *testing.T) {
	t.Parallel()

	result := Reconcile(nil, []PanelBooking{
		{IssuanceID: "i1", Locator: "ONLYUS", IssuedOn: day("2026-08-15"), Points: 120000},
	}, nil)

	line, _ := bookingLine(result.Bookings, "ONLYUS")
	if line.Verdict != VerdictMissingInStatement {
		t.Errorf("verdict = %s, want missing_in_statement", line.Verdict)
	}
}

func TestReconcileFindsABookingWorthDifferentPoints(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-15"), Kind: KindRedemption, Locator: "MJF6HC", Points: -360000},
	}, []PanelBooking{
		{IssuanceID: "i1", Locator: "MJF6HC", IssuedOn: day("2026-08-15"), Points: 350000},
	}, nil)

	line, _ := bookingLine(result.Bookings, "MJF6HC")
	if line.Verdict != VerdictAmountDiffers {
		t.Fatalf("verdict = %s, want amount_differs", line.Verdict)
	}
	if line.StatementPoints != 360000 || line.PanelPoints != 350000 {
		t.Errorf("got %d vs %d, want 360000 vs 350000", line.StatementPoints, line.PanelPoints)
	}
}

// The case that costs money: the airline gave points back and the panel never recorded it,
// so the costing is still charging them.
func TestReconcileFindsARefundThePanelMissed(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-16"), Kind: KindRefund, Locator: "YW51QG", Points: 40000},
	}, []PanelBooking{
		{IssuanceID: "i1", Locator: "YW51QG", IssuedOn: day("2026-08-06"), Points: 40000, RefundedPoints: 0},
	}, nil)

	if len(result.Refunds) != 1 {
		t.Fatalf("got %d refund lines, want 1", len(result.Refunds))
	}
	refund := result.Refunds[0]
	if refund.Verdict != VerdictAmountDiffers {
		t.Errorf("verdict = %s, want amount_differs - the panel recorded no refund", refund.Verdict)
	}
	if refund.StatementPoints != 40000 || refund.PanelPoints != 0 {
		t.Errorf("got %d vs %d, want 40000 vs 0", refund.StatementPoints, refund.PanelPoints)
	}
}

func TestReconcileAcceptsARefundBothSidesAgreeOn(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-16"), Kind: KindRefund, Locator: "YW51QG", Points: 40000},
	}, []PanelBooking{
		{IssuanceID: "i1", Locator: "YW51QG", IssuedOn: day("2026-08-06"), Points: 40000, RefundedPoints: 40000},
	}, nil)

	if result.Refunds[0].Verdict != VerdictMatched {
		t.Errorf("verdict = %s, want matched", result.Refunds[0].Verdict)
	}
}

// The finding the package exists for: points arrived and nobody priced them.
func TestReconcileFindsACreditWithNoBatch(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-21"), Kind: KindAccrual, Points: 632700, Description: "Transferência de Pontos Livelo"},
	}, nil, nil)

	if len(result.Credits) != 1 {
		t.Fatalf("got %d credit lines, want 1", len(result.Credits))
	}
	credit := result.Credits[0]
	if credit.Verdict != VerdictMissingInPanel {
		t.Errorf("verdict = %s, want missing_in_panel", credit.Verdict)
	}
	if credit.Description != "Transferência de Pontos Livelo" {
		t.Errorf("description = %q - it is what suggests the origin", credit.Description)
	}
}

// Four instalments of one purchase are four credits and four batches. Pairing by day alone
// would match the first and call the other three missing.
func TestReconcilePairsInstalmentsOneToOne(t *testing.T) {
	t.Parallel()

	var movements []Movement
	var batches []PanelBatch
	for i := 0; i < 4; i++ {
		movements = append(movements, Movement{
			OccurredOn: day("2026-08-27"), Kind: KindAccrual, Points: 158175, Description: "Resgate pontos Inter",
		})
		batches = append(batches, PanelBatch{
			BatchID: string(rune('a' + i)), AcquiredOn: day("2026-08-27"), Points: 158175,
		})
	}

	result := Reconcile(movements, nil, batches)

	if len(result.Credits) != 4 {
		t.Fatalf("got %d credit lines, want 4", len(result.Credits))
	}
	claimed := map[string]bool{}
	for _, credit := range result.Credits {
		if credit.Verdict != VerdictMatched {
			t.Errorf("credit of %d is %s, want matched", credit.Points, credit.Verdict)
		}
		if claimed[credit.BatchID] {
			t.Errorf("batch %s matched twice - the pairing must be one to one", credit.BatchID)
		}
		claimed[credit.BatchID] = true
	}
}

// Three instalments recorded out of four: the fourth has to surface, not be absorbed.
func TestReconcileFindsTheOneInstalmentThatWasNotRecorded(t *testing.T) {
	t.Parallel()

	var movements []Movement
	for i := 0; i < 4; i++ {
		movements = append(movements, Movement{
			OccurredOn: day("2026-08-27"), Kind: KindAccrual, Points: 158175,
		})
	}
	batches := []PanelBatch{
		{BatchID: "a", AcquiredOn: day("2026-08-27"), Points: 158175},
		{BatchID: "b", AcquiredOn: day("2026-08-27"), Points: 158175},
		{BatchID: "c", AcquiredOn: day("2026-08-27"), Points: 158175},
	}

	result := Reconcile(movements, nil, batches)

	missing := 0
	for _, credit := range result.Credits {
		if credit.Verdict == VerdictMissingInPanel {
			missing++
		}
	}
	if missing != 1 {
		t.Errorf("got %d unrecorded credits, want exactly 1", missing)
	}
}

// A batch nobody claimed is recorded in the panel and absent from the statement - either
// wrong or from another period, and either way worth saying.
func TestReconcileFindsABatchTheStatementDoesNotMention(t *testing.T) {
	t.Parallel()

	result := Reconcile(nil, nil, []PanelBatch{
		{BatchID: "ghost", AcquiredOn: day("2026-08-01"), Points: 100000},
	})

	if len(result.Credits) != 1 || result.Credits[0].Verdict != VerdictMissingInStatement {
		t.Fatalf("got %+v, want one missing_in_statement", result.Credits)
	}
}

// Neither export states a balance, so agreeing on the change in balance is the only check
// available - and disagreeing is the signal that something above was missed.
func TestReconcileComparesTheChangeInBalance(t *testing.T) {
	t.Parallel()

	result := Reconcile([]Movement{
		{OccurredOn: day("2026-08-21"), Kind: KindAccrual, Points: 632700},
		{OccurredOn: day("2026-08-22"), Kind: KindRedemption, Locator: "A1", Points: -120000},
		{OccurredOn: day("2026-08-23"), Kind: KindRefund, Locator: "A1", Points: 40000},
	}, []PanelBooking{
		{IssuanceID: "i1", Locator: "A1", IssuedOn: day("2026-08-22"), Points: 120000, RefundedPoints: 40000},
	}, []PanelBatch{
		{BatchID: "b1", AcquiredOn: day("2026-08-21"), Points: 632700},
	})

	if result.StatementNetPoints != result.PanelNetPoints {
		t.Errorf("statement says %d, panel says %d - they agree line by line, so the totals must agree",
			result.StatementNetPoints, result.PanelNetPoints)
	}
	if result.StatementNetPoints != 552700 {
		t.Errorf("net = %d, want 552700", result.StatementNetPoints)
	}
}

// The file that was already committed, sent again. The four Inter credits were summed into
// one batch of 675.000, which resembles none of them - only the provenance recorded on the
// batch can say they are already in the panel.
func TestReconcileClaimsTheCreditsASummedBatchCameFrom(t *testing.T) {
	t.Parallel()

	var movements []Movement
	var lines []BatchLine
	for i := 0; i < 4; i++ {
		movements = append(movements, Movement{
			OccurredOn: day("2026-09-04"), Kind: KindAccrual, Points: 168750, Description: "Resgate pontos Inter",
		})
		lines = append(lines, BatchLine{OccurredOn: day("2026-09-04"), Points: 168750})
	}
	// The accruals of three different days, summed into a batch dated at the earliest.
	accruals := []struct {
		day    string
		points int
	}{{"2026-09-03", 1340}, {"2026-09-04", 2584}, {"2026-09-05", 7320}}
	var accrualLines []BatchLine
	for _, accrual := range accruals {
		movements = append(movements, Movement{
			OccurredOn: day(accrual.day), Kind: KindAccrual, Points: accrual.points,
		})
		accrualLines = append(accrualLines, BatchLine{OccurredOn: day(accrual.day), Points: accrual.points})
	}

	batches := []PanelBatch{
		{BatchID: "inter", AcquiredOn: day("2026-09-04"), Points: 675000, Lines: lines},
		{BatchID: "accrual", AcquiredOn: day("2026-09-03"), Points: 11244, Lines: accrualLines},
	}

	result := Reconcile(movements, nil, batches)

	for _, credit := range result.Credits {
		if credit.Verdict != VerdictMatched {
			t.Errorf("credit of %d on %s is %s, want matched",
				credit.Points, credit.OccurredOn.Format("2006-01-02"), credit.Verdict)
		}
	}
	if len(result.Credits) != len(movements) {
		t.Errorf("got %d credit lines, want %d - a summed batch must not add a line of its own",
			len(result.Credits), len(movements))
	}
}

// Provenance covers only what it names. A credit that arrived after the batch was recorded
// still has to surface, or the second import silently swallows it.
func TestReconcileStillFindsACreditTheSummedBatchDoesNotCover(t *testing.T) {
	t.Parallel()

	movements := []Movement{
		{OccurredOn: day("2026-09-04"), Kind: KindAccrual, Points: 168750},
		{OccurredOn: day("2026-09-04"), Kind: KindAccrual, Points: 168750},
		{OccurredOn: day("2026-09-06"), Kind: KindAccrual, Points: 8500},
	}
	batches := []PanelBatch{{
		BatchID: "inter", AcquiredOn: day("2026-09-04"), Points: 337500,
		Lines: []BatchLine{
			{OccurredOn: day("2026-09-04"), Points: 168750},
			{OccurredOn: day("2026-09-04"), Points: 168750},
		},
	}}

	result := Reconcile(movements, nil, batches)

	missing := 0
	for _, credit := range result.Credits {
		if credit.Verdict == VerdictMissingInPanel {
			missing++
			if credit.Points != 8500 {
				t.Errorf("the unrecorded credit is %d, want the 8.500 one", credit.Points)
			}
		}
	}
	if missing != 1 {
		t.Errorf("got %d unrecorded credits, want exactly 1", missing)
	}
}

// The column survives a round trip, and a value that does not parse reads as no provenance
// rather than as an error - the batch then compares the way a hand-entered one does.
func TestBatchLinesRoundTrip(t *testing.T) {
	t.Parallel()

	lines := []BatchLine{
		{OccurredOn: day("2026-09-03"), Points: 1340},
		{OccurredOn: day("2026-09-04"), Points: 168750},
	}
	encoded, err := EncodeBatchLines(lines)
	if err != nil {
		t.Fatalf("encoding failed: %v", err)
	}
	if string(encoded) != `[{"d":"2026-09-03","p":1340},{"d":"2026-09-04","p":168750}]` {
		t.Errorf("stored shape changed: %s", encoded)
	}

	decoded := DecodeBatchLines(encoded)
	if len(decoded) != 2 || !decoded[1].OccurredOn.Equal(day("2026-09-04")) || decoded[1].Points != 168750 {
		t.Errorf("round trip lost the lines: %+v", decoded)
	}
	if empty, err := EncodeBatchLines(nil); err != nil || empty != nil {
		t.Errorf("no lines must store no value, got %q (%v)", empty, err)
	}
	if broken := DecodeBatchLines([]byte(`{"nope":1}`)); broken != nil {
		t.Errorf("a value that does not parse must read as no provenance, got %+v", broken)
	}
}
