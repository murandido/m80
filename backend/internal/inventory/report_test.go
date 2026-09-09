package inventory

import "testing"

const (
	wilmara = "wilmara"
	tiago    = "tiago"
)

// The ESFERA batch of August 2026, line for line off the spreadsheet.
//
// Five bookings drew on it, two of them issued by the agency that did not fund it, one at
// a different sale rate (EJC5NP sold at 15,25 rather than 15,00). The sheet closed it at
// R$ 962,20 of profit with R$ 3.606,40 owed - both reproduced below.
func esferaSlices() []Slice {
	return []Slice{
		// points x 15,00 per thousand, except EJC5NP at 15,25.
		{BatchID: "esfera", AgencyID: wilmara, Points: 251798, CostCents: 341942, RevenueCents: 377697}, // MJF6HC
		{BatchID: "esfera", AgencyID: tiago, Points: 120000, CostCents: 162960, RevenueCents: 180000},    // PKFM7M
		{BatchID: "esfera", AgencyID: wilmara, Points: 120000, CostCents: 162960, RevenueCents: 180000}, // XRGM2H
		{BatchID: "esfera", AgencyID: tiago, Points: 150000, CostCents: 203700, RevenueCents: 228750},    // EJC5NP
		{BatchID: "esfera", AgencyID: wilmara, Points: 9402, CostCents: 12768, RevenueCents: 14103},     // EPU9MK
	}
}

func esferaBatch() Batch {
	// 651,2 milheiros for R$ 8.843,30.
	return Batch{ID: "esfera", AcquiredOn: day("2026-08-05"), Points: 651200, CostCents: 884330}
}

// R$ 962,20 of profit and R$ 3.606,40 owed, which are the spreadsheet's own R4 and T4.
func TestSummariseBatchReproducesTheSpreadsheetsClosing(t *testing.T) {
	t.Parallel()

	result := SummariseBatch(esferaBatch(), wilmara, 50, esferaSlices(), []string{wilmara, tiago})

	if result.ChargedCents != 884330 {
		t.Errorf("charged %d cents, want the batch's whole 884330", result.ChargedCents)
	}
	if result.ProfitCents != 96220 {
		t.Errorf("profit = %d cents, want 96220 (R$ 962,20)", result.ProfitCents)
	}
	if result.RemainingPoints != 0 {
		t.Errorf("remaining = %d points, want 0 - the batch was fully consumed", result.RemainingPoints)
	}

	if len(result.Settlements) != 1 {
		t.Fatalf("got %d settlements, want 1 - only TIAGO did not fund it", len(result.Settlements))
	}
	settlement := result.Settlements[0]
	if settlement.AgencyID != tiago || settlement.ToAgencyID != wilmara {
		t.Errorf("settlement is %s owing %s, want tiago owing wilmara", settlement.AgencyID, settlement.ToAgencyID)
	}
	if settlement.AmountCents != 360640 {
		t.Errorf("owed = %d cents, want 360640 (R$ 3.606,40)", settlement.AmountCents)
	}
}

// The RESTO ULTIMA batch: every booking sold at exactly what the points cost, so the batch
// made nothing and the settlement is purely the cost of the points the other agency spent.
func TestSummariseBatchSettlesCostAloneWhenThereIsNoProfit(t *testing.T) {
	t.Parallel()

	carried := Batch{ID: "resto-ultima", AcquiredOn: day("2026-07-31"), Points: 1126202, CostCents: 1689303}
	slices := []Slice{
		// QGNBML: 240 milheiros bought and sold at 15,00.
		{BatchID: "resto-ultima", AgencyID: tiago, Points: 240000, CostCents: 360000, RevenueCents: 360000},
	}

	result := SummariseBatch(carried, wilmara, 50, slices, []string{wilmara, tiago})

	if result.ProfitCents != 0 {
		t.Errorf("profit = %d, want 0", result.ProfitCents)
	}
	if got := result.Settlements[0].AmountCents; got != 360000 {
		t.Errorf("owed = %d cents, want 360000 (R$ 3.600,00) - the cost of the points TIAGO spent", got)
	}
}

// A batch the funder used alone still settles: the split is on the batch's whole profit,
// so the partner is owed its share of what the funder earned even having taken nothing.
//
// These are the spreadsheet's CLUBE AZUL figures - 21 milheiros that cost nothing, sold at
// 15,00 for R$ 315,00 of profit, closing at R$ 157,50 owed the other way.
func TestSummariseBatchSettlesTheFundersOwnProfitWithThePartner(t *testing.T) {
	t.Parallel()

	club := Batch{ID: "clube-azul", AcquiredOn: day("2026-08-09"), Points: 21000, CostCents: 0}
	slices := []Slice{
		{BatchID: "clube-azul", AgencyID: wilmara, Points: 21000, CostCents: 0, RevenueCents: 31500},
	}

	result := SummariseBatch(club, wilmara, 50, slices, []string{wilmara, tiago})

	if result.ProfitCents != 31500 {
		t.Fatalf("profit = %d cents, want 31500", result.ProfitCents)
	}
	if len(result.Settlements) != 1 {
		t.Fatalf("got %d settlements, want 1 - the partner is owed its half", len(result.Settlements))
	}
	// Negative: the debt runs from the funder to the partner, not the other way.
	if got := result.Settlements[0].AmountCents; got != -15750 {
		t.Errorf("owed = %d cents, want -15750 (R$ 157,50 from wilmara to tiago)", got)
	}
}

