package handlers

import (
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

// waitForWaitersOn blocks until n backends are waiting on a lock held by blockerPID.
// Asking about this pid specifically, rather than counting every lock wait in the
// database, keeps other packages' tests running against the same server out of it.
func waitForWaitersOn(t *testing.T, db *gorm.DB, blockerPID, n int) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var waiting int
		if err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))", blockerPID).
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
