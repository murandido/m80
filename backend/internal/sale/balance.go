// Package sale holds what a sale is worth and how much of it has been settled - the rules
// shared by the panel's handlers, which read them for the screen, and by the cancellation
// flow in internal/issuance, which changes what is owed and therefore has to recompute
// them. The bot cancels through that same flow, so this is the second rule (after issuance
// creation) that two processes need and neither owns.
//
// It deliberately knows nothing about HTTP: callers turn its numbers into a response, a
// spreadsheet cell or a WhatsApp reply themselves.
package sale

import (
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// PointsValueCents converts miles to money at a given price per thousand, rounding up to
// the cent. Rounding up (rather than truncating) matches the ROUNDUP the export
// spreadsheet applies, so the two never disagree by a cent on the same issuance.
func PointsValueCents(points, valueOfTheThousand int) int {
	total := points * valueOfTheThousand
	if total <= 0 {
		return total / 1000
	}
	return (total + 999) / 1000
}

// AdjustmentClientCents is what one alteration adds to the sale: its points at the rate the
// client pays for them, plus its taxes when those were passed on. Negative when the client
// gets money back.
//
// The points are priced by magnitude and the sign put back on afterwards, so a refund
// rounds up to the client's favour the same way a charge rounds up to the agency's.
// PointsValueCents alone would truncate a negative product toward zero.
func AdjustmentClientCents(points, valueOfTheThousand, taxes int, taxesPassedToClient bool) int {
	cents := PointsValueCents(points, valueOfTheThousand)
	if points < 0 {
		cents = -PointsValueCents(-points, valueOfTheThousand)
	}
	if taxesPassedToClient {
		cents += taxes
	}
	return cents
}

// AdjustmentCentsJoin is AdjustmentClientCents for every issuance at once, as a subquery
// keyed by issuance_id with the total in `cents`. The list queries join it rather than
// loading the alterations row by row. It has to stay the same arithmetic as the Go
// function above, and TestAdjustmentCentsJoinMatchesTheGoFunction holds the two together.
const AdjustmentCentsJoin = `(
	SELECT issuance_id,
		SUM(SIGN(points) * CEIL(ABS(points)::numeric * value_of_the_thousand / 1000)
			+ CASE WHEN taxes_passed_to_client THEN taxes ELSE 0 END)::integer AS cents
	FROM issuance_adjustments
	WHERE deleted_at IS NULL
	GROUP BY issuance_id
)`

// Amount is what a sale is worth and whether the issuance behind it was cancelled - the two
// facts BalanceCents needs.
type Amount struct {
	Cents     int
	Cancelled bool
	// AdjustmentCents is the part of Cents the ticket's alterations added, so a caller can
	// tell what was owed for the booking alone.
	AdjustmentCents int
	// CancellationFeeCents is the airline's cancellation fee, which the client always pays.
	// It is what a cancelled sale still owes. Zero on a live sale and on a cancellation with
	// no fee.
	CancellationFeeCents int
	// PriceAdjustmentCents is the part of Cents typed in the payments control on top of (or
	// below) the computed price - sales.price_adjustment_cents. Zero on a cancelled sale,
	// whose price is no longer owed.
	PriceAdjustmentCents int
}

// AmountOf reads a sale's price from the issuance behind it, alterations included.
func AmountOf(db *gorm.DB, saleID string) (Amount, error) {
	var row struct {
		Points               int
		ValueOfTheThousand   int
		Taxes                *int
		AdjustmentCents      int
		CancelledAt          *string
		CancellationFee      *int
		PriceAdjustmentCents int
	}
	err := db.Table("sales").
		Select(`issuances.points, issuances.value_of_the_thousand, issuances.taxes, issuances.cancelled_at,
			issuances.cancellation_fee, sales.price_adjustment_cents,
			COALESCE(adjustments.cents, 0) AS adjustment_cents`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Joins("LEFT JOIN "+AdjustmentCentsJoin+" AS adjustments ON adjustments.issuance_id = issuances.id").
		Where("sales.id = ?", saleID).
		Scan(&row).Error
	if err != nil {
		return Amount{}, err
	}

	cents := PointsValueCents(row.Points, row.ValueOfTheThousand) + row.AdjustmentCents
	if row.Taxes != nil {
		cents += *row.Taxes
	}
	amount := Amount{Cents: cents, Cancelled: row.CancelledAt != nil, AdjustmentCents: row.AdjustmentCents}
	if amount.Cancelled && row.CancellationFee != nil {
		amount.CancellationFeeCents = *row.CancellationFee
	}
	if !amount.Cancelled {
		amount.Cents += row.PriceAdjustmentCents
		amount.PriceAdjustmentCents = row.PriceAdjustmentCents
	}
	return amount, nil
}

// PaidCents is the sum of a sale's payments. Negative entries are refunds, so this can go
// down as well as up, and past zero.
func PaidCents(db *gorm.DB, saleID string) (int, error) {
	var total *int
	err := db.Model(&data.SalePayment{}).
		Where("sale_id = ?", saleID).
		Select("SUM(amount_cents)").
		Scan(&total).Error
	if err != nil || total == nil {
		return 0, err
	}
	return *total, nil
}

// BalanceCents is the money still to move on a sale, signed: positive is owed by the
// client, negative is owed back to them, zero is nothing left to do.
//
// A cancelled sale is owed its cancellation fee and nothing else. The ticket is gone, so its
// price stopped being a debt the moment it was cancelled - what survives is the fee, which
// the client always pays, against whatever the client already handed over. Paid more than
// the fee, the rest goes back; paid less, the client still owes the difference. Without the
// fee this is "give back everything that came in", which is what it was before the fee was
// counted: a cancelled sale nobody ever paid read as owing its full price forever, and one
// that was paid and then refunded read the same.
func BalanceCents(amount Amount, paidCents int) int {
	if amount.Cancelled {
		return amount.CancellationFeeCents - paidCents
	}
	return amount.Cents - paidCents
}

// Settled reports whether a sale needs anybody to act on it. It is what sales.paid caches
// and what the ?paid= filter separates on.
//
// The one exception is a live sale worth nothing, which is an issuance still missing its
// points or its price. Its balance is zero by arithmetic, not because it is settled, and
// calling it settled would hide the row that most needs filling in.
func Settled(amount Amount, paidCents int) bool {
	return BalanceCents(amount, paidCents) == 0 && (amount.Cancelled || amount.Cents > 0)
}

// Recalculate brings sales.paid back in line with the ledger and the issuance's state.
// Every write that can change either - a payment, a cancellation, a reverted cancellation -
// ends here, inside the same transaction, so the cache and its source can never be observed
// disagreeing.
//
// The column survives because the API image can be rolled back independently of the schema:
// the previous binary reads sales.paid and stays right as long as this keeps it true.
func Recalculate(tx *gorm.DB, saleID string) error {
	amount, err := AmountOf(tx, saleID)
	if err != nil {
		return err
	}
	paidCents, err := PaidCents(tx, saleID)
	if err != nil {
		return err
	}

	// UpdateColumn, not Update: this is bookkeeping derived from another table, and bumping
	// sales.updated_at would make it look like somebody edited the sale.
	return tx.Model(&data.Sale{}).Where("id = ?", saleID).
		UpdateColumn("paid", Settled(amount, paidCents)).Error
}

// RecalculateByIssuance is the same, for the callers that hold an issuance rather than a
// sale - the cancellation flow, which changes what is owed without touching the ledger.
// A missing sale is not an error: an issuance whose sale was deleted has nothing to update.
//
// A sale with a client also has the client's payments replayed (see RebalanceClient):
// what a lump sum had put on a ticket that is now cancelled is the client's money again,
// and goes to their next open sale or to their credit instead of waiting to be handed
// back; a restored ticket is a debt again and takes its turn in the queue. The ledger
// lines recorded on the sale itself are still left alone, but what they paid beyond the
// fee is surplus, and the replay carries it to the client's other sales or credit too.
func RecalculateByIssuance(tx *gorm.DB, issuanceID string) error {
	var found data.Sale
	err := tx.Select("id", "client_id").
		Where("issuance_id = ?", issuanceID).
		Limit(1).
		Find(&found).Error
	if err != nil || found.ID == "" {
		return err
	}
	if err := Recalculate(tx, found.ID); err != nil {
		return err
	}
	if found.ClientID == nil {
		return nil
	}
	_, err = RebalanceClient(tx, *found.ClientID)
	return err
}
