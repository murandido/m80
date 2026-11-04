package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/events"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The issuance handlers talk to Postgres directly (there is no service layer to mock), so
// these tests need a live database - the same one CI provisions. When no database is
// reachable (e.g. a local machine without `make dev` up) the whole file skips instead of
// failing, keeping `go test ./...` green everywhere.

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error

	// phoneCounter feeds unique, valid Brazilian phone numbers to seeded users.
	phoneCounter int64
)

// setupTestDB connects once per test binary. It first probes connectivity with a silent
// gorm session + ping so an absent database yields t.Skip rather than the log.Fatal that
// data.Connect() would trigger; only after a successful ping does it run the production
// connect path (which applies the golang-migrate schema).
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}

		// Schema is owned by golang-migrate; data.Connect runs it and sets data.DB.
		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping issuance cancel tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs the test body inside a transaction that is always rolled back, so seeded
// rows never leak between tests. The handler opens its own nested transaction, which GORM
// maps to a SAVEPOINT under this outer tx.
//
// It also marks the test parallel, which is what that rollback buys. These tests are
// almost entirely waiting on Postgres, and READ COMMITTED plus a transaction nobody ever
// commits means a concurrent test cannot see another one's rows - the isolation the
// rollback already provided is the isolation parallelism needs. Every seed that touches a
// unique column already carries a per-call random suffix, so there is nothing to collide
// on either.
//
// The consequence to know about: a test that calls this may not also call t.Setenv, which
// the runtime rejects after t.Parallel. Process-wide environment belongs in TestMain.
func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)
	t.Parallel()

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}

// seedIssuance creates the full FK chain (airline → loyalty program → account, plus agency
// and user) required for a valid issuance and returns the created issuance and issuer user.
// Unique-constrained columns get a per-call random suffix so seeds never collide.
func seedIssuance(t *testing.T, tx *gorm.DB) (data.Issuance, data.User) {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air", Code: "T" + suffix}
	create(t, tx, &airline)

	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)

	account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID}
	create(t, tx, &account)

	agency := data.Agency{Name: "Test Agency " + suffix}
	create(t, tx, &agency)

	user := createUser(t, tx, "Tester")

	issuance := data.Issuance{
		LoyaltyProgramID:   program.ID,
		AirlineID:          airline.ID,
		ProgramLocator:     "PL" + suffix,
		Points:             1000,
		ValueOfTheThousand: 50,
		AccountID:          account.ID,
		IssuerID:           user.ID,
		AgencyID:           agency.ID,
	}
	create(t, tx, &issuance)

	return issuance, user
}

func create(t *testing.T, tx *gorm.DB, model any) {
	t.Helper()
	if err := tx.Create(model).Error; err != nil {
		t.Fatalf("failed to seed %T: %v", model, err)
	}
}

// createUser seeds a user with a unique, valid Brazilian phone (User.BeforeCreate
// validates DDD + 9 digits), built from an atomic counter so seeds never collide.
// The "1199" prefix keeps the seed a realistic mobile number: only local parts
// starting with 6-9 carry the optional "nono dígito", so this is the only shape
// for which dropping that digit is reversible - see utils.NormalizeBRPhone.
func createUser(t *testing.T, tx *gorm.DB, name string) data.User {
	t.Helper()
	phone := fmt.Sprintf("1199%07d", atomic.AddInt64(&phoneCounter, 1)%10000000)
	user := data.User{Name: name, Phone: phone, Password: "irrelevant"}
	create(t, tx, &user)
	return user
}

// cancelResponse mirrors the envelope produced by utils.SendJSON for a successful cancel.
type cancelResponse struct {
	Success bool                     `json:"success"`
	Message string                   `json:"message"`
	Data    schemas.IssuanceResponse `json:"data"`
}

