package sale

import (
	"errors"
	"sort"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A client pays the agency in lump sums, not per ticket. Each lump sum is a
// data.ClientPayment, and how they cover the client's sales is not stored as a decision:
// RebalanceClient derives it every time, replaying the active payments in the order they
// came in over the open sales, oldest first. The sale_payments it writes carry the lump
// sum's id and are the only lines it ever touches - a payment recorded on the sale itself
// (the Pago switch) is left alone and simply counts as already paid.
//
// Deriving instead of recording is what makes a wrong amount undoable: undoing the
// payment, or moving a sale to another client, is one more replay, and every sale ends
// up where it would have been had the mistake never happened. What no open sale absorbs
// is the client's credit, which is therefore a difference and never a stored balance.

// ErrClientNotFound is returned when the client being paid for does not exist.
var ErrClientNotFound = errors.New("client not found")

// ErrClientPaymentNotFound is returned when undoing a payment that does not exist or was
// already undone.
var ErrClientPaymentNotFound = errors.New("client payment not found")

const notePaidByClient = "Pagamento do cliente"

// ClientPaymentResult is what a change to a client's payments did, for the confirmation
// and for refreshing the rows on screen.
type ClientPaymentResult struct {
	// ClientPaymentID is the payment just recorded, empty for anything else.
	ClientPaymentID string
	// SaleIDs are every sale whose spread lines changed - the rows to refresh.
	SaleIDs []string
	// AppliedCents is how much of the payment just recorded went into sales; the rest
	// became credit.
	AppliedCents int
	// SettledSales counts the sales that payment left with nothing to pay.
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

// lockClient serializes everything that spreads money for one client: two operators
// changing the same client's payments at once would otherwise both replay from the same
// picture and write the spread twice.
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

type openSale struct {
	ID      string
	Balance int
}

// openSalesOf lists the client's sales that still owe money once the spread lines are set
// aside, oldest issuance first - the order the money is spent in. A cancelled issuance owes
// only its cancellation fee (see BalanceCents): a replay takes back whatever a lump sum had
// put on it beyond the fee, and that goes on to the next sale, or to credit. One cancelled
// with no fee is left out entirely.
func openSalesOf(tx *gorm.DB, clientID string) ([]openSale, error) {
	var rows []struct {
		ID                   string
		Points               int
		ValueOfTheThousand   int
		Taxes                *int
		AdjustmentCents      int
		CancelledAt          *string
		CancellationFee      *int
		PriceAdjustmentCents int
		PaidCents            int
	}
	err := tx.Table("sales").
		Select(`sales.id, issuances.points, issuances.value_of_the_thousand, issuances.taxes,
			issuances.cancelled_at, issuances.cancellation_fee, sales.price_adjustment_cents,
			COALESCE(adjustments.cents, 0) AS adjustment_cents,
			COALESCE((SELECT SUM(amount_cents) FROM sale_payments
				WHERE sale_id = sales.id AND client_payment_id IS NULL), 0) AS paid_cents`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Joins("LEFT JOIN "+AdjustmentCentsJoin+" AS adjustments ON adjustments.issuance_id = issuances.id").
		Where("sales.client_id = ?", clientID).
		// A cancelled ticket is a debt only for its fee, so it stays in the queue when it has
		// one; without a fee there is nothing for a lump sum to cover.
		Where("issuances.deleted_at IS NULL").
		Where("issuances.cancelled_at IS NULL OR COALESCE(issuances.cancellation_fee, 0) > 0").
		Order("issuances.created_at, sales.id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	open := make([]openSale, 0, len(rows))
	for _, row := range rows {
		cents := PointsValueCents(row.Points, row.ValueOfTheThousand) + row.AdjustmentCents
		if row.Taxes != nil {
			cents += *row.Taxes
		}
		amount := Amount{Cents: cents, Cancelled: row.CancelledAt != nil}
		if amount.Cancelled && row.CancellationFee != nil {
			amount.CancellationFeeCents = *row.CancellationFee
		}
		if !amount.Cancelled {
			amount.Cents += row.PriceAdjustmentCents
		}
		if balance := BalanceCents(amount, row.PaidCents); balance > 0 {
			open = append(open, openSale{ID: row.ID, Balance: balance})
		}
	}
	return open, nil
}

func activePaymentsOf(tx *gorm.DB, clientID string) ([]data.ClientPayment, error) {
	var payments []data.ClientPayment
	err := tx.Where("client_id = ? AND deleted_at IS NULL", clientID).
		Order("paid_on, created_at, id").
		Find(&payments).Error
	return payments, err
}

// CreditOf is the client's credit: what their active payments brought in minus what was
// spread over sales. Right after a RebalanceClient it is exactly what no open sale took.
func CreditOf(tx *gorm.DB, clientID string) (int, error) {
	var credit int
	err := tx.Raw(`
		SELECT
			COALESCE((SELECT SUM(amount_cents) FROM client_payments
				WHERE client_id = ? AND deleted_at IS NULL), 0)
			- COALESCE((SELECT SUM(sp.amount_cents) FROM sale_payments sp
				JOIN client_payments cp ON cp.id = sp.client_payment_id
				WHERE cp.client_id = ? AND cp.deleted_at IS NULL), 0)`,
		clientID, clientID).Scan(&credit).Error
	return credit, err
}

// CreditOfClients is CreditOf summed over several clients, in one query - the history of a
// whole agency would otherwise cost a round trip per client.
func CreditOfClients(tx *gorm.DB, clientIDs []string) (int, error) {
	if len(clientIDs) == 0 {
		return 0, nil
	}
	var credit int
	err := tx.Raw(`
		SELECT
			COALESCE((SELECT SUM(amount_cents) FROM client_payments
				WHERE client_id IN ? AND deleted_at IS NULL), 0)
			- COALESCE((SELECT SUM(sp.amount_cents) FROM sale_payments sp
				JOIN client_payments cp ON cp.id = sp.client_payment_id
				WHERE cp.client_id IN ? AND cp.deleted_at IS NULL), 0)`,
		clientIDs, clientIDs).Scan(&credit).Error
	return credit, err
}

type spreadKey struct {
	clientPaymentID string
	saleID          string
}

// RebalanceClient replays the client's active payments over their open sales and writes
// only what changed, so an action that moves nothing (linking a sale to a client with no
// payments) costs a few reads and touches no row. It returns the sales whose lines changed.
//
// Lines of undone payments are always removed: they are spread lines like any other, only
// with nothing left to replay them from.
func RebalanceClient(tx *gorm.DB, clientID string) ([]string, error) {
	if err := lockClient(tx, clientID); err != nil {
		return nil, err
	}

	payments, err := activePaymentsOf(tx, clientID)
	if err != nil {
		return nil, err
	}
	open, err := openSalesOf(tx, clientID)
	if err != nil {
		return nil, err
	}

	// What the lines should be: each payment, in the order it came in, takes from what the
	// sales still miss after the payments before it.
	balances := make([]int, len(open))
	for i, s := range open {
		balances[i] = s.Balance
	}
	want := map[spreadKey]int{}
	for _, payment := range payments {
		takes, _ := AllocateFIFO(balances, payment.AmountCents)
		for i, take := range takes {
			if take == 0 {
				continue
			}
			balances[i] -= take
			want[spreadKey{payment.ID, open[i].ID}] = take
		}
	}

	// What the lines are, across every payment of this client (undone ones included) - a
	// sale that left the client still has its lines here, and loses them below.
	var existing []data.SalePayment
	err = tx.Joins("JOIN client_payments cp ON cp.id = sale_payments.client_payment_id").
		Where("cp.client_id = ?", clientID).
		Find(&existing).Error
	if err != nil {
		return nil, err
	}

	touched := map[string]bool{}
	for _, line := range existing {
		key := spreadKey{*line.ClientPaymentID, line.SaleID}
		if cents, ok := want[key]; ok && cents == line.AmountCents {
			delete(want, key)
			continue
		}
		if err := tx.Delete(&line).Error; err != nil {
			return nil, err
		}
		touched[line.SaleID] = true
	}

	byID := make(map[string]data.ClientPayment, len(payments))
	for _, payment := range payments {
		byID[payment.ID] = payment
	}
	note := notePaidByClient
	for key, cents := range want {
		payment := byID[key.clientPaymentID]
		// The line is the client's payment seen from one sale, so it carries that payment's
		// day and author - not whoever's action happened to trigger this replay.
		line := data.SalePayment{
			SaleID:          key.saleID,
			AmountCents:     cents,
			PaidOn:          payment.PaidOn,
			Note:            &note,
			ClientPaymentID: &payment.ID,
			CreatedByID:     payment.CreatedByID,
		}
		if err := tx.Create(&line).Error; err != nil {
			return nil, err
		}
		touched[key.saleID] = true
	}

	saleIDs := make([]string, 0, len(touched))
	for saleID := range touched {
		saleIDs = append(saleIDs, saleID)
	}
	// Map order is random; sorted, the result - and the order of the broadcasts - is stable.
	sort.Strings(saleIDs)
	for _, saleID := range saleIDs {
		if err := Recalculate(tx, saleID); err != nil {
			return nil, err
		}
	}
	return saleIDs, nil
}

// ReceiveClientPayment records a lump sum from a client and spreads it.
func ReceiveClientPayment(tx *gorm.DB, clientID string, amountCents int, paidOn time.Time, actorID string) (ClientPaymentResult, error) {
	if err := lockClient(tx, clientID); err != nil {
		return ClientPaymentResult{}, err
	}

	payment := data.ClientPayment{
		ClientID:    clientID,
		AmountCents: amountCents,
		PaidOn:      paidOn,
		CreatedByID: actorID,
	}
	if err := tx.Create(&payment).Error; err != nil {
		return ClientPaymentResult{}, err
	}

	saleIDs, err := RebalanceClient(tx, clientID)
	if err != nil {
		return ClientPaymentResult{}, err
	}
	result := ClientPaymentResult{ClientPaymentID: payment.ID, SaleIDs: saleIDs}

	// What this payment did, read back from the lines the replay left for it. A payment
	// dated before older ones can also shift money between other sales; those are in
	// SaleIDs, but only this payment's own lines are "abatido" in the confirmation.
	var lines []struct {
		AmountCents int
		Paid        bool
	}
	err = tx.Table("sale_payments").
		Select("sale_payments.amount_cents, sales.paid").
		Joins("JOIN sales ON sales.id = sale_payments.sale_id").
		Where("sale_payments.client_payment_id = ?", payment.ID).
		Scan(&lines).Error
	if err != nil {
		return ClientPaymentResult{}, err
	}
	for _, line := range lines {
		result.AppliedCents += line.AmountCents
		if line.Paid {
			result.SettledSales++
		}
	}

	if result.CreditCents, err = CreditOf(tx, clientID); err != nil {
		return ClientPaymentResult{}, err
	}
	return result, nil
}

// UndoClientPayment marks a lump sum as undone and replays the rest. The row stays, so the
// history still shows what was typed and who took it back.
func UndoClientPayment(tx *gorm.DB, paymentID, actorID string) (ClientPaymentResult, error) {
	var payment data.ClientPayment
	err := tx.First(&payment, "id = ? AND deleted_at IS NULL", paymentID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ClientPaymentResult{}, ErrClientPaymentNotFound
	}
	if err != nil {
		return ClientPaymentResult{}, err
	}

	if err := lockClient(tx, payment.ClientID); err != nil {
		return ClientPaymentResult{}, err
	}
	// Under the lock, so two operators undoing the same payment cannot both succeed.
	res := tx.Model(&data.ClientPayment{}).
		Where("id = ? AND deleted_at IS NULL", paymentID).
		Updates(map[string]any{"deleted_at": time.Now(), "deleted_by_id": actorID})
	if res.Error != nil {
		return ClientPaymentResult{}, res.Error
	}
	if res.RowsAffected == 0 {
		return ClientPaymentResult{}, ErrClientPaymentNotFound
	}

	saleIDs, err := RebalanceClient(tx, payment.ClientID)
	if err != nil {
		return ClientPaymentResult{}, err
	}
	credit, err := CreditOf(tx, payment.ClientID)
	if err != nil {
		return ClientPaymentResult{}, err
	}
	return ClientPaymentResult{SaleIDs: saleIDs, CreditCents: credit}, nil
}
