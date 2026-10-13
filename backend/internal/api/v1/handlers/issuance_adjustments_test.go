package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

type adjustmentResponse struct {
	Success bool                               `json:"success"`
	Message string                             `json:"message"`
	Data    schemas.IssuanceAdjustmentResponse `json:"data"`
}

// callAdjustment drives one of the three alteration handlers the way the router would.
func callAdjustment(tx *gorm.DB, method, issuanceID, adjustmentID, userID, body string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(method, "/issuances/"+issuanceID+"/adjustments", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", issuanceID)
	if adjustmentID != "" {
		rctx.URLParams.Add("adjustmentID", adjustmentID)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	switch method {
	case http.MethodPost:
		handler.CreateIssuanceAdjustment(rec, req)
	case http.MethodPatch:
		handler.UpdateIssuanceAdjustment(rec, req)
	case http.MethodDelete:
		handler.DeleteIssuanceAdjustment(rec, req)
	}
	return rec
}

func decodeAdjustment(t *testing.T, rec *httptest.ResponseRecorder) schemas.IssuanceAdjustmentResponse {
	t.Helper()
	var resp adjustmentResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decoding the adjustment: %v (body: %s)", err, rec.Body.String())
	}
	return resp.Data
}

func saleAmountCents(t *testing.T, tx *gorm.DB, saleID string) int {
	t.Helper()
	amount, err := sale.AmountOf(tx, saleID)
	if err != nil {
		t.Fatalf("reading the sale amount: %v", err)
	}
	return amount.Cents
}

const passedOnAlteration = `{"occurred_on":"2026-10-13","points":10000,"taxes":8000,
	"value_of_the_thousand":2000,"taxes_passed_to_client":true,"reason":"GRU-CNF trocado para GRU-BHZ"}`

// The whole round trip: recording an alteration the client pays for raises the sale, is
// logged, and comes back on the issuance; deleting it puts the sale back.
func TestIssuanceAdjustmentRoundTrip(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		saleRow := data.Sale{IssuanceID: issuance.ID}
		create(t, tx, &saleRow)
		before := saleAmountCents(t, tx, saleRow.ID)

		rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, passedOnAlteration)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d, want 201 (body: %s)", rec.Code, rec.Body.String())
		}
		created := decodeAdjustment(t, rec)
		// 10 milheiros at 20,00 plus R$ 80,00 of taxes.
		if created.ClientAmountCents != 28000 || created.OccurredOn != "2026-10-13" {
			t.Errorf("created = %+v, want 28000 cents on 2026-10-13", created)
		}
		if got := saleAmountCents(t, tx, saleRow.ID); got != before+28000 {
			t.Errorf("sale amount = %d, want %d", got, before+28000)
		}

		var logged int64
		tx.Model(&data.AuditLog{}).
			Where("issuance_id = ? AND action = ? AND details->>'reason' = ?", issuance.ID, "updated", "adjustment_created").
			Count(&logged)
		if logged != 1 {
			t.Errorf("got %d audit entries for the alteration, want 1", logged)
		}

		var reread data.Issuance
		if err := preloadIssuanceAssociations(tx).First(&reread, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("re-reading the issuance: %v", err)
		}
		if got := toIssuanceResponse(reread).Adjustments; len(got) != 1 || got[0].ID != created.ID {
			t.Errorf("issuance adjustments = %+v, want the one just created", got)
		}

		rec = callAdjustment(tx, http.MethodPatch, issuance.ID, created.ID, user.ID,
			`{"occurred_on":"2026-10-14","points":10000,"taxes":8000,"value_of_the_thousand":0,"taxes_passed_to_client":false,"reason":"absorvido"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("update status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if got := saleAmountCents(t, tx, saleRow.ID); got != before {
			t.Errorf("sale amount after absorbing = %d, want %d", got, before)
		}

		rec = callAdjustment(tx, http.MethodDelete, issuance.ID, created.ID, user.ID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("delete status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		var left int64
		tx.Model(&data.IssuanceAdjustment{}).Where("issuance_id = ?", issuance.ID).Count(&left)
		if left != 0 {
			t.Errorf("got %d live alterations after deleting, want 0", left)
		}
	})
}

func TestIssuanceAdjustmentRefusals(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		empty := `{"occurred_on":"2026-10-13","points":0,"taxes":0,"reason":"nada"}`
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, empty); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("empty alteration: status %d, want 422", rec.Code)
		}
		noReason := `{"occurred_on":"2026-10-13","points":1000,"taxes":0,"reason":""}`
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, noReason); rec.Code != http.StatusBadRequest {
			t.Errorf("no reason: status %d, want 400", rec.Code)
		}

		seedClosedPeriod(t, tx, issuance.AccountID, user.ID, time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC))
		closed := `{"occurred_on":"2026-10-09","points":1000,"taxes":0,"reason":"no período fechado"}`
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, closed); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("closed period: status %d, want 422", rec.Code)
		}

		now := time.Now()
		if err := tx.Model(&issuance).Updates(map[string]any{"active": false, "cancelled_at": now}).Error; err != nil {
			t.Fatalf("cancelling: %v", err)
		}
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, passedOnAlteration); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("cancelled issuance: status %d, want 422", rec.Code)
		}
	})
}

// sale.AdjustmentCentsJoin is the SQL twin of sale.AdjustmentClientCents, used by every
// list query. The awkward amounts are the ones that round, in both directions.
func TestAdjustmentCentsJoinMatchesTheGoFunction(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		cases := []data.IssuanceAdjustment{
			{Points: 1001, ValueOfTheThousand: 1500, Taxes: 333, TaxesPassedToClient: true},
			{Points: -1001, ValueOfTheThousand: 1500, Taxes: -333, TaxesPassedToClient: true},
			{Points: 7777, ValueOfTheThousand: 1999, Taxes: 100, TaxesPassedToClient: false},
			{Points: -3, ValueOfTheThousand: 1, Taxes: 0},
		}
		want := 0
		for i := range cases {
			cases[i].IssuanceID = issuance.ID
			cases[i].OccurredOn = time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
			cases[i].Reason = "arredondamento"
			cases[i].CreatedByID = user.ID
			create(t, tx, &cases[i])
			want += sale.AdjustmentClientCents(cases[i].Points, cases[i].ValueOfTheThousand, cases[i].Taxes, cases[i].TaxesPassedToClient)
		}

		var got struct{ Cents int }
		if err := tx.Raw("SELECT cents FROM "+sale.AdjustmentCentsJoin+" AS adjustments WHERE issuance_id = ?", issuance.ID).
			Scan(&got).Error; err != nil {
			t.Fatalf("running the join: %v", err)
		}
		if got.Cents != want {
			t.Errorf("SQL total = %d, Go total = %d", got.Cents, want)
		}
	})
}

// An alteration is its own line on the supplier statement, on its own day, still read as
// an issued line so a panel that does not know alterations parses it.
func TestSupplierStatementCarriesTheAlteration(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		today := spDay(t, 0)

		body := `{"occurred_on":"` + today + `","points":500,"taxes":1000,"reason":"remarcação"}`
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, body); rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d (body: %s)", rec.Code, rec.Body.String())
		}

		balance := findBalance(callGetSupplierBalances(t, tx, "date_from="+today+"&date_to="+today), issuance.AccountID)
		if balance == nil {
			t.Fatalf("no balance for the account")
		}
		// 1000 booked + 500 altered.
		if balance.IssuedPoints != 1500 {
			t.Errorf("issued points = %d, want 1500", balance.IssuedPoints)
		}
		var line *schemas.SupplierIssuanceResponse
		for i := range balance.Issuances {
			if balance.Issuances[i].AdjustmentID != nil {
				line = &balance.Issuances[i]
			}
		}
		if line == nil || line.Kind != "issued" || line.Points != 500 || line.Taxes == nil || *line.Taxes != 1000 {
			t.Errorf("alteration line = %+v, want an issued line of 500 points and 1000 in taxes", line)
		}
	})
}

// An old booking altered today belongs to today: on the board, in the flow of whoever
// recorded the alteration, and - when the client owes the difference - in the payments
// control. An absorbed alteration has nothing to collect, so the sale stays on its own day.
func TestAlteredIssuanceShowsOnTheAlterationDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		passedOn, user := seedIssuance(t, tx)
		absorbed, _ := seedIssuance(t, tx)
		old := time.Now().AddDate(0, 0, -10)
		// The sales listing defaults to the main agency.
		main := defaultAgency(t, tx)
		for _, issuance := range []data.Issuance{passedOn, absorbed} {
			if err := tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
				Updates(map[string]any{"created_at": old, "agency_id": main.ID}).Error; err != nil {
				t.Fatalf("backdating: %v", err)
			}
			create(t, tx, &data.Sale{IssuanceID: issuance.ID})
		}

		today := spDay(t, 0)
		charge := `{"occurred_on":"` + today + `","points":1000,"taxes":0,"value_of_the_thousand":2000,"reason":"remarcação"}`
		absorb := `{"occurred_on":"` + today + `","points":1000,"taxes":0,"value_of_the_thousand":0,"reason":"absorvida"}`
		for issuanceID, body := range map[string]string{passedOn.ID: charge, absorbed.ID: absorb} {
			if rec := callAdjustment(tx, http.MethodPost, issuanceID, "", user.ID, body); rec.Code != http.StatusCreated {
				t.Fatalf("create status = %d (body: %s)", rec.Code, rec.Body.String())
			}
		}

		listed := callGetIssuances(t, tx, "date_from="+today+"&date_to="+today+"&issuer_id="+user.ID)
		if !listed[passedOn.ID] || !listed[absorbed.ID] {
			t.Errorf("today's flow = %v, want both altered issuances", listed)
		}
		if earlier := callGetIssuances(t, tx, "date_from="+spDay(t, -3)+"&date_to="+spDay(t, -1)); earlier[passedOn.ID] {
			t.Error("the altered issuance showed up on days with no booking and no alteration")
		}

		sales := callGetSales(t, tx, "date_from="+today+"&date_to="+today)
		var sawPassed, sawAbsorbed bool
		for _, s := range sales {
			sawPassed = sawPassed || s.IssuanceID == passedOn.ID
			sawAbsorbed = sawAbsorbed || s.IssuanceID == absorbed.ID
		}
		if !sawPassed || sawAbsorbed {
			t.Errorf("today's sales: passed-on %v, absorbed %v - want true, false", sawPassed, sawAbsorbed)
		}
	})
}

// On the alteration day the sale shows what there is to collect that day: only the
// alteration when the booking was already paid, the whole sale when it was not.
func TestSaleOnTheAlterationDayShowsWhatIsOwed(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		main := defaultAgency(t, tx)
		paidBefore, user := seedIssuance(t, tx)
		unpaid, _ := seedIssuance(t, tx)
		old := time.Now().AddDate(0, 0, -10)
		sales := map[string]data.Sale{}
		for _, issuance := range []data.Issuance{paidBefore, unpaid} {
			if err := tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
				Updates(map[string]any{"created_at": old, "agency_id": main.ID}).Error; err != nil {
				t.Fatalf("backdating: %v", err)
			}
			s := data.Sale{IssuanceID: issuance.ID}
			create(t, tx, &s)
			sales[issuance.ID] = s
		}
		// 1000 points at 0,50 the thousand: the booking is worth 50 centavos.
		create(t, tx, &data.SalePayment{SaleID: sales[paidBefore.ID].ID, AmountCents: 50, PaidOn: old, CreatedByID: user.ID})

		today := spDay(t, 0)
		charge := `{"occurred_on":"` + today + `","points":1000,"taxes":0,"value_of_the_thousand":2000,"reason":"remarcação"}`
		for _, issuance := range []data.Issuance{paidBefore, unpaid} {
			if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, charge); rec.Code != http.StatusCreated {
				t.Fatalf("create status = %d (body: %s)", rec.Code, rec.Body.String())
			}
		}

		listed := callGetSales(t, tx, "date_from="+today+"&date_to="+today)
		paid := findSale(listed, sales[paidBefore.ID].ID)
		open := findSale(listed, sales[unpaid.ID].ID)
		if paid == nil || open == nil {
			t.Fatalf("expected both altered sales today, got %+v", listed)
		}
		if !paid.AlterationDay || paid.AmountCents != 2000 || paid.PaidCents != 0 || paid.BalanceCents != 2000 || paid.AdjustmentCents != 2000 {
			t.Errorf("paid-before sale = amount %d paid %d balance %d, want 2000/0/2000", paid.AmountCents, paid.PaidCents, paid.BalanceCents)
		}
		if !open.AlterationDay || open.AmountCents != 2050 || open.BalanceCents != 2050 {
			t.Errorf("unpaid sale = amount %d balance %d, want the whole 2050", open.AmountCents, open.BalanceCents)
		}

		// On its own issue day the sale is whole and not an alteration-day row.
		issued := findSale(callGetSales(t, tx, "date_from="+spDay(t, -10)+"&date_to="+spDay(t, -10)), sales[paidBefore.ID].ID)
		if issued == nil || issued.AlterationDay || issued.AmountCents != 2050 {
			t.Errorf("issue-day sale = %+v, want the whole 2050", issued)
		}
	})
}

// A booking paid in full and then altered is not "partly paid": the difference is a new
// charge, and the switch settles it. A booking only partly paid stays locked as before.
func TestAlterationOfAPaidBookingCanBeSettledByHand(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		s := data.Sale{IssuanceID: issuance.ID}
		create(t, tx, &s)
		// 1000 points at 0,50 the thousand: the booking is worth 50 centavos.
		create(t, tx, &data.SalePayment{SaleID: s.ID, AmountCents: 50, PaidOn: time.Now(), CreatedByID: user.ID})

		charge := `{"occurred_on":"` + spDay(t, 0) + `","points":1000,"taxes":0,"value_of_the_thousand":2000,"reason":"remarcação"}`
		if rec := callAdjustment(tx, http.MethodPost, issuance.ID, "", user.ID, charge); rec.Code != http.StatusCreated {
			t.Fatalf("create status = %d (body: %s)", rec.Code, rec.Body.String())
		}
		if partial, err := salePartlyPaid(tx, s.ID); err != nil || partial {
			t.Errorf("salePartlyPaid = %v, %v; want false - only the alteration is missing", partial, err)
		}

		if err := tx.Model(&data.SalePayment{}).Where("sale_id = ?", s.ID).Update("amount_cents", 20).Error; err != nil {
			t.Fatalf("shrinking the payment: %v", err)
		}
		if partial, err := salePartlyPaid(tx, s.ID); err != nil || !partial {
			t.Errorf("salePartlyPaid = %v, %v; want true - the booking itself is not covered", partial, err)
		}
	})
}