// With only one agency on the account there is no partner, so there is nothing to settle.
// This is the account of a single owner, and it needs no flag to say so.
func TestSummariseBatchSettlesNothingWithoutAPartner(t *testing.T) {
	t.Parallel()

	slices := []Slice{
		{BatchID: "esfera", AgencyID: wilmara, Points: 251798, CostCents: 341942, RevenueCents: 377697},
	}

	result := SummariseBatch(esferaBatch(), wilmara, 50, slices, []string{wilmara})

	if len(result.Settlements) != 0 {
		t.Errorf("got %v, want no settlement at all", result.Settlements)
	}
	if result.ProfitCents == 0 {
		t.Error("the batch should still report its profit - only the settlement is empty")
	}
}

// A share of zero leaves the profit whole with whoever funded the batch: the other agency
// reimburses the points it spent and keeps nothing of what it earned on them.
func TestSummariseBatchWithNoSplitOwesTheWholeProfit(t *testing.T) {
	t.Parallel()

	result := SummariseBatch(esferaBatch(), wilmara, 0, esferaSlices(), []string{wilmara, tiago})

	// TIAGO's own cost plus all of TIAGO's profit, and no share of WILMARA's.
	const tiagoCost, tiagoProfit = 162960 + 203700, (180000 + 228750) - (162960 + 203700)
	if got, want := result.Settlements[0].AmountCents, tiagoCost+tiagoProfit; got != want {
		t.Errorf("owed = %d cents, want %d", got, want)
	}
}

// The split is read from the batch, so an arrangement other than half works without any
// special case - the reason it is not hard-coded.
func TestSummariseBatchHonoursASplitOtherThanHalf(t *testing.T) {
	t.Parallel()

	result := SummariseBatch(esferaBatch(), wilmara, 60, esferaSlices(), []string{wilmara, tiago})

	const (
		tiagoCost      = 162960 + 203700
		tiagoProfit    = (180000 + 228750) - tiagoCost
		wilmaraProfit = (377697 + 180000 + 14103) - (341942 + 162960 + 12768)
	)
	// TIAGO keeps 60% of its own profit and takes 60% of WILMARA's.
	want := tiagoCost + shareOf(tiagoProfit, 40) - shareOf(wilmaraProfit, 60)
	if got := result.Settlements[0].AmountCents; got != want {
		t.Errorf("owed = %d cents, want %d", got, want)
	}
}

// A batch bought too dear loses money - C6 at 27,00 per thousand against sales at 15,00.
// The loss has to split like a profit does, or the agency that funded it carries all of it.
func TestSummariseBatchSplitsALossToo(t *testing.T) {
	t.Parallel()

	// 333 milheiros for R$ 8.991,00: 27,00 per thousand.
	c6 := Batch{ID: "c6", AcquiredOn: day("2026-08-25"), Points: 333000, CostCents: 899100}
	slices := []Slice{
		// XW2RXG, issued by TIAGO: 66,783 milheiros costing 27,00 and sold at 15,00.
		{BatchID: "c6", AgencyID: tiago, Points: 66783, CostCents: 180314, RevenueCents: 100175},
	}

	result := SummariseBatch(c6, wilmara, 50, slices, []string{wilmara, tiago})

	if result.ProfitCents >= 0 {
		t.Fatalf("profit = %d, want a loss", result.ProfitCents)
	}
	// TIAGO reimburses the points and gives back half of the loss it made on them.
	want := 180314 + shareOf(100175-180314, 50)
	if got := result.Settlements[0].AmountCents; got != want {
		t.Errorf("owed = %d cents, want %d", got, want)
	}
}

// A club batch costs nothing, so the whole sale is profit - and the other agency owes only
// its share of it, with no points to reimburse.
func TestSummariseBatchHandlesAZeroCostBatch(t *testing.T) {
	t.Parallel()

	club := Batch{ID: "clube-azul", AcquiredOn: day("2026-08-10"), Points: 21000, CostCents: 0}
	slices := []Slice{
		{BatchID: "clube-azul", AgencyID: tiago, Points: 21000, CostCents: 0, RevenueCents: 31500},
	}

	result := SummariseBatch(club, wilmara, 50, slices, []string{wilmara, tiago})

	if result.ProfitCents != 31500 {
		t.Errorf("profit = %d cents, want the whole sale, 31500", result.ProfitCents)
	}
	if got := result.Settlements[0].AmountCents; got != 15750 {
		t.Errorf("owed = %d cents, want half the profit, 15750", got)
	}
}

// A batch still holding points reports what is left, which is what carries into the next
// period.
func TestSummariseBatchReportsTheRemainder(t *testing.T) {
	t.Parallel()

	slices := []Slice{
		{BatchID: "esfera", AgencyID: wilmara, Points: 251798, CostCents: 341942, RevenueCents: 377697},
	}

	result := SummariseBatch(esferaBatch(), wilmara, 50, slices, []string{wilmara, tiago})

	if want := 651200 - 251798; result.RemainingPoints != want {
		t.Errorf("remaining = %d points, want %d", result.RemainingPoints, want)
	}
}

// Slices from another batch must not leak into this one's line.
func TestSummariseBatchIgnoresOtherBatchesSlices(t *testing.T) {
	t.Parallel()

	slices := append(esferaSlices(), Slice{
		BatchID: "livelo", AgencyID: tiago, Points: 500000, CostCents: 700000, RevenueCents: 750000,
	})

	result := SummariseBatch(esferaBatch(), wilmara, 50, slices, []string{wilmara, tiago})

	if result.ConsumedPoints != 651200 {
		t.Errorf("consumed = %d points, want only esfera's 651200", result.ConsumedPoints)
	}
}