// callCancel drives CancelIssuance directly, injecting the chi {id} param and the
// authenticated user the same way the real middleware chain would.
func callCancel(tx *gorm.DB, id, userID, body string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	var reqBody *strings.Reader
	if body == "" {
		reqBody = strings.NewReader("")
	} else {
		reqBody = strings.NewReader(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/issuances/"+id+"/cancel", reqBody)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.CancelIssuance(rec, req)
	return rec
}

func TestCancelIssuance_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		body := `{"refunded_points": 800, "refunded_taxes": 1500, "cancellation_fee": 500}`
		rec := callCancel(tx, issuance.ID, user.ID, body)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp cancelResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Data.Active {
			t.Error("expected active=false in response")
		}
		if resp.Data.CancelledAt == nil {
			t.Error("expected cancelled_at to be set in response")
		}
		if resp.Data.Status != "cancelled" {
			t.Errorf("status: got %q, want %q", resp.Data.Status, "cancelled")
		}
		if resp.Data.RefundedPoints == nil || *resp.Data.RefundedPoints != 800 {
			t.Errorf("refunded_points: got %v, want 800", resp.Data.RefundedPoints)
		}
		if resp.Data.RefundedTaxes == nil || *resp.Data.RefundedTaxes != 1500 {
			t.Errorf("refunded_taxes: got %v, want 1500", resp.Data.RefundedTaxes)
		}
		if resp.Data.CancellationFee == nil || *resp.Data.CancellationFee != 500 {
			t.Errorf("cancellation_fee: got %v, want 500", resp.Data.CancellationFee)
		}

		// Confirm the row was actually persisted, not just serialized from memory.
		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if persisted.Active {
			t.Error("persisted issuance still active")
		}
		if persisted.CancelledAt == nil {
			t.Error("persisted issuance has no cancelled_at")
		}
		if persisted.CancelledByID == nil || *persisted.CancelledByID != user.ID {
			t.Errorf("cancelled_by_id: got %v, want %q", persisted.CancelledByID, user.ID)
		}
	})
}

func TestCancelIssuance_NoBodySucceeds(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		rec := callCancel(tx, issuance.ID, user.ID, "")

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var resp cancelResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Data.Active {
			t.Error("expected active=false")
		}
		if resp.Data.RefundedPoints != nil {
			t.Errorf("expected refunded_points nil, got %v", *resp.Data.RefundedPoints)
		}
	})
}

func TestCancelIssuance_AlreadyCancelled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		if err := tx.Model(&issuance).Update("active", false).Error; err != nil {
			t.Fatalf("failed to pre-cancel issuance: %v", err)
		}

		rec := callCancel(tx, issuance.ID, user.ID, "")

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
		var resp utils.StandardResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Message != "issuance already cancelled" {
			t.Errorf("message: got %q, want %q", resp.Message, "issuance already cancelled")
		}
	})
}

func TestCancelIssuance_NotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		_, user := seedIssuance(t, tx)

		rec := callCancel(tx, uuid.New().String(), user.ID, "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

func TestCancelIssuance_DeletedIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		if err := tx.Delete(&issuance).Error; err != nil { // soft delete
			t.Fatalf("failed to soft-delete issuance: %v", err)
		}

		rec := callCancel(tx, issuance.ID, user.ID, "")

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

// Cancelling never touches the sale's ledger - money does not move because a booking was
// cancelled. What it changes is what is *owed*, and the two cases pull in opposite
// directions, so both are pinned here.
//
// Paid: the price stops being a debt and the refund becomes one, so the sale stays pending
// until somebody hands the money back.
func TestCancelIssuance_APaidSaleBecomesARefundToMake(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		sale := data.Sale{IssuanceID: issuance.ID}
		create(t, tx, &sale)

		// Through the compatibility switch, which records a payment for the full value: a
		// sale marked paid over an empty statement is a state the system cannot produce.
		if rec := callUpdateSale(t, tx, sale.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("marking paid: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		rec := callCancel(tx, issuance.ID, user.ID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var persisted data.Sale
		if err := tx.First(&persisted, "issuance_id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if persisted.Paid {
			t.Error("the cancelled sale reads as settled while the client's money is still in the till")
		}

		// The statement itself is untouched: the payment happened and still did.
		var payments int64
		tx.Model(&data.SalePayment{}).Where("sale_id = ?", sale.ID).Count(&payments)
		if payments != 1 {
			t.Errorf("statement has %d lines, want the original payment still there", payments)
		}
	})
}

// Unpaid: nothing came in, and the price is no longer owed either - so there is nothing
// left to do on this sale at all. This is the case that used to read as owing its full
// value forever, long after the ticket it was for had been cancelled.
func TestCancelIssuance_AnUnpaidSaleHasNothingLeftToDo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		sale := data.Sale{IssuanceID: issuance.ID}
		create(t, tx, &sale)

		if rec := callCancel(tx, issuance.ID, user.ID, ""); rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Sale
		if err := tx.First(&persisted, "id = ?", sale.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if !persisted.Paid {
			t.Error("a cancelled sale nobody ever paid still counts as pending; there is nothing to collect")
		}
	})
}

