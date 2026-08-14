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

// Amount is what a sale is worth and whether the issuance behind it was cancelled - the two
// facts BalanceCents needs.
type Amount struct {
	Cents     int
	Cancelled bool
}

// AmountOf reads a sale's price from the issuance behind it.
func AmountOf(db *gorm.DB, saleID string) (Amount, error) {
	var row struct {
		Points             int
		ValueOfTheThousand int
		Taxes              *int
		CancelledAt        *string
	}
	err := db.Table("sales").
		Select("issuances.points, issuances.value_of_the_thousand, issuances.taxes, issuances.cancelled_at").
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Where("sales.id = ?", saleID).
		Scan(&row).Error
	if err != nil {
		return Amount{}, err
	}

	cents := PointsValueCents(row.Points, row.ValueOfTheThousand)
	if row.Taxes != nil {
		cents += *row.Taxes
	}
	return Amount{Cents: cents, Cancelled: row.CancelledAt != nil}, nil
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
// A cancelled sale is owed nothing. The ticket is gone, so its price stopped being a debt
// the moment it was cancelled - what survives is whatever the client already handed over,
// which now has to go back. Without this, a cancelled sale nobody ever paid read as owing
// its full price forever, and one that was paid and then refunded read the same, when in
// both cases there is nothing left to do.
func BalanceCents(amount Amount, paidCents int) int {
	if amount.Cancelled {
		return -paidCents
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
func RecalculateByIssuance(tx *gorm.DB, issuanceID string) error {
	var saleID string
	err := tx.Model(&data.Sale{}).
		Where("issuance_id = ?", issuanceID).
		Limit(1).
		Pluck("id", &saleID).Error
	if err != nil || saleID == "" {
		return err
	}
	return Recalculate(tx, saleID)
}
