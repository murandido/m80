package inventory

import (
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// Closing is what an account's period closed at: one line per batch, plus the totals the
// settlement is actually paid on.
type Closing struct {
	AccountID string
	StartsOn  time.Time
	EndsOn    time.Time

	Batches []BatchResult
	// PartnerAgencyIDs are the agencies sharing this account. See UnsupportedSplit for why
	// the count matters.
	PartnerAgencyIDs []string
	// UnsupportedSplit is set when the account has more than two partner agencies, and the
	// settlement figures below must not be acted on.
	//
	// The split is one percent frozen on the batch, which says everything only while there
	// are two sides: "50" means half each. With three partners it does not say how the
	// other half divides, and the arithmetic silently hands the funder's share to whichever
	// agency happens to be iterated - a plausible number that is wrong.
	//
	// TODO: make the split explicit instead of derived, when an account with three or more
	// partners actually exists. The shape that would work:
	//
	//   - accounts gain a list of partner agencies with a percent each, summing to 100;
	//   - point_batches freeze that list at creation, the way profit_share_percent is
	//     frozen today, so changing the arrangement never rewrites a settled month;
	//   - SummariseBatch takes the frozen list rather than one percent, and owes each
	//     partner its own share of the batch's profit.
	//
	// Deliberately not built ahead of the need: the arrangement of a three-way account is
	// something to read off a real agreement, not to guess at now. Until then this flag
	// makes the gap loud rather than quiet.
	UnsupportedSplit bool
	// PendingPoints is what was issued against stock no batch covers yet. It is the one
	// number on this report that means "something is missing" rather than "here is the
	// figure" - the costing never blocks an issuance, so a shortfall shows up here
	// instead, and disappears by itself once the missing batch is entered.
	PendingPoints int

	CostCents    int
	RevenueCents int
	ProfitCents  int
	// Settlements are the per-agency totals across every batch, which is what one agency
	// actually transfers to the other. Netting them is the point: an agency can owe on
	// one batch and be owed on the next, and paying each line separately would be two
	// transfers where one will do. These are on the points alone.
	Settlements []Settlement

	// RedemptionFeeSavedCents is the redemption fee the period's bookings did not pay, and
	// FeeSettlements its split; TotalSettlements nets the two with the points - the figure
	// that is actually transferred. Kept as three so the screen can show each part.
	RedemptionFeeSavedCents int
	FeeSettlements          []Settlement
	TotalSettlements        []Settlement
}

// Close a period and you settle these figures. ReadClosing computes them without freezing
// anything, so the same numbers can be checked before the button is pressed.
func ReadClosing(db *gorm.DB, accountID string, startsOn, endsOn time.Time) (Closing, error) {
	closing := Closing{AccountID: accountID, StartsOn: startsOn, EndsOn: endsOn}

	var batchRows []data.PointBatch
	err := db.Where("account_id = ?", accountID).
		Order("acquired_on ASC").Order("created_at ASC").
		Find(&batchRows).Error
	if err != nil {
		return closing, fmt.Errorf("inventory.ReadClosing: loading batches: %w", err)
	}

	slices, pending, err := loadSlices(db, accountID, startsOn, endsOn)
	if err != nil {
		return closing, err
	}
	closing.PendingPoints = pending

	// The agencies sharing this account: everyone that funded a batch on it or issued
	// against it. They are derived rather than configured because that is what they are -
	// an agency with no batch and no booking here is not a partner in it. Each one gets a
	// settlement line on every batch, since the split is on the batch's whole profit.
	partners, err := loadPartnerAgencies(db, accountID)
	if err != nil {
		return closing, err
	}
	closing.PartnerAgencyIDs = partners
	closing.UnsupportedSplit = len(partners) > 2

	// Netted across batches, so the report ends with one figure per pair of agencies
	// rather than a column of transfers that cancel each other out.
	var pointSettlements, feeSettlements []Settlement

	for _, row := range batchRows {
		result := SummariseBatch(
			Batch{ID: row.ID, AcquiredOn: row.AcquiredOn, Points: row.Points, CostCents: row.CostCents},
			row.FundingAgencyID,
			row.ProfitSharePercent,
			slices,
			partners,
		)
		// A batch nothing drew on in this period has no line here. It is not finished -
		// it keeps its points for the next one - it simply did nothing this month.
		if result.ConsumedPoints == 0 {
			continue
		}

		closing.Batches = append(closing.Batches, result)
		closing.CostCents += result.ChargedCents
		closing.RevenueCents += result.RevenueCents
		closing.RedemptionFeeSavedCents += result.RedemptionFeeSavedCents
		pointSettlements = append(pointSettlements, result.Settlements...)
		feeSettlements = append(feeSettlements, result.FeeSettlements...)
	}

	closing.ProfitCents = closing.RevenueCents - closing.CostCents
	closing.Settlements = NetSettlements(pointSettlements)
	closing.FeeSettlements = NetSettlements(feeSettlements)
	closing.TotalSettlements = NetSettlements(pointSettlements, feeSettlements)

	return closing, nil
}

// loadSlices reads the allocations of a period with the sale attached, and separately the
// points no batch covered.
//
// The revenue is the issuance's own value_of_the_thousand, applied to the slice rather
// than to the whole booking: a booking split across two batches earned part of its margin
// against each of them, and the spreadsheet wrote it exactly that way.
func loadSlices(db *gorm.DB, accountID string, startsOn, endsOn time.Time) ([]Slice, int, error) {
	type row struct {
		BatchID                 *string
		AgencyID                string
		Points                  int
		CostCents               int
		RevenueCents            int
		RedemptionFeeSavedCents int
		IssuanceID              string
		ProgramLocator          string
		OccurredOn              time.Time
	}
	var rows []row

	err := db.Table("point_batch_allocations").
		Select(`point_batch_allocations.batch_id,
		        issuances.agency_id,
		        point_batch_allocations.points,
		        point_batch_allocations.cost_cents,
		        -- points/1000 * the sale rate, rounded half away from zero so a reversal
		        -- gives back exactly what its booking earned.
		        --
		        -- The cast to numeric comes first, and has to: both columns are 32-bit
		        -- integers, and a single booking of 2.254.000 points at R$ 15,00 the
		        -- thousand multiplies to 3.381.000.000 before the division ever runs -
		        -- past what an integer holds, and Postgres refuses the whole query. It
		        -- stayed hidden while every slice was bounded by the batch it drew on;
		        -- an account left with no batches makes each booking one whole slice.
		        CAST(ROUND(point_batch_allocations.points::numeric * issuances.value_of_the_thousand / 1000.0) AS INTEGER) AS revenue_cents,
		        -- The booking's saved redemption fee, prorated by the slice's share of its
		        -- points - numeric first for the same overflow reason as above. Zero when the
		        -- account is not exempt or the fee was never worked out.
		        CAST(COALESCE(ROUND(point_batch_allocations.points::numeric * issuances.redemption_fee_saved_cents / NULLIF(issuances.points, 0)), 0) AS INTEGER) AS redemption_fee_saved_cents,
		        point_batch_allocations.issuance_id,
		        issuances.program_locator,
		        point_batch_allocations.occurred_on`).
		Joins("JOIN issuances ON issuances.id = point_batch_allocations.issuance_id").
		Where("issuances.account_id = ?", accountID).
		Where("point_batch_allocations.occurred_on >= ? AND point_batch_allocations.occurred_on <= ?", startsOn, endsOn).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, fmt.Errorf("inventory.ReadClosing: loading allocations: %w", err)
	}

	slices := make([]Slice, 0, len(rows))
	pending := 0
	for _, r := range rows {
		if r.BatchID == nil {
			pending += r.Points
			continue
		}
		slices = append(slices, Slice{
			BatchID:                 *r.BatchID,
			AgencyID:                r.AgencyID,
			Points:                  r.Points,
			CostCents:               r.CostCents,
			RevenueCents:            r.RevenueCents,
			RedemptionFeeSavedCents: r.RedemptionFeeSavedCents,
			IssuanceID:              r.IssuanceID,
			ProgramLocator:          r.ProgramLocator,
			OccurredOn:              r.OccurredOn,
		})
	}
	return slices, pending, nil
}

// loadPartnerAgencies lists the agencies sharing an account: those that funded a batch on
// it, and those that issued against it. Sorted so the report is stable.
func loadPartnerAgencies(db *gorm.DB, accountID string) ([]string, error) {
	var ids []string
	err := db.Raw(`
		SELECT DISTINCT agency_id FROM (
			SELECT funding_agency_id AS agency_id FROM point_batches
			 WHERE account_id = ? AND deleted_at IS NULL
			UNION
			SELECT agency_id FROM issuances
			 WHERE account_id = ? AND deleted_at IS NULL
		) AS partners
		ORDER BY agency_id`, accountID, accountID).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("inventory.ReadClosing: loading partner agencies: %w", err)
	}
	return ids, nil
}
