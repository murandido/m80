package data

import (
	"testing"

	"gorm.io/gorm"
)

func TestAccountEncryptsTheLatamSessionAtRest(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)

		token := "latam-refresh-token-em-claro"
		account.LatamRefreshToken = &token
		if err := tx.Save(&account).Error; err != nil {
			t.Fatalf("saving the session: %v", err)
		}

		// What is on disk must not be the token itself. Read raw, bypassing the AfterFind hook
		// that would decrypt it.
		var stored string
		if err := tx.Raw("SELECT latam_refresh_token FROM accounts WHERE id = ?", account.ID).
			Scan(&stored).Error; err != nil {
			t.Fatalf("reading the raw column: %v", err)
		}
		if stored == token {
			t.Fatal("the refresh token was stored in clear")
		}

		var reloaded Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading the account: %v", err)
		}
		if reloaded.LatamRefreshToken == nil || *reloaded.LatamRefreshToken != token {
			t.Fatalf("read back %v, want the token", reloaded.LatamRefreshToken)
		}

		// The state row takes the enum's values and defaults to pending.
		state := LatamSyncState{AccountID: account.ID}
		if err := tx.Create(&state).Error; err != nil {
			t.Fatalf("creating the state: %v", err)
		}
		var readBack LatamSyncState
		if err := tx.First(&readBack, "account_id = ?", account.ID).Error; err != nil {
			t.Fatalf("reading the state: %v", err)
		}
		if readBack.State != LatamSyncPending {
			t.Errorf("state = %q, want pending", readBack.State)
		}
	})
}
