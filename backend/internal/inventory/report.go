package inventory

// This file turns the allocations into the two figures the month is closed on: what each
// batch earned, and what one agency owes the other for it.
//
// Both come straight from the spreadsheet's right-hand block. The profit is its
// "LUCRO TOTAL" - revenue minus cost, per batch - and the settlement is the SUMIFS
// expression in its "TIAGO (A PAGAR)" column, generalised so the split is not hard-coded
// at half.

// Slice is one allocation with the sale attached: what it cost, and what the booking that
// consumed it sold for.
type Slice struct {
	BatchID string
	// AgencyID is the agency that issued the booking, which is not necessarily the one
	// that funded the batch - that difference is the entire reason a settlement exists.
	AgencyID     string
	Points       int
	CostCents    int
	RevenueCents int
}

// Consumer is one agency's activity against one batch.
type Consumer struct {
	AgencyID     string
	Points       int
	CostCents    int
	RevenueCents int
	ProfitCents  int
}

// Settlement is one agency owing the batch's funder. A negative amount is the funder
// owing them instead, which happens when the funder's own bookings made the profit that
// the other agency has a share of.
type Settlement struct {
	AgencyID    string
	ToAgencyID  string
	AmountCents int
}

// BatchResult is one batch's line in the closing report.
type BatchResult struct {
	BatchID            string
	FundingAgencyID    string
	ProfitSharePercent int

	Points    int
	CostCents int

	ConsumedPoints  int
	ChargedCents    int
	RevenueCents    int
	ProfitCents     int
	RemainingPoints int

	Consumers   []Consumer
	Settlements []Settlement
}

// SummariseBatch folds one batch's slices into its closing line.
//
// The settlement follows the spreadsheet exactly, with the split read from the batch
// instead of being fixed at half. For a batch funded by F, every other agency A owes:
//
//	the cost of the points A consumed        - F paid for them
//	+ F's share of the profit A made         - (100 - share)%
//	- A's share of the profit F made         - share%
//
// The third term is why `partners` exists and why it is not simply "the agencies that
// consumed this batch". The split is on the batch's whole profit, so an agency that took
// nothing from it is still owed its share of what the funder earned on it - the spreadsheet
// computes exactly that, and a batch the funder used alone comes out as the funder owing,
// not as nothing to settle. Reading the partners off this batch's own slices would silently
// drop every one of those.
//
// A share of zero leaves the profit whole with whoever funded the batch, which is the right
// answer for the accounts that have no arrangement at all: every term collapses to the cost
// of the points the other agency spent.
func SummariseBatch(batch Batch, fundingAgencyID string, sharePercent int, slices []Slice, partners []string) BatchResult {
	result := BatchResult{
		BatchID:            batch.ID,
		FundingAgencyID:    fundingAgencyID,
		ProfitSharePercent: sharePercent,
		Points:             batch.Points,
		CostCents:          batch.CostCents,
	}

	// Insertion-ordered, so the report reads the same way twice. Ranging over a map alone
	// would shuffle the consumers between identical requests.
	byAgency := map[string]*Consumer{}
	order := make([]string, 0)

	for _, slice := range slices {
		if slice.BatchID != batch.ID {
			continue
		}
		consumer, seen := byAgency[slice.AgencyID]
		if !seen {
			consumer = &Consumer{AgencyID: slice.AgencyID}
			byAgency[slice.AgencyID] = consumer
			order = append(order, slice.AgencyID)
		}
		consumer.Points += slice.Points
		consumer.CostCents += slice.CostCents
		consumer.RevenueCents += slice.RevenueCents

		result.ConsumedPoints += slice.Points
		result.ChargedCents += slice.CostCents
		result.RevenueCents += slice.RevenueCents
	}

	result.ProfitCents = result.RevenueCents - result.ChargedCents
	result.RemainingPoints = batch.Points - result.ConsumedPoints

	funderProfit := 0
	if funder, ok := byAgency[fundingAgencyID]; ok {
		funder.ProfitCents = funder.RevenueCents - funder.CostCents
		funderProfit = funder.ProfitCents
	}

	for _, agencyID := range order {
		consumer := byAgency[agencyID]
		consumer.ProfitCents = consumer.RevenueCents - consumer.CostCents
		result.Consumers = append(result.Consumers, *consumer)
	}

	// Every partner gets a line, whether or not it touched this batch - see the note on
	// the third term above. Ordered by the caller's list so the report reads the same twice.
	for _, agencyID := range partners {
		if agencyID == fundingAgencyID {
			continue
		}

		spent, earned := 0, 0
		if consumer, ok := byAgency[agencyID]; ok {
			spent, earned = consumer.CostCents, consumer.ProfitCents
		}

		owed := spent +
			shareOf(earned, 100-sharePercent) -
			shareOf(funderProfit, sharePercent)

		// Nothing spent and nothing earned on either side is not a settlement, it is an
		// empty line. Dropping it keeps a report from listing agencies with nothing to do
		// with the batch.
		if owed == 0 {
			continue
		}

		result.Settlements = append(result.Settlements, Settlement{
			AgencyID:    agencyID,
			ToAgencyID:  fundingAgencyID,
			AmountCents: owed,
		})
	}

	return result
}

// shareOf is percent% of an amount, rounded half away from zero so a share of a loss is
// as large as the same share of the equivalent gain.
func shareOf(amountCents, percent int) int {
	if amountCents < 0 {
		return -proratedCents(-amountCents, percent, 100)
	}
	return proratedCents(amountCents, percent, 100)
}
