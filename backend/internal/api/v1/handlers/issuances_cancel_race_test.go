package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"github.com/murandido/m-80-backend/internal/sale"
	"gorm.io/gorm"
)

// These tests race real transactions against each other, so unlike the rest of the
// package they cannot live inside runInTx: a row lock only means something between two
// connections, and the rolled-back outer transaction would be the one connection both
// sides share. They commit their seeds instead and delete them on the way out - which is
// why they must only ever run against a disposable database, never the dev one.
//
// The races are made deterministic rather than hoped for: one side holds the row lock,
// the test waits until Postgres reports the other side blocked behind it, and only then
// lets go. Nothing here sleeps and hopes.

// seedCommittedIssuance is seedIssuance plus a sale, committed for real, with the
// cleanup registered.
func seedCommittedIssuance(t *testing.T) (*gorm.DB, data.Issuance, data.User, data.Sale) {
	t.Helper()
	db := setupTestDB(t)
	issuance, user := seedIssuance(t, db)
	linked := data.Sale{IssuanceID: issuance.ID}
	create(t, db, &linked)
	t.Cleanup(func() { deleteCommittedIssuance(t, db, issuance, user.ID) })
	return db, issuance, user, linked
}

// deleteCommittedIssuance removes everything seedCommittedIssuance and the handlers
// under test wrote, children first.
func deleteCommittedIssuance(t *testing.T, db *gorm.DB, issuance data.Issuance, userID string) {
	t.Helper()
	var account data.Account
	if err := db.Unscoped().First(&account, "id = ?", issuance.AccountID).Error; err != nil {
		t.Errorf("cleanup: failed to read the account: %v", err)
		return
	}
	statements := []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM audit_logs WHERE issuance_id = ?", []any{issuance.ID}},
		{"DELETE FROM sale_payments WHERE sale_id IN (SELECT id FROM sales WHERE issuance_id = ?)", []any{issuance.ID}},
		{"DELETE FROM sales WHERE issuance_id = ?", []any{issuance.ID}},
		{"DELETE FROM issuances WHERE id = ?", []any{issuance.ID}},
		{"DELETE FROM accounts WHERE id = ?", []any{account.ID}},
		{"DELETE FROM people WHERE id = ?", []any{account.HolderID}},
		{"DELETE FROM loyalty_programs WHERE id = ?", []any{issuance.LoyaltyProgramID}},
		{"DELETE FROM airlines WHERE id = ?", []any{issuance.AirlineID}},
		{"DELETE FROM agencies WHERE id = ?", []any{issuance.AgencyID}},
		{"DELETE FROM users WHERE id = ?", []any{userID}},
	}
	for _, statement := range statements {
		if err := db.Exec(statement.sql, statement.args...).Error; err != nil {
			t.Errorf("cleanup %q: %v", statement.sql, err)
		}
	}
}

// backendPID is the Postgres backend serving tx, so the test can ask who is waiting on it.
func backendPID(t *testing.T, tx *gorm.DB) int {
	t.Helper()
	var pid int
	if err := tx.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
		t.Fatalf("failed to read the backend pid: %v", err)
	}
	return pid
}

