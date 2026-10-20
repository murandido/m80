package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

// committedPaidSwitchSale is a priced sale whose rows are committed, not held in a test
// transaction: two requests racing each other need two real transactions, and neither can
// see rows another transaction never committed.
type committedPaidSwitchSale struct {
	saleID   string
	clientID *string
	actors   [2]data.User
}

// seedCommittedPaidSwitchSale commits a R$ 1.000,00 sale (optionally linked to a client of
// its own agency) and registers a cleanup that deletes every row it created. Rows go in
// dependency order; sale_payments cascade from the sale.
func seedCommittedPaidSwitchSale(t *testing.T, db *gorm.DB, withClient bool) committedPaidSwitchSale {
	t.Helper()

	var seeded committedPaidSwitchSale
	var issuance data.Issuance
	var issuer data.User
	var account data.Account
	err := db.Transaction(func(tx *gorm.DB) error {
		issuance, issuer = seedIssuance(t, tx)
		if err := tx.First(&account, "id = ?", issuance.AccountID).Error; err != nil {
			return err
		}
		if err := tx.Model(&issuance).Updates(map[string]any{
			"points":                20000,
			"value_of_the_thousand": 5000,
			"taxes":                 nil,
		}).Error; err != nil {
			return err
		}

		sale := data.Sale{IssuanceID: issuance.ID}
		if withClient {
			client := data.Client{Name: "Cliente " + uuid.New().String()[:8], AgencyID: &issuance.AgencyID}
			create(t, tx, &client)
			sale.ClientID = &client.ID
			seeded.clientID = &client.ID
		}
		create(t, tx, &sale)
		seeded.saleID = sale.ID

		seeded.actors[0] = createUser(t, tx, "Operador A")
		seeded.actors[1] = createUser(t, tx, "Operador B")
		return nil
	})
	if err != nil {
		t.Fatalf("seeding the committed sale: %v", err)
	}

	t.Cleanup(func() {
		statements := []struct {
			query string
			args  []any
		}{
			{"DELETE FROM sales WHERE id = ?", []any{seeded.saleID}},
			{"DELETE FROM clients WHERE id = ?", []any{seeded.clientID}},
			{"DELETE FROM issuances WHERE id = ?", []any{issuance.ID}},
			{"DELETE FROM accounts WHERE id = ?", []any{account.ID}},
			{"DELETE FROM people WHERE id = ?", []any{account.HolderID}},
			{"DELETE FROM loyalty_programs WHERE id = ?", []any{issuance.LoyaltyProgramID}},
			{"DELETE FROM airlines WHERE id = ?", []any{issuance.AirlineID}},
			{"DELETE FROM agencies WHERE id = ?", []any{issuance.AgencyID}},
			{"DELETE FROM users WHERE id IN ?", []any{[]string{issuer.ID, seeded.actors[0].ID, seeded.actors[1].ID}}},
		}
		for _, statement := range statements {
			if err := db.Exec(statement.query, statement.args...).Error; err != nil {
				t.Errorf("cleaning up %q: %v", statement.query, err)
			}
		}
	})

	return seeded
}

// patchSaleAs runs UpdateSale against the real connection pool, so each call opens its own
// transaction the way two browser tabs would.
func patchSaleAs(db *gorm.DB, actor data.User, saleID, body string) *httptest.ResponseRecorder {
	handler := NewSaleHandler(db, events.NewBroker())

	req := httptest.NewRequest(http.MethodPatch, "/sales/"+saleID, strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", saleID)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, actor)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.UpdateSale(rec, req)
	return rec
}

// Two operators (or one operator in two tabs) clicking "Pago" on the same sale at once
// must record the payment once. Without a lock on the sale both requests read nothing
// paid and both insert the full amount: the sale ends up paid twice, and with a client the
// second rebalance turns the extra R$ 1.000 into credit that then pays another sale with
// money that never came in.
//
// Not parallel and not inside runInTx: the race only exists between committed
// transactions. Several rounds, because a single pair of goroutines may simply not
// overlap on a given run.
func TestUpdateSale_ConcurrentPaidSwitchRecordsThePaymentOnce(t *testing.T) {
	db := setupTestDB(t)

	for _, withClient := range []bool{false, true} {
		name := "without client"
		if withClient {
			name = "with client"
		}
		t.Run(name, func(t *testing.T) {
			seeded := seedCommittedPaidSwitchSale(t, db, withClient)

			const rounds = 15
			for round := range rounds {
				var wg sync.WaitGroup
				var codes [2]int
				var bodies [2]string
				for i := range 2 {
					wg.Go(func() {
						rec := patchSaleAs(db, seeded.actors[i], seeded.saleID, `{"paid":true}`)
						codes[i], bodies[i] = rec.Code, rec.Body.String()
					})
				}
				wg.Wait()
				for i := range 2 {
					if codes[i] != http.StatusOK {
						t.Fatalf("round %d: request %d got %d (body: %s)", round, i, codes[i], bodies[i])
					}
				}

				if seeded.clientID != nil {
					credit, err := sale.CreditOf(db, *seeded.clientID)
					if err != nil {
						t.Fatalf("reading the client's credit: %v", err)
					}
					if credit != 0 {
						t.Fatalf("round %d: client credit = %d, want 0 (credit made of a double payment)", round, credit)
					}
				}

				var ownCents int
				if err := db.Raw(`SELECT COALESCE(SUM(amount_cents), 0) FROM sale_payments
					WHERE sale_id = ? AND client_payment_id IS NULL AND credit_from_sale_id IS NULL`,
					seeded.saleID).Scan(&ownCents).Error; err != nil {
					t.Fatalf("summing the statement: %v", err)
				}
				if ownCents != pricedSaleAmount {
					t.Fatalf("round %d: switch recorded %d, want %d (paid twice)", round, ownCents, pricedSaleAmount)
				}

				var allCents int
				if err := db.Raw(`SELECT COALESCE(SUM(amount_cents), 0) FROM sale_payments WHERE sale_id = ?`,
					seeded.saleID).Scan(&allCents).Error; err != nil {
					t.Fatalf("summing the statement: %v", err)
				}
				if allCents != pricedSaleAmount {
					t.Fatalf("round %d: statement sums to %d, want %d", round, allCents, pricedSaleAmount)
				}

				// Back to open for the next round, one request at a time.
				if rec := patchSaleAs(db, seeded.actors[0], seeded.saleID, `{"paid":false}`); rec.Code != http.StatusOK {
					t.Fatalf("round %d: reopening got %d (body: %s)", round, rec.Code, rec.Body.String())
				}
			}
		})
	}
}
