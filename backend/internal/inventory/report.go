package inventory

import (
	"sort"
	"time"
)

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
	// RedemptionFeeSavedCents is this slice's part of the redemption fee the booking did not
	// pay because the account is exempt (Azul's Diamante Unique). The issuing agency still
	// charged it to the client, so it is a gain on the account like the margin on the points,
	// and is split by the same percent. Prorated by points like the revenue, so a booking
	// that drew on two batches leaves part of the saving on each, and a reversal gives its
	// share back.
	RedemptionFeeSavedCents int

	// What the screen lists when a batch is opened: which booking or refund this slice is.
	IssuanceID     string
	ProgramLocator string
	OccurredOn     time.Time
}

// Consumer is one agency's activity against one batch.
type Consumer struct {
	AgencyID                string
	Points                  int
	CostCents               int
	RevenueCents            int
	ProfitCents             int
	RedemptionFeeSavedCents int
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

	// RedemptionFeeSavedCents is the redemption fee saved by every booking on this batch,
	// kept apart from the profit on the points so the screen can show each on its own.
	RedemptionFeeSavedCents int

	Consumers []Consumer
	// Settlements are on the points alone, exactly as the spreadsheet computed them.
	Settlements []Settlement
	// FeeSettlements split the redemption fee saved by the same percent, and
	// TotalSettlements are the two netted per pair of agencies - what is actually paid.
	FeeSettlements   []Settlement
	TotalSettlements []Settlement

	// Lines are the period's slices on this batch, in date order, so the screen can list the
	// bookings and refunds behind each figure.
	Lines []Slice

	// Parts are the terms the settlements above are added up from, one per term per pair of
	// agencies, so the screen can show the calculation and not only its result.
	Parts []SettlementPart
}

// The kinds of term a settlement is made of.
const (
	// PartCost is the cost of the points an agency used from a batch another agency paid for.
	PartCost = "cost"
	// PartProfitShare is the share of an agency's profit on the points that belongs to the
	// other side.
	PartProfitShare = "profit_share"
	// PartFeeShare is the share of the redemption fee an agency saved that belongs to the
	// other side.
	PartFeeShare = "fee_share"
)

// MixedPercent marks a merged part whose batches did not all use the same percent, so no
// single percent describes it.
const MixedPercent = -1

// SettlementPart is one term of a settlement: Payer owes Payee AmountCents, which is
// Percent% of BaseCents (the payer's own cost, profit or saved fee). AmountCents is exactly
// the value the settlement was summed from, rounding included, so the parts of a pair always
// add up to its settlement to the centavo.
type SettlementPart struct {
	Kind        string
	PayerID     string
	PayeeID     string
	BaseCents   int
	Percent     int
	AmountCents int
}

// MergeParts adds parts of the same kind between the same payer and payee together - across
// batches, or across accounts - keeping the order they first appeared in.
func MergeParts(lists ...[]SettlementPart) []SettlementPart {
	type key struct{ kind, payer, payee string }
	merged := map[key]*SettlementPart{}
	order := make([]key, 0)

	for _, list := range lists {
		for _, part := range list {
			k := key{kind: part.Kind, payer: part.PayerID, payee: part.PayeeID}
			existing, seen := merged[k]
			if !seen {
				copied := part
				merged[k] = &copied
				order = append(order, k)
				continue
			}
			existing.BaseCents += part.BaseCents
			existing.AmountCents += part.AmountCents
			if existing.Percent != part.Percent {
				existing.Percent = MixedPercent
			}
		}
	}

	parts := make([]SettlementPart, 0, len(order))
	for _, k := range order {
		parts = append(parts, *merged[k])
	}
	return parts
}

