package statement

import (
	"fmt"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// LoadAzulStatement builds a Statement out of the synced azul_statement_lines of one account,
// the way ParseAzul used to build one out of an uploaded CSV. It is the bridge the plan
// promised: the reconciliation core (Reconcile) stays untouched and is now fed from the
// database instead of a file.
//
// The stored rows are already the sync's window (at most a month), so the whole of what is
// stored is the period - the caller takes it with PeriodOf, exactly as the file path did. No
// date arguments: bounding the window is the sync's job, not the reader's.
//
// The airline gives no usable transaction id (always 0), so identical rows on one day are
// legitimately distinct and every stored row becomes its own movement - no deduplication, the
// same reason ReplaceAzulStatementWindow stores them verbatim.
func LoadAzulStatement(db *gorm.DB, accountID string) (Statement, error) {
	var rows []data.AzulStatementLine
	if err := db.Where("account_id = ?", accountID).
		Order("occurred_on ASC, id ASC").
		Find(&rows).Error; err != nil {
		return Statement{}, fmt.Errorf("statement.LoadAzulStatement: %w", err)
	}

	movements := make([]Movement, 0, len(rows))
	for _, row := range rows {
		movements = append(movements, Movement{
			OccurredOn:  row.OccurredOn,
			Kind:        azulLineKind(row.Kind),
			Locator:     deref(row.Locator),
			Points:      row.Points,
			Description: deref(row.Description),
		})
	}
	return Statement{Movements: movements}, nil
}

// azulLineKind maps the stored kind onto the reconciliation's Kind. The three reconcilable
// kinds share their string values with data.AzulLine*, so they pass straight through;
// expiration and anything unrecognised map to the pass-through kinds that Reconcile counts in
// the net but does not act on.
func azulLineKind(stored string) Kind {
	switch stored {
	case data.AzulLineRedemption:
		return KindRedemption
	case data.AzulLineRefund:
		return KindRefund
	case data.AzulLineAccrual, data.AzulLinePurchase:
		// A purchase (which includes campaign/promotion credits) is a point entry that needs a cost
		// basis just like any accrual, so the reconciliation treats them the same - the split exists
		// only for the page's label.
		return KindAccrual
	case data.AzulLineExpiration:
		return KindExpiration
	default:
		return KindOther
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