func TestCancelIssuance_WritesAuditLog(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)

		body := `{"refunded_points": 200}`
		rec := callCancel(tx, issuance.ID, user.ID, body)

		if rec.Code != http.StatusOK {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
		}

		var logs []data.AuditLog
		if err := tx.Where("issuance_id = ?", issuance.ID).Find(&logs).Error; err != nil {
			t.Fatalf("failed to read audit logs: %v", err)
		}
		if len(logs) != 1 {
			t.Fatalf("audit logs: got %d, want 1", len(logs))
		}
		log := logs[0]
		if log.Action != auditActionCancelled {
			t.Errorf("action: got %q, want %q", log.Action, auditActionCancelled)
		}
		if log.ChangedByID != user.ID {
			t.Errorf("changed_by_id: got %q, want %q", log.ChangedByID, user.ID)
		}

		var details map[string]any
		if err := json.Unmarshal(log.Details, &details); err != nil {
			t.Fatalf("failed to decode audit details: %v", err)
		}
		if got := details["refunded_points"]; got != float64(200) {
			t.Errorf("details.refunded_points: got %v, want 200", got)
		}
	})
}

// listResponse mirrors the GET /issuances envelope.
type listResponse struct {
	Success bool                       `json:"success"`
	Message string                     `json:"message"`
	Data    []schemas.IssuanceResponse `json:"data"`
}

// callGetIssuances drives GetIssuances with a raw query string (the handler reads only
// query params, no auth context) and returns the set of issuance ids it returned.
func callGetIssuances(t *testing.T, tx *gorm.DB, query string) map[string]bool {
	t.Helper()
	return callGetIssuancesAs(t, tx, query, access.FullScope())
}

// callGetIssuancesAs lists issuances as a caller restricted to the given agency scope.
func callGetIssuancesAs(t *testing.T, tx *gorm.DB, query string, scope access.AgencyScope) map[string]bool {
	t.Helper()
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)
	req := withScope(httptest.NewRequest(http.MethodGet, "/issuances?"+query, nil), scope)
	rec := httptest.NewRecorder()
	handler.GetIssuances(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GetIssuances status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	var resp listResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}
	ids := make(map[string]bool, len(resp.Data))
	for _, issuance := range resp.Data {
		ids[issuance.ID] = true
	}
	return ids
}

// spTodayString returns today's calendar date in the list filter's timezone, matching how
// applyIssuanceListFilters resolves date_from/date_to.
func spTodayString(t *testing.T) string {
	t.Helper()
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.UTC
	}
	return time.Now().In(loc).Format("2006-01-02")
}

func TestGetIssuances_CancelledSurfacesByCanceller(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// Created last week by its issuer (userA), cancelled today by userB.
		issuance, userA := seedIssuance(t, tx)
		userB := createUser(t, tx, "Canceller")

		lastWeek := time.Now().AddDate(0, 0, -7)
		if err := tx.Model(&issuance).Update("created_at", lastWeek).Error; err != nil {
			t.Fatalf("failed to backdate created_at: %v", err)
		}
		if err := tx.Model(&issuance).Updates(map[string]any{
			"active":          false,
			"cancelled_at":    time.Now(),
			"cancelled_by_id": userB.ID,
		}).Error; err != nil {
			t.Fatalf("failed to mark cancelled: %v", err)
		}

		today := spTodayString(t)

		// userB cancelled it today -> it appears in userB's today flow.
		byCanceller := callGetIssuances(t, tx, "issuer_id="+userB.ID+"&date_from="+today+"&date_to="+today)
		if !byCanceller[issuance.ID] {
			t.Error("expected cancelled issuance to appear in the canceller's today flow")
		}

		// userA created it last week -> it must NOT appear in userA's today flow.
		byCreator := callGetIssuances(t, tx, "issuer_id="+userA.ID+"&date_from="+today+"&date_to="+today)
		if byCreator[issuance.ID] {
			t.Error("cancelled issuance created last week should not appear in the creator's today flow")
		}
	})
}