// waitForWaitersOn blocks until n backends are queued behind blockerPID. Two levels are
// counted because Postgres queues a second writer on the first waiter's tuple lock, not
// on the holder itself. Asking about this pid specifically, rather than counting every
// lock wait in the database, keeps other packages' tests on the same server out of it.
func waitForWaitersOn(t *testing.T, db *gorm.DB, blockerPID, n int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var waiting int
		if err := db.Raw(`
			SELECT count(*) FROM pg_stat_activity
			WHERE pg_blocking_pids(pid) && (
				SELECT ARRAY[?]::int[] || array_agg(pid) FROM pg_stat_activity
				WHERE ? = ANY(pg_blocking_pids(pid))
			)`, blockerPID, blockerPID).
			Scan(&waiting).Error; err != nil {
			t.Fatalf("failed to read lock waits: %v", err)
		}
		if waiting >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d of %d requests got blocked on the lock", waiting, n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// Operator A edits the observation (the board's autosave) while B cancels the same
// issuance. A's PATCH read the row before B's commit and used to write the whole row back
// from that read: active=true, cancelled_at and the refunds NULL. The audit log said
// "cancelled", the sale charged the full price and the costing never saw the refund -
// with no error for anybody.
func TestUpdateIssuance_AnEditInFlightDoesNotUndoACancellation(t *testing.T) {
	db, issuance, user, linked := seedCommittedIssuance(t)

	cancelTx := db.Begin()
	// Registered after the seed's cleanup, so it runs first: a failed assertion must not
	// leave the row locked under the DELETEs. Rolling back a committed tx is a no-op.
	t.Cleanup(func() { cancelTx.Rollback() })
	fee := 15000
	if _, err := issuancesvc.Cancel(cancelTx, issuance, issuancesvc.CancelInput{CancellationFee: &fee}, user.ID); err != nil {
		cancelTx.Rollback()
		t.Fatalf("cancel: %v", err)
	}
	cancelPID := backendPID(t, cancelTx)

	patched := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		patched <- callPatchIssuance(db, issuance.ID, user.ID, `{"observation":"cliente pediu janela"}`)
	}()

	waitForWaitersOn(t, db, cancelPID, 1)
	if err := cancelTx.Commit().Error; err != nil {
		t.Fatalf("commit cancel: %v", err)
	}

	rec := <-patched
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d (body: %s)", rec.Code, rec.Body.String())
	}

	var persisted data.Issuance
	if err := db.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
		t.Fatalf("failed to re-read issuance: %v", err)
	}
	if persisted.CancelledAt == nil || persisted.Active {
		t.Errorf("the edit undid the cancellation: active=%v cancelled_at=%v", persisted.Active, persisted.CancelledAt)
	}
	if persisted.CancellationFee == nil || *persisted.CancellationFee != fee {
		t.Errorf("cancellation_fee: got %v, want %d", persisted.CancellationFee, fee)
	}
	if persisted.CancelledByID == nil || *persisted.CancelledByID != user.ID {
		t.Errorf("cancelled_by_id: got %v, want %q", persisted.CancelledByID, user.ID)
	}
	if persisted.Observation == nil || *persisted.Observation != "cliente pediu janela" {
		t.Errorf("observation: got %v, want the edit kept", persisted.Observation)
	}

	amount, err := sale.AmountOf(db, linked.ID)
	if err != nil {
		t.Fatalf("AmountOf: %v", err)
	}
	if !amount.Cancelled || amount.CancellationFeeCents != fee {
		t.Errorf("sale amount: got cancelled=%v fee=%d, want cancelled with the fee", amount.Cancelled, amount.CancellationFeeCents)
	}
}

// The panel still lets an operator fix a field of a cancelled issuance (a typo in the
// observation, say). That edit must leave the cancellation exactly as it was.
func TestUpdateIssuance_EditingACancelledIssuanceKeepsItCancelled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		issuance, user := seedIssuance(t, tx)
		if rec := callCancel(tx, issuance.ID, user.ID, `{"refunded_points": 800, "cancellation_fee": 15000}`); rec.Code != http.StatusOK {
			t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		if rec := callPatchIssuance(tx, issuance.ID, user.ID, `{"observation":"corrigido"}`); rec.Code != http.StatusOK {
			t.Fatalf("patch: got %d (body: %s)", rec.Code, rec.Body.String())
		}

		var persisted data.Issuance
		if err := tx.First(&persisted, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to re-read issuance: %v", err)
		}
		if persisted.Active || persisted.CancelledAt == nil {
			t.Errorf("cancellation lost: active=%v cancelled_at=%v", persisted.Active, persisted.CancelledAt)
		}
		if persisted.RefundedPoints == nil || *persisted.RefundedPoints != 800 ||
			persisted.CancellationFee == nil || *persisted.CancellationFee != 15000 {
			t.Errorf("refunds lost: points=%v fee=%v", persisted.RefundedPoints, persisted.CancellationFee)
		}
		if persisted.Observation == nil || *persisted.Observation != "corrigido" {
			t.Errorf("observation: got %v, want %q", persisted.Observation, "corrigido")
		}
	})
}

// holdIssuanceRow opens a transaction that locks the issuance row and returns it with its
// backend pid, so requests aimed at the row can be lined up behind it.
func holdIssuanceRow(t *testing.T, db *gorm.DB, issuanceID string) (*gorm.DB, int) {
	t.Helper()
	blocker := db.Begin()
	// Runs before the seed's cleanup (LIFO), so a failed wait never leaves the row locked
	// under the DELETEs. A second rollback is harmless.
	t.Cleanup(func() { blocker.Rollback() })
	if err := blocker.Exec("SELECT id FROM issuances WHERE id = ? FOR UPDATE", issuanceID).Error; err != nil {
		blocker.Rollback()
		t.Fatalf("failed to lock the issuance: %v", err)
	}
	return blocker, backendPID(t, blocker)
}