// SummariseAgencies adds each agency's activity up across batches: the points it issued,
// what they cost, what they sold for, the profit and the redemption fee it saved.
func SummariseAgencies(batches []BatchResult) []Consumer {
	byAgency := map[string]*Consumer{}
	order := make([]string, 0)
	for _, batch := range batches {
		for _, consumer := range batch.Consumers {
			total, seen := byAgency[consumer.AgencyID]
			if !seen {
				total = &Consumer{AgencyID: consumer.AgencyID}
				byAgency[consumer.AgencyID] = total
				order = append(order, consumer.AgencyID)
			}
			total.Points += consumer.Points
			total.CostCents += consumer.CostCents
			total.RevenueCents += consumer.RevenueCents
			total.ProfitCents += consumer.ProfitCents
			total.RedemptionFeeSavedCents += consumer.RedemptionFeeSavedCents
		}
	}
	summaries := make([]Consumer, 0, len(order))
	for _, agencyID := range order {
		summaries = append(summaries, *byAgency[agencyID])
	}
	return summaries
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
		consumer.RedemptionFeeSavedCents += slice.RedemptionFeeSavedCents

		result.ConsumedPoints += slice.Points
		result.ChargedCents += slice.CostCents
		result.RevenueCents += slice.RevenueCents
		result.RedemptionFeeSavedCents += slice.RedemptionFeeSavedCents
		result.Lines = append(result.Lines, slice)
	}

	sort.SliceStable(result.Lines, func(i, j int) bool {
		if !result.Lines[i].OccurredOn.Equal(result.Lines[j].OccurredOn) {
			return result.Lines[i].OccurredOn.Before(result.Lines[j].OccurredOn)
		}
		return result.Lines[i].ProgramLocator < result.Lines[j].ProgramLocator
	})

	result.ProfitCents = result.RevenueCents - result.ChargedCents
	result.RemainingPoints = batch.Points - result.ConsumedPoints

	funderProfit, funderFee := 0, 0
	if funder, ok := byAgency[fundingAgencyID]; ok {
		funder.ProfitCents = funder.RevenueCents - funder.CostCents
		funderProfit = funder.ProfitCents
		funderFee = funder.RedemptionFeeSavedCents
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

		spent, earned, fee := 0, 0, 0
		if consumer, ok := byAgency[agencyID]; ok {
			spent, earned, fee = consumer.CostCents, consumer.ProfitCents, consumer.RedemptionFeeSavedCents
		}

		// Each term is computed once and used both for the settlement and as a part, so the
		// calculation the screen shows is the one that was paid, rounding and all.
		earnedShare := shareOf(earned, 100-sharePercent)
		funderShare := shareOf(funderProfit, sharePercent)
		owed := spent + earnedShare - funderShare

		// The saved fee follows the profit's rule without the cost term: nobody paid for it,
		// so each side owes the other its share of what it kept.
		feeShare := shareOf(fee, 100-sharePercent)
		funderFeeShare := shareOf(funderFee, sharePercent)
		owedFee := feeShare - funderFeeShare

		addPart := func(kind, payer, payee string, base, percent, amount int) {
			if amount == 0 {
				return
			}
			result.Parts = append(result.Parts, SettlementPart{
				Kind: kind, PayerID: payer, PayeeID: payee,
				BaseCents: base, Percent: percent, AmountCents: amount,
			})
		}
		addPart(PartCost, agencyID, fundingAgencyID, spent, 100, spent)
		addPart(PartProfitShare, agencyID, fundingAgencyID, earned, 100-sharePercent, earnedShare)
		addPart(PartProfitShare, fundingAgencyID, agencyID, funderProfit, sharePercent, funderShare)
		addPart(PartFeeShare, agencyID, fundingAgencyID, fee, 100-sharePercent, feeShare)
		addPart(PartFeeShare, fundingAgencyID, agencyID, funderFee, sharePercent, funderFeeShare)

		// Nothing spent and nothing earned on either side is not a settlement, it is an
		// empty line. Dropping it keeps a report from listing agencies with nothing to do
		// with the batch.
		if owed != 0 {
			result.Settlements = append(result.Settlements, Settlement{
				AgencyID:    agencyID,
				ToAgencyID:  fundingAgencyID,
				AmountCents: owed,
			})
		}
		if owedFee != 0 {
			result.FeeSettlements = append(result.FeeSettlements, Settlement{
				AgencyID:    agencyID,
				ToAgencyID:  fundingAgencyID,
				AmountCents: owedFee,
			})
		}
	}

	result.TotalSettlements = NetSettlements(result.Settlements, result.FeeSettlements)
	return result
}

// NetSettlements adds settlements up per pair of agencies, whichever way round each one
// was written, and reports every pair once, as a positive amount owed in the direction it
// actually runs.
//
// "A owes B -100" and "B owes A 100" are the same debt. Keying on the ordered pair kept them
// apart, and a period whose batches were funded by both agencies came out as two lines in
// the same direction that had to be added up by hand.
func NetSettlements(lists ...[]Settlement) []Settlement {
	type pair struct{ low, high string }
	owed := map[pair]int{}
	order := make([]pair, 0)

	for _, list := range lists {
		for _, settlement := range list {
			key := pair{low: settlement.AgencyID, high: settlement.ToAgencyID}
			amount := settlement.AmountCents
			if key.high < key.low {
				key = pair{low: key.high, high: key.low}
				amount = -amount
			}
			if _, seen := owed[key]; !seen {
				order = append(order, key)
			}
			owed[key] += amount
		}
	}

	netted := make([]Settlement, 0, len(order))
	for _, key := range order {
		amount := owed[key]
		switch {
		case amount > 0:
			netted = append(netted, Settlement{AgencyID: key.low, ToAgencyID: key.high, AmountCents: amount})
		case amount < 0:
			netted = append(netted, Settlement{AgencyID: key.high, ToAgencyID: key.low, AmountCents: -amount})
		}
	}
	return netted
}

// shareOf is percent% of an amount, rounded half away from zero so a share of a loss is
// as large as the same share of the equivalent gain.
func shareOf(amountCents, percent int) int {
	if amountCents < 0 {
		return -proratedCents(-amountCents, percent, 100)
	}
	return proratedCents(amountCents, percent, 100)
}
