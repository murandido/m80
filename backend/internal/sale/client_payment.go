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
// sum's id. A payment recorded on the sale itself (the Pago switch) is never touched and
// simply counts as already paid.
//
// What a sale's own payments brought in beyond what it is worth - a ticket paid and then
// made cheaper, or cancelled - is the client's money too: the agency never hands it back,
// it carries it to the client's other tickets. The replay moves that surplus with lines of
// its own, marked with the sale it came from (credit_from_sale_id): a negative one on that
// sale, which brings it to zero, and positive ones on the open sales it covers. They are
// spent before the lump sums, since that money came in first.
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

// surplusSale is a sale whose own payments add up to more than it is worth. Cents is how
// much more; PaidOn and CreatedByID come from its latest own payment, and are what the
// lines moving the surplus carry - the money was in hand from that day, typed by that
// person, not by whoever's action triggered the replay.
type surplusSale struct {
	ID          string
	Cents       int
	PaidOn      time.Time
	CreatedByID string
}

// ownPaymentLine is the condition for a line typed on the sale itself, as opposed to one
// the replay derived from a client payment or from another sale's surplus.
const ownPaymentLine = "client_payment_id IS NULL AND credit_from_sale_id IS NULL"

// clientSalesOf reads the client's sales by what their own payments leave them at, oldest
// issuance first - the order the money is spent in. Those still owing are open; those paid
// beyond their worth are surpluses. A cancelled issuance owes only its cancellation fee
// (see BalanceCents), so whatever its own payments brought in beyond the fee - all of it,
// when there is no fee - is surplus; what a lump sum had put on it goes back to the pool
// simply because the replay no longer finds it open.
func clientSalesOf(tx *gorm.DB, clientID string) ([]openSale, []surplusSale, error) {
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
		LastPaidOn           *time.Time
		LastPaidByID         *string
	}
	err := tx.Table("sales").
		Select(`sales.id, issuances.points, issuances.value_of_the_thousand, issuances.taxes,
			issuances.cancelled_at, issuances.cancellation_fee, sales.price_adjustment_cents,
			COALESCE(adjustments.cents, 0) AS adjustment_cents,
			COALESCE((SELECT SUM(amount_cents) FROM sale_payments
				WHERE sale_id = sales.id AND `+ownPaymentLine+`), 0) AS paid_cents,
			(SELECT paid_on FROM sale_payments
				WHERE sale_id = sales.id AND `+ownPaymentLine+`
				ORDER BY paid_on DESC, created_at DESC LIMIT 1) AS last_paid_on,
			(SELECT created_by_id FROM sale_payments
				WHERE sale_id = sales.id AND `+ownPaymentLine+`
				ORDER BY paid_on DESC, created_at DESC LIMIT 1) AS last_paid_by_id`).
		Joins("JOIN issuances ON issuances.id = sales.issuance_id").
		Joins("LEFT JOIN "+AdjustmentCentsJoin+" AS adjustments ON adjustments.issuance_id = issuances.id").
		Where("sales.client_id = ?", clientID).
		Where("issuances.deleted_at IS NULL").
		Order("issuances.created_at, sales.id").
		Scan(&rows).Error
	if err != nil {
		return nil, nil, err
	}

	open := make([]openSale, 0, len(rows))
	var surpluses []surplusSale
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
		balance := BalanceCents(amount, row.PaidCents)
		switch {
		case balance > 0:
			open = append(open, openSale{ID: row.ID, Balance: balance})
		// A negative balance can only come from own payments, so the latest one exists.
		case balance < 0 && row.LastPaidOn != nil && row.LastPaidByID != nil:
			surpluses = append(surpluses, surplusSale{
				ID:          row.ID,
				Cents:       -balance,
				PaidOn:      *row.LastPaidOn,
				CreatedByID: *row.LastPaidByID,
			})
		}
	}
	return open, surpluses, nil
}

func activePaymentsOf(tx *gorm.DB, clientID string) ([]data.ClientPayment, error) {
	var payments []data.ClientPayment
	err := tx.Where("client_id = ? AND deleted_at IS NULL", clientID).
		Order("paid_on, created_at, id").
		Find(&payments).Error
	return payments, err
}

