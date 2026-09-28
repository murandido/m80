package statement

import (
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// LoadSmilesStatement builds a Statement out of the synced smiles_statement_lines of one
// account, the way ParseSmiles used to build one out of an uploaded HTML page. It is the
// twin of LoadAzulStatement: the reconciliation core (Reconcile) stays untouched and is fed
// from the database instead of a file.
//
// The stored rows are already the sync's window (at most a month), so the whole of what is
// stored is the period - the caller takes it with PeriodOf, exactly as the file path did. No
// date arguments: bounding the window is the sync's job, not the reader's.
func LoadSmilesStatement(db *gorm.DB, accountID string) (Statement, error) {
	var rows []data.SmilesStatementLine
	if err := db.Where("account_id = ?", accountID).
		Order("occurred_on ASC, id ASC").
		Find(&rows).Error; err != nil {
		return Statement{}, fmt.Errorf("statement.LoadSmilesStatement: %w", err)
	}

	movements := make([]Movement, 0, len(rows))
	for _, row := range rows {
		movements = append(movements, Movement{
			OccurredOn:  row.OccurredOn,
			Kind:        smilesLineKind(row.Kind),
			Locator:     smilesReconcilableLocator(row),
			Points:      row.Points,
			Description: deref(row.Description),
		})
	}
	return Statement{Movements: movements}, nil
}

// smilesLineKind maps the stored kind onto the reconciliation's Kind. The reconcilable kinds
// share their string values with data.SmilesLine*, so they pass straight through; anything
// unrecognised maps to KindOther, which Reconcile counts in the net but does not act on.
func smilesLineKind(stored string) Kind {
	switch stored {
	case data.SmilesLineRedemption:
		return KindRedemption
	case data.SmilesLineRefund:
		return KindRefund
	case data.SmilesLineAccrual, data.SmilesLinePurchase:
		// A purchase is a miles entry that needs a cost basis just like any accrual, so the
		// reconciliation treats them the same - the split exists only for the page's label.
		return KindAccrual
	default:
		return KindOther
	}
}

// smilesReconcilableLocator is the booking reference the reconciliation may match on, which
// is not always the one the row carries.
//
// A flight credit has a locator too - miles earned on a flight, and the estorno of a
// cancelled redemption, both name the booking they came from. Handing that locator to
// Reconcile would pair a credit with the very booking it refers to and report the issuance
// as settled twice. The same call the HTML parser made, for the same reason.
//
// The refund keeps its locator, because matching a refund to its booking is exactly what the
// reconciliation is for.
func smilesReconcilableLocator(row data.SmilesStatementLine) string {
	if row.Kind == data.SmilesLineAccrual || row.Kind == data.SmilesLinePurchase {
		return ""
	}
	return deref(row.Locator)
}