func TestGetIssuances_ActiveTodaySurfacesByCreator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		// seedIssuance leaves created_at at NOW() and the row active.
		issuance, userA := seedIssuance(t, tx)
		today := spTodayString(t)

		ids := callGetIssuances(t, tx, "issuer_id="+userA.ID+"&date_from="+today+"&date_to="+today)
		if !ids[issuance.ID] {
			t.Error("expected today's active issuance to appear in the creator's today flow")
		}
	})
}

// callUncancel drives UncancelIssuance directly, mirroring callCancel's param/auth injection.
func callUncancel(tx *gorm.DB, id, userID string) *httptest.ResponseRecorder {
	handler := NewIssuanceHandler(tx, events.NewBroker(), nil)

	req := httptest.NewRequest(http.MethodPost, "/issuances/"+id+"/uncancel", nil)

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = asUser(ctx, data.User{ID: userID, Role: "admin"})
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.UncancelIssuance(rec, req)
	return rec
}

func TestUncancelIssuance_Success(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		// A sale paid in full, through the compatibility switch so its statement is real:
		// what the client paid lives there, and neither cancel nor uncancel moves it.
		sale := data.Sale{IssuanceID: issuance.ID}
		create(t, tx, &sale)
		if rec := callUpdateSale(t, tx, sale.ID, `{"paid":true}`); rec.Code != http.StatusOK {
			t.Fatalf("marking paid: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if rec := callCancel(tx, issuance.ID, user.ID, `{"refunded_points": 800, "refunded_taxes": 1500}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		reverter := createUser(t, tx, "Reverter")
		rec := callUncancel(tx, issuance.ID, reverter.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("uncancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var resp cancelResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Data.Active {
			t.Error("expected active=true in response")
		}
		if resp.Data.CancelledAt != nil {
			t.Error("expected cancelled_at to be cleared in response")
		}
		if resp.Data.Status != "incomplete" {
			t.Errorf("status: got %q, want %q (derived from complete)", resp.Data.Status, "incomplete")
		}
		if resp.Data.RefundedPoints != nil || resp.Data.RefundedTaxes != nil || resp.Data.CancellationFee != nil {
			t.Error("expected refund fields cleared in response")
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if !persisted.Active {
			t.Error("persisted issuance still inactive")
		}
		if persisted.CancelledAt != nil || persisted.CancelledByID != nil {
			t.Error("persisted issuance still carries cancellation metadata")
		}
		if persisted.RefundedPoints != nil || persisted.RefundedTaxes != nil || persisted.CancellationFee != nil {
			t.Error("persisted issuance still carries refund amounts")
		}

		var persistedSale data.Sale
		if err := tx.First(&persistedSale, "issuance_id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		// Back to owing nothing: the ticket is a debt again and the payment still covers it.
		if !persistedSale.Paid {
			t.Error("the sale came back pending from a round trip that never moved any money")
		}

		var restoreLog data.AuditLog
		if err := tx.Where("issuance_id = ? AND action = ?", issuance.ID, auditActionRestored).First(&restoreLog).Error; err != nil {
			t.Fatalf("expected a 'restored' audit log: %v", err)
		}
		if restoreLog.ChangedByID != reverter.ID {
			t.Errorf("changed_by_id: got %q, want %q", restoreLog.ChangedByID, reverter.ID)
		}
	})
}

// The other half of the same rule: an unpaid sale is not fabricated into a paid one by a
// round trip either. Neither direction of the cancellation touches the money.
func TestUncancelIssuance_LeavesAnUnpaidSaleUnpaid(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		sale := data.Sale{IssuanceID: issuance.ID, Paid: false}
		create(t, tx, &sale)

		if rec := callCancel(tx, issuance.ID, user.ID, ""); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}
		if rec := callUncancel(tx, issuance.ID, user.ID); rec.Code != http.StatusOK {
			t.Fatalf("uncancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persistedSale data.Sale
		if err := tx.First(&persistedSale, "issuance_id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read sale: %v", err)
		}
		if persistedSale.Paid {
			t.Error("a sale nobody had paid came back paid")
		}
	})
}

func TestUncancelIssuance_NotCancelled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx) // active by default

		rec := callUncancel(tx, issuance.ID, user.ID)

		if rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusUnprocessableEntity, rec.Body.String())
		}
		var resp utils.StandardResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.Message != "issuance is not cancelled" {
			t.Errorf("message: got %q, want %q", resp.Message, "issuance is not cancelled")
		}
	})
}