// raceTwice fires the same request twice, waits until both are blocked on the row, lets
// them go and returns their status codes.
func raceTwice(t *testing.T, db *gorm.DB, issuanceID string, request func() *httptest.ResponseRecorder) []int {
	t.Helper()
	blocker, blockerPID := holdIssuanceRow(t, db, issuanceID)

	results := make(chan *httptest.ResponseRecorder, 2)
	for range 2 {
		go func() { results <- request() }()
	}
	waitForWaitersOn(t, db, blockerPID, 2)
	blocker.Rollback()

	var codes []int
	for range 2 {
		rec := <-results
		if rec.Code != http.StatusOK && rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("unexpected response %d: %s", rec.Code, rec.Body.String())
		}
		codes = append(codes, rec.Code)
	}
	return codes
}

func assertOneWinner(t *testing.T, codes []int) {
	t.Helper()
	ok, refused := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			ok++
		case http.StatusUnprocessableEntity:
			refused++
		}
	}
	if ok != 1 || refused != 1 {
		t.Errorf("responses %v: want exactly one 200 and one 422", codes)
	}
}

func countAuditLogs(t *testing.T, db *gorm.DB, issuanceID, action string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&data.AuditLog{}).Where("issuance_id = ? AND action = ?", issuanceID, action).Count(&count).Error; err != nil {
		t.Fatalf("failed to count audit logs: %v", err)
	}
	return count
}

// A double click on Cancelar, or two operators on the same card: both requests read the
// row while it was still active, both passed the guard, and both wrote - two "cancelled"
// audit lines, and the second request's refunds silently replacing the first's.
func TestCancelIssuance_TwoAtOnceCancelOnlyOnce(t *testing.T) {
	db, issuance, user, _ := seedCommittedIssuance(t)

	codes := raceTwice(t, db, issuance.ID, func() *httptest.ResponseRecorder {
		return callCancel(db, issuance.ID, user.ID, `{"cancellation_fee": 15000}`)
	})

	assertOneWinner(t, codes)
	if got := countAuditLogs(t, db, issuance.ID, auditActionCancelled); got != 1 {
		t.Errorf("cancelled audit lines: got %d, want 1", got)
	}
}

// The same on the way back: two Reverter clicks must restore once and write one line.
func TestUncancelIssuance_TwoAtOnceRestoreOnlyOnce(t *testing.T) {
	db, issuance, user, _ := seedCommittedIssuance(t)
	if rec := callCancel(db, issuance.ID, user.ID, ""); rec.Code != http.StatusOK {
		t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
	}

	codes := raceTwice(t, db, issuance.ID, func() *httptest.ResponseRecorder {
		return callUncancel(db, issuance.ID, user.ID)
	})

	assertOneWinner(t, codes)
	if got := countAuditLogs(t, db, issuance.ID, auditActionRestored); got != 1 {
		t.Errorf("restored audit lines: got %d, want 1", got)
	}
}

// The other direction of the same race: A edits a cancelled issuance while B reverts the
// cancellation. The edit no longer writes the cancellation back, but its response used to
// be read over the struct it started from, and GORM does not reset a pointer when the
// column comes back NULL - so A's board got a 200 saying "cancelled" for a row the
// database already had live, and kept showing it that way.
func TestUpdateIssuance_AnEditInFlightReportsARevertCommittedMeanwhile(t *testing.T) {
	db, issuance, user, _ := seedCommittedIssuance(t)
	if rec := callCancel(db, issuance.ID, user.ID, `{"cancellation_fee": 15000}`); rec.Code != http.StatusOK {
		t.Fatalf("cancel: got %d (body: %s)", rec.Code, rec.Body.String())
	}

	// The same write UncancelIssuance makes, held open so the edit lines up behind it.
	revertTx := db.Begin()
	t.Cleanup(func() { revertTx.Rollback() })
	if err := revertTx.Model(&data.Issuance{}).Where("id = ? AND active = FALSE", issuance.ID).
		Updates(map[string]any{
			"active": true, "cancelled_at": nil, "cancelled_by_id": nil,
			"refunded_points": nil, "refunded_taxes": nil, "cancellation_fee": nil,
		}).Error; err != nil {
		t.Fatalf("revert: %v", err)
	}
	revertPID := backendPID(t, revertTx)

	patched := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		patched <- callPatchIssuance(db, issuance.ID, user.ID, `{"observation":"corrigido"}`)
	}()

	waitForWaitersOn(t, db, revertPID, 1)
	if err := revertTx.Commit().Error; err != nil {
		t.Fatalf("commit revert: %v", err)
	}

	rec := <-patched
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: got %d (body: %s)", rec.Code, rec.Body.String())
	}
	var resp cancelResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Data.Active || resp.Data.CancelledAt != nil || resp.Data.Status == "cancelled" ||
		resp.Data.CancellationFee != nil {
		t.Errorf("response disagrees with the database: active=%v cancelled_at=%v status=%q fee=%v",
			resp.Data.Active, resp.Data.CancelledAt, resp.Data.Status, resp.Data.CancellationFee)
	}
}
