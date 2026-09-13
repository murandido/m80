package sale

import (
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A client pays the agency in lump sums, not per ticket. The payments screen takes the sum
// and spreads it over the client's open sales, oldest first, so at any moment a client has
// a run of settled sales, at most one partly paid, and the rest open. What no open sale is
// left to absorb becomes the client's credit (data.ClientCredit) and is spent on the next
// sale linked to them.
//
// Everything below writes ordinary sale payments, so the balance of each sale - and
// sales.paid, through Recalculate - is still the one rule in balance.go.

// ErrClientNotFound is returned when the client being paid for does not exist.
var ErrClientNotFound = errors.New("client not found")

const (
	notePaidByClient     = "Pagamento do cliente"
	noteSpentFromBalance = "Abatido do crédito do cliente"
	noteSurplusKept      = "Excedente de pagamento do cliente"
)

// ClientPaymentResult is what a lump sum did, for the operator's confirmation.
type ClientPaymentResult struct {
	// SaleIDs are every sale that received money, credit included - the rows to refresh.
	SaleIDs []string
	// AppliedCents is how much of the new money went into sales; the rest became credit.
	AppliedCents int
	// SettledSales counts the sales left with nothing to pay.
	SettledSales int
	// CreditCents is the client's credit once everything was spread.
	CreditCents int
}

// AllocateFIFO spreads pool over balances in order: each positive balance takes as much as
// it is missing until the pool runs out. Balances of zero or less take nothing. leftover is
// what no balance absorbed.
func AllocateFIFO(balances []int, pool int) (takes []int, leftover int) {
	takes = make([]int, len(balances))
	for i, balance := range balances {
		if pool <= 0 {
			break
		}
		if balance <= 0 {
			continue
		}
		take := min(balance, pool)
		takes[i] = take
		pool -= take
	}
	return takes, pool
}

type openSale struct {
	ID      string
	Balance int
}

// lockClient serializes everything that spreads money for one client: two operators
// registering a payment for the same client at once would otherwise both read the same
// open sales and pay the oldest one twice.
func lockClient(tx *gorm.DB, clientID string) error {
	var client data.Client
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Select("id").
		First(&client, "id = ?", clientID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrClientNotFound
	}
	return err
}

// openSalesOf lists the client's sales that still owe money, oldest issuance first - the
// order the money is spent in. Cancelled issuances are left out: a cancelled ticket is not
// a debt (see BalanceCents), and whatever it holds is for returning, not for moving along.
func openSalesOf(tx *gorm.DB, clientID string) ([]openSale, error) {
	var rows []struct {
		ID                 string
		Points             int
		ValueOfTheThousand int
		Taxes              *int
		PaidCents          int
	}
	err := tx.Table("sales").
		Select(`sales.id, issuances.points, issuances.value_of_the_thousand, issuances.taxes,
			COALESCE((SELECT SUM(amount_cents) FROM sale_payments WHERE sale_id = sales.id), 0) AS paid_cents`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Where("sales.client_id = ?", clientID).
		Where("issuances.deleted_at IS NULL AND issuances.cancelled_at IS NULL").
		Order("issuances.created_at, sales.id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	open := make([]openSale, 0, len(rows))
	for _, row := range rows {
		cents := PointsValueCents(row.Points, row.ValueOfTheThousand)
		if row.Taxes != nil {
			cents += *row.Taxes
		}
		if balance := BalanceCents(Amount{Cents: cents}, row.PaidCents); balance > 0 {
			open = append(open, openSale{ID: row.ID, Balance: balance})
		}
	}
	return open, nil
}

// CreditOf is the client's credit balance.
func CreditOf(tx *gorm.DB, clientID string) (int, error) {
	var total *int
	err := tx.Model(&data.ClientCredit{}).
		Where("client_id = ?", clientID).
		Select("SUM(amount_cents)").
		Scan(&total).Error
	if err != nil || total == nil {
		return 0, err
	}
	return *total, nil
}

// spread pays the open sales from pool, oldest first, and returns what was left. onPaid
// runs for each payment written, inside the same transaction.
func spread(tx *gorm.DB, open []openSale, pool int, paidOn time.Time, note, actorID string,
	onPaid func(saleID string, cents int) error,
) (result ClientPaymentResult, leftover int, err error) {
	balances := make([]int, len(open))
	for i, s := range open {
		balances[i] = s.Balance
	}
	takes, leftover := AllocateFIFO(balances, pool)

	for i, take := range takes {
		if take == 0 {
			continue
		}
		payment := data.SalePayment{
			SaleID:      open[i].ID,
			AmountCents: take,
			PaidOn:      paidOn,
			Note:        &note,
			CreatedByID: actorID,
		}
		if err := tx.Create(&payment).Error; err != nil {
			return result, 0, err
		}
		if err := Recalculate(tx, open[i].ID); err != nil {
			return result, 0, err
		}
		if onPaid != nil {
			if err := onPaid(open[i].ID, take); err != nil {
				return result, 0, err
			}
		}
		result.SaleIDs = append(result.SaleIDs, open[i].ID)
		result.AppliedCents += take
		if take == open[i].Balance {
			result.SettledSales++
		}
	}
	return result, leftover, nil
}

// ApplyClientCredit spends the client's credit on their open sales, oldest first. It is
// what makes a surplus "come off the next issuance": sales are born without a client, so
// linking one is the moment it can take the credit. A client with no credit costs one query.
func ApplyClientCredit(tx *gorm.DB, clientID, actorID string) (ClientPaymentResult, error) {
	credit, err := CreditOf(tx, clientID)
	if err != nil || credit <= 0 {
		return ClientPaymentResult{CreditCents: credit}, err
	}
	if err := lockClient(tx, clientID); err != nil {
		return ClientPaymentResult{}, err
	}
	// Read again under the lock: a concurrent spend may have used it up in between.
	if credit, err = CreditOf(tx, clientID); err != nil || credit <= 0 {
		return ClientPaymentResult{CreditCents: credit}, err
	}

	open, err := openSalesOf(tx, clientID)
	if err != nil {
		return ClientPaymentResult{}, err
	}

	result, leftover, err := spread(tx, open, credit, time.Now(), noteSpentFromBalance, actorID,
		func(saleID string, cents int) error {
			id := saleID
			note := noteSpentFromBalance
			return tx.Create(&data.ClientCredit{
				ClientID:    clientID,
				AmountCents: -cents,
				SaleID:      &id,
				Note:        &note,
				CreatedByID: actorID,
			}).Error
		})
	result.CreditCents = leftover
	return result, err
}

// ReceiveClientPayment takes a lump sum from a client and spreads it over their open sales,
// oldest first; what is left over becomes credit.
//
// Credit already held is spent first. It normally only exists when nothing is open, but an
// older sale switched back to unpaid breaks that, and spending new money around old credit
// would leave both sitting side by side.
func ReceiveClientPayment(tx *gorm.DB, clientID string, amountCents int, paidOn time.Time, actorID string) (ClientPaymentResult, error) {
	if err := lockClient(tx, clientID); err != nil {
		return ClientPaymentResult{}, err
	}

	fromCredit, err := ApplyClientCredit(tx, clientID, actorID)
	if err != nil {
		return ClientPaymentResult{}, err
	}

	open, err := openSalesOf(tx, clientID)
	if err != nil {
		return ClientPaymentResult{}, err
	}
	result, leftover, err := spread(tx, open, amountCents, paidOn, notePaidByClient, actorID, nil)
	if err != nil {
		return ClientPaymentResult{}, err
	}

	if leftover > 0 {
		note := noteSurplusKept
		if err := tx.Create(&data.ClientCredit{
			ClientID:    clientID,
			AmountCents: leftover,
			Note:        &note,
			CreatedByID: actorID,
		}).Error; err != nil {
			return ClientPaymentResult{}, err
		}
	}

	result.SaleIDs = mergeIDs(fromCredit.SaleIDs, result.SaleIDs)
	result.SettledSales += fromCredit.SettledSales
	if result.CreditCents, err = CreditOf(tx, clientID); err != nil {
		return ClientPaymentResult{}, err
	}
	return result, nil
}

// mergeIDs joins two lists of sale ids without repeating one: the sale left partly paid by
// the credit is the first one the new money reaches.
func mergeIDs(first, second []string) []string {
	seen := make(map[string]bool, len(first)+len(second))
	merged := make([]string, 0, len(first)+len(second))
	for _, id := range append(append([]string{}, first...), second...) {
		if !seen[id] {
			seen[id] = true
			merged = append(merged, id)
		}
	}
	return merged
}