func TestUncancelIssuance_NotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		_, user := seedIssuance(t, tx)

		rec := callUncancel(tx, uuid.New().String(), user.ID)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

func TestUncancelIssuance_DeletedIsNotFound(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		if err := tx.Delete(&issuance).Error; err != nil { // soft delete
			t.Fatalf("failed to soft-delete issuance: %v", err)
		}

		rec := callUncancel(tx, issuance.ID, user.ID)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusNotFound, rec.Body.String())
		}
	})
}

// Reverting a cancellation asserts the booking is live, which is the opposite of what
// cancellation_only claims. Left set, the row counts neither as issued - the flag
// suppresses it - nor as refunded, because cancelled_at is nil again: it disappears from
// the supplier balance on both sides at once, which is the worst kind of wrong number
// because nothing on any screen says it is missing.
func TestUncancelIssuance_ClearsCancellationOnly(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		if err := tx.Model(&issuance).Update("cancellation_only", true).Error; err != nil {
			t.Fatalf("marking the issuance cancellation-only: %v", err)
		}
		if rec := callCancel(tx, issuance.ID, user.ID, `{"refunded_points": 800}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		if rec := callUncancel(tx, issuance.ID, user.ID); rec.Code != http.StatusOK {
			t.Fatalf("uncancel status: got %d, want 200 (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if !persisted.Active || persisted.CancelledAt != nil {
			t.Fatalf("active = %v, cancelled_at = %v, want the cancellation reverted",
				persisted.Active, persisted.CancelledAt)
		}
		if persisted.CancellationOnly {
			t.Error("cancellation_only is still set: the row would vanish from the supplier " +
				"balance, counted neither as issued nor as refunded")
		}
	})
}

// The panel refuses negative amounts, but the Go side used to take them: a fee of -15000
// made the sale "owe back" R$ 150 more than came in, which with a client became credit
// on the next sale. Zero stays valid - it is how "no fee" is typed.
func TestCancelIssuance_RejectsNegativeAmounts(t *testing.T) {
	for _, field := range []string{"cancellation_fee", "refunded_points", "refunded_taxes"} {
		t.Run(field, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				issuance, user := seedIssuance(t, tx)

				rec := callCancel(tx, issuance.ID, user.ID, `{"`+field+`": -1}`)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("status: got %d, want %d (body: %s)", rec.Code, http.StatusBadRequest, rec.Body.String())
				}

				var persisted data.Issuance
				if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
					t.Fatalf("failed to re-read issuance: %v", err)
				}
				if !persisted.Active {
					t.Error("the issuance was cancelled with a negative amount")
				}
			})
		})
	}

	t.Run("zero is valid", func(t *testing.T) {
		runInTx(t, func(tx *gorm.DB) {
			issuance, user := seedIssuance(t, tx)
			if rec := callCancel(tx, issuance.ID, user.ID, `{"cancellation_fee": 0}`); rec.Code != http.StatusOK {
				t.Fatalf("zero fee: got %d (body: %s)", rec.Code, rec.Body.String())
			}
		})
	})
}