// CreditOf is the client's credit: what their active payments brought in minus what was
// spread over sales, plus the surplus of their sales that no other sale took. The surplus
// term is the sum of the lines moving it: the negative line on the sale it left adds it,
// the positive ones on the sales it covered take it back off. Right after a RebalanceClient
// it is exactly what no open sale took.
func CreditOf(tx *gorm.DB, clientID string) (int, error) {
	return CreditOfClients(tx, []string{clientID})
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
				WHERE cp.client_id IN ? AND cp.deleted_at IS NULL), 0)
			- COALESCE((SELECT SUM(sp.amount_cents) FROM sale_payments sp
				JOIN sales source ON source.id = sp.credit_from_sale_id
				WHERE source.client_id IN ?), 0)`,
		clientIDs, clientIDs, clientIDs).Scan(&credit).Error
	return credit, err
}

// spreadKey names a derived line: where its money came from - a client payment, or the
// surplus of a sale - and the sale it sits on. Exactly one of the two sources is set.
type spreadKey struct {
	clientPaymentID  string
	creditFromSaleID string
	saleID           string
}

const (
	noteSurplusToCredit = "Sobra convertida em crédito do cliente"
	noteCreditFromSale  = "Crédito de outra emissão do cliente" //nolint:gosec // G101: a ledger note, not a credential
)

// planSpread is what the derived lines should be, without the database: the surpluses are
// spent first, in the order of their sales, then each payment in the order it came in, each
// taking from what the open sales still miss after the sources before it. A surplus always
// leaves its sale whole (the negative line), whether or not an open sale takes it: what
// none takes is credit, waiting for the client's next ticket, never money to hand back.
func planSpread(open []openSale, surpluses []surplusSale, payments []data.ClientPayment) map[spreadKey]int {
	balances := make([]int, len(open))
	for i, s := range open {
		balances[i] = s.Balance
	}
	want := map[spreadKey]int{}
	spend := func(source spreadKey, pool int) {
		takes, _ := AllocateFIFO(balances, pool)
		for i, take := range takes {
			if take == 0 {
				continue
			}
			balances[i] -= take
			key := source
			key.saleID = open[i].ID
			want[key] = take
		}
	}
	for _, surplus := range surpluses {
		want[spreadKey{creditFromSaleID: surplus.ID, saleID: surplus.ID}] = -surplus.Cents
		spend(spreadKey{creditFromSaleID: surplus.ID}, surplus.Cents)
	}
	for _, payment := range payments {
		spend(spreadKey{clientPaymentID: payment.ID}, payment.AmountCents)
	}
	return want
}

// RebalanceClient replays the client's active payments and the surplus of their sales over
// their open sales, and writes only what changed, so an action that moves nothing (linking
// a sale to a client with no payments) costs a few reads and touches no row. It returns
// the sales whose lines changed.
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
	open, surpluses, err := clientSalesOf(tx, clientID)
	if err != nil {
		return nil, err
	}
	want := planSpread(open, surpluses, payments)

	// What the lines are: those of every payment of this client (undone ones included), and
	// every surplus line that leaves or reaches a sale of theirs. A sale that left the
	// client still has its lines here, and loses them below; the replay of its new client
	// does the same from the other side.
	var existing []data.SalePayment
	err = tx.Where(`client_payment_id IN (SELECT id FROM client_payments WHERE client_id = ?)
		OR (credit_from_sale_id IS NOT NULL AND (
			credit_from_sale_id IN (SELECT id FROM sales WHERE client_id = ?)
			OR sale_id IN (SELECT id FROM sales WHERE client_id = ?)))`,
		clientID, clientID, clientID).
		Find(&existing).Error
	if err != nil {
		return nil, err
	}

	touched := map[string]bool{}
	for _, line := range existing {
		key := spreadKey{saleID: line.SaleID}
		if line.ClientPaymentID != nil {
			key.clientPaymentID = *line.ClientPaymentID
		}
		if line.CreditFromSaleID != nil {
			key.creditFromSaleID = *line.CreditFromSaleID
		}
		if cents, ok := want[key]; ok && cents == line.AmountCents {
			delete(want, key)
			continue
		}
		if err := tx.Delete(&line).Error; err != nil {
			return nil, err
		}
		touched[line.SaleID] = true
	}

	paymentsByID := make(map[string]data.ClientPayment, len(payments))
	for _, payment := range payments {
		paymentsByID[payment.ID] = payment
	}
	surplusesByID := make(map[string]surplusSale, len(surpluses))
	for _, surplus := range surpluses {
		surplusesByID[surplus.ID] = surplus
	}
	for key, cents := range want {
		// The line is the money seen from one sale, so it carries the day and the author of
		// where the money came from - not whoever's action happened to trigger this replay.
		line := data.SalePayment{SaleID: key.saleID, AmountCents: cents}
		var note string
		if key.clientPaymentID != "" {
			payment := paymentsByID[key.clientPaymentID]
			note = notePaidByClient
			line.PaidOn = payment.PaidOn
			line.ClientPaymentID = &payment.ID
			line.CreatedByID = payment.CreatedByID
		} else {
			surplus := surplusesByID[key.creditFromSaleID]
			note = noteCreditFromSale
			if key.saleID == surplus.ID {
				note = noteSurplusToCredit
			}
			line.PaidOn = surplus.PaidOn
			line.CreditFromSaleID = &surplus.ID
			line.CreatedByID = surplus.CreatedByID
		}
		line.Note = &note
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
