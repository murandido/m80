package inventory

import "testing"

const (
	wilmara = "wilmara"
	tiago   = "tiago"
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
		{BatchID: "esfera", AgencyID: tiago, Points: 120000, CostCents: 162960, RevenueCents: 180000},   // PKFM7M
		{BatchID: "esfera", AgencyID: wilmara, Points: 120000, CostCents: 162960, RevenueCents: 180000}, // XRGM2H
		{BatchID: "esfera", AgencyID: tiago, Points: 150000, CostCents: 203700, RevenueCents: 228750},   // EJC5NP
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
		tiagoCost     = 162960 + 203700
		tiagoProfit   = (180000 + 228750) - tiagoCost
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

// A Diamante account's saved redemption fee is split by the batch's percent, like the
// profit but without the cost term: TIAGO kept R$ 300 of fees on a WILMARA batch and
// WILMARA kept R$ 100, so at half each TIAGO owes 150 - 50 = R$ 100. The points
// settlement is left exactly as it was.
func TestSummariseBatchSplitsTheRedemptionFeeSavedByTheSamePercent(t *testing.T) {
	t.Parallel()

	slices := []Slice{
		{BatchID: "b", AgencyID: tiago, Points: 10000, CostCents: 10000, RevenueCents: 15000, RedemptionFeeSavedCents: 30000, ProgramLocator: "TTT111", OccurredOn: day("2026-09-10")},
		{BatchID: "b", AgencyID: wilmara, Points: 10000, CostCents: 10000, RevenueCents: 15000, RedemptionFeeSavedCents: 10000, ProgramLocator: "WWW111", OccurredOn: day("2026-09-05")},
	}
	batch := Batch{ID: "b", Points: 20000, CostCents: 20000}

	result := SummariseBatch(batch, wilmara, 50, slices, []string{wilmara, tiago})

	if result.RedemptionFeeSavedCents != 40000 {
		t.Errorf("fee saved = %d, want 40000", result.RedemptionFeeSavedCents)
	}
	// Points: tiago spent 10000, made 5000, wilmara made 5000 -> 10000 + 2500 - 2500.
	if len(result.Settlements) != 1 || result.Settlements[0].AmountCents != 10000 {
		t.Fatalf("points settlement = %+v, want tiago owing 10000", result.Settlements)
	}
	if len(result.FeeSettlements) != 1 || result.FeeSettlements[0].AgencyID != tiago || result.FeeSettlements[0].AmountCents != 10000 {
		t.Fatalf("fee settlement = %+v, want tiago owing 10000", result.FeeSettlements)
	}
	if len(result.TotalSettlements) != 1 || result.TotalSettlements[0].AgencyID != tiago ||
		result.TotalSettlements[0].ToAgencyID != wilmara || result.TotalSettlements[0].AmountCents != 20000 {
		t.Fatalf("total settlement = %+v, want tiago owing wilmara 20000", result.TotalSettlements)
	}
	// The lines come back in date order, whatever order the slices arrived in.
	if len(result.Lines) != 2 || result.Lines[0].ProgramLocator != "WWW111" {
		t.Errorf("lines = %+v, want both, earliest first", result.Lines)
	}
}

// With a percent other than half, each side owes its partner that share of what it kept.
func TestSummariseBatchSplitsTheFeeByAPercentOtherThanHalf(t *testing.T) {
	t.Parallel()

	slices := []Slice{
		{BatchID: "b", AgencyID: tiago, Points: 1000, CostCents: 1000, RevenueCents: 1000, RedemptionFeeSavedCents: 10000},
	}
	result := SummariseBatch(Batch{ID: "b", Points: 1000, CostCents: 1000}, wilmara, 30, slices, []string{wilmara, tiago})

	// tiago keeps 30% of its own fee and owes wilmara the other 70%.
	if len(result.FeeSettlements) != 1 || result.FeeSettlements[0].AmountCents != 7000 {
		t.Fatalf("fee settlement = %+v, want tiago owing 7000", result.FeeSettlements)
	}
}

// "A owes B -100" and "B owes A 100" are one debt: netting reports each pair once, positive,
// in the direction it runs - the two same-direction lines the Smiles closing used to show.
func TestNetSettlementsMergesBothDirectionsOfAPair(t *testing.T) {
	t.Parallel()

	netted := NetSettlements(
		[]Settlement{{AgencyID: tiago, ToAgencyID: wilmara, AmountCents: -85499}},
		[]Settlement{{AgencyID: wilmara, ToAgencyID: tiago, AmountCents: 1151376}},
	)

	if len(netted) != 1 {
		t.Fatalf("got %d lines, want 1: %+v", len(netted), netted)
	}
	if netted[0].AgencyID != wilmara || netted[0].ToAgencyID != tiago || netted[0].AmountCents != 85499+1151376 {
		t.Errorf("netted = %+v, want wilmara owing tiago %d", netted[0], 85499+1151376)
	}
}

// A pair that nets to nothing has nothing to transfer, and no line.
func TestNetSettlementsDropsAPairThatCancelsOut(t *testing.T) {
	t.Parallel()

	netted := NetSettlements([]Settlement{
		{AgencyID: tiago, ToAgencyID: wilmara, AmountCents: 500},
		{AgencyID: wilmara, ToAgencyID: tiago, AmountCents: 500},
	})
	if len(netted) != 0 {
		t.Errorf("got %+v, want nothing", netted)
	}
}

// signedSum adds parts up in the direction of a settlement: what the debtor owes counts in,
// what the creditor owes back counts out.
func signedSum(parts []SettlementPart, debtor, creditor string) int {
	total := 0
	for _, part := range parts {
		switch {
		case part.PayerID == debtor && part.PayeeID == creditor:
			total += part.AmountCents
		case part.PayerID == creditor && part.PayeeID == debtor:
			total -= part.AmountCents
		}
	}
	return total
}

// The parts are the terms the settlement was summed from: on the spreadsheet's ESFERA batch
// they add back up to its R$ 3.606,40 to the centavo.
func TestSummariseBatchPartsAddUpToTheSettlement(t *testing.T) {
	t.Parallel()

	result := SummariseBatch(esferaBatch(), wilmara, 50, esferaSlices(), []string{wilmara, tiago})

	if got := signedSum(result.Parts, tiago, wilmara); got != 360640 {
		t.Errorf("parts add up to %d, want the settlement's 360640", got)
	}
	kinds := map[string]int{}
	for _, part := range result.Parts {
		kinds[part.Kind]++
	}
	// The cost tiago used, its profit share and wilmara's profit share; no fee on this batch.
	if kinds[PartCost] != 1 || kinds[PartProfitShare] != 2 || kinds[PartFeeShare] != 0 {
		t.Errorf("parts = %+v, want one cost and two profit shares", result.Parts)
	}
}

// Merging keeps one part per kind and direction, adds the bases and amounts, and stops
// claiming a percent once the batches disagree on it.
func TestMergePartsAddsUpAndFlagsAMixedPercent(t *testing.T) {
	t.Parallel()

	merged := MergeParts(
		[]SettlementPart{{Kind: PartProfitShare, PayerID: tiago, PayeeID: wilmara, BaseCents: 1000, Percent: 50, AmountCents: 500}},
		[]SettlementPart{{Kind: PartProfitShare, PayerID: tiago, PayeeID: wilmara, BaseCents: 2000, Percent: 30, AmountCents: 1400}},
		[]SettlementPart{{Kind: PartCost, PayerID: tiago, PayeeID: wilmara, BaseCents: 700, Percent: 100, AmountCents: 700}},
	)

	if len(merged) != 2 {
		t.Fatalf("got %d parts, want 2: %+v", len(merged), merged)
	}
	share := merged[0]
	if share.BaseCents != 3000 || share.AmountCents != 1900 || share.Percent != MixedPercent {
		t.Errorf("merged share = %+v, want base 3000, amount 1900, mixed percent", share)
	}
}

// Each agency's activity is added up across batches.
func TestSummariseAgenciesAddsConsumersAcrossBatches(t *testing.T) {
	t.Parallel()

	summaries := SummariseAgencies([]BatchResult{
		{Consumers: []Consumer{{AgencyID: tiago, Points: 10, ProfitCents: 100, RedemptionFeeSavedCents: 7}}},
		{Consumers: []Consumer{{AgencyID: tiago, Points: 5, ProfitCents: 50}, {AgencyID: wilmara, Points: 1}}},
	})

	if len(summaries) != 2 || summaries[0].Points != 15 || summaries[0].ProfitCents != 150 || summaries[0].RedemptionFeeSavedCents != 7 {
		t.Errorf("summaries = %+v, want tiago 15 points, 150 profit, 7 fee", summaries)
	}
}
