package data

import (
	"testing"

	"gorm.io/gorm"
)

func smilesLine(day string, points int, kind, externalID string) SmilesStatementLine {
	line := SmilesStatementLine{OccurredOn: azulDay(day), Points: points, Kind: kind}
	if externalID != "" {
		line.ExternalID = &externalID
	}
	return line
}

func smilesLinesFor(t *testing.T, tx *gorm.DB, accountID string) []SmilesStatementLine {
	t.Helper()
	var out []SmilesStatementLine
	if err := tx.Where("account_id = ?", accountID).
		Order("occurred_on, points").Find(&out).Error; err != nil {
		t.Fatalf("reading back lines: %v", err)
	}
	return out
}

func TestSmilesKindFromType(t *testing.T) {
	cases := []struct {
		name        string
		typeCode    string
		subType     string
		productName string
		partnerName string
		want        string
	}{
		{"booking", "Redemption", "Product", "VOO", "", SmilesLineRedemption},
		{"case and spacing do not matter", " redemption ", "", "VOO", "", SmilesLineRedemption},
		// An estorno arrives as a credit whose legs say "Cancellation" - the only way this
		// API spells one. Reading it as an ordinary credit would price returned miles as if
		// they had been bought.
		{"estorno", "Accrual", "Cancellation", "VOO", "", SmilesLineRefund},
		// Buying miles and transferring them in are the same acquisition to the panel...
		{"partner transfer", "Accrual", "Product", "Pontos Livelo", "LIVELO S.A.", SmilesLinePurchase},
		{"miles bought", "Accrual", "", "Compra de Milhas PaGol", "PAGOL", SmilesLinePurchase},
		{"bank credit", "Accrual", "", "Créditos Inter Loop", "BANCO INTER S.A", SmilesLinePurchase},
		// ...unless it is a membership or a co-branded card, which is earned, not bought.
		{"club", "Accrual", "", "Bônus Clube Smiles Plano 5000", "Programa Smiles", SmilesLineAccrual},
		{"card", "Accrual", "", "Cartão Smiles Bradesco Visa Gold", "BANCO BRADESCO", SmilesLineAccrual},
		{"card, unaccented", "Accrual", "", "Cartao Smiles", "", SmilesLineAccrual},
		// Miles earned flying are an accrual too.
		{"flight accrual", "Accrual", "Product", "VOO", "", SmilesLineAccrual},
		{"unknown type", "Transfer", "", "", "", SmilesLineOther},
		{"empty", "", "", "", "", SmilesLineOther},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := SmilesKindFromType(c.typeCode, c.subType, c.productName, c.partnerName)
			if got != c.want {
				t.Errorf("SmilesKindFromType(%q, %q, %q, %q) = %q, want %q",
					c.typeCode, c.subType, c.productName, c.partnerName, got, c.want)
			}
		})
	}
}

func TestSmilesLineKeyPrefersTheAirlinesIdentifier(t *testing.T) {
	withID := smilesLine("2026-09-10", -27400, SmilesLineRedemption, "IOTFQF")
	if got := withID.LineKey(); got != "IOTFQF" {
		t.Errorf("LineKey() = %q, want the airline's own identifier", got)
	}

	// Without one, the key has to be derived - and must still be stable across syncs, so it
	// is built only from fields a re-read reproduces.
	withoutID := smilesLine("2026-09-10", -27400, SmilesLineRedemption, "")
	want := "sig:2026-09-10:redemption:-27400"
	if got := withoutID.LineKey(); got != want {
		t.Errorf("LineKey() = %q, want %q", got, want)
	}
	// Stability is the point: a re-sync rebuilds the row from scratch, and a key that
	// changed with it would orphan every manual link.
	rebuilt := smilesLine("2026-09-10", -27400, SmilesLineRedemption, "")
	if rebuilt.LineKey() != withoutID.LineKey() {
		t.Error("the derived key changed between two reads of the same movement")
	}
}

func TestReplaceSmilesStatementWindow_InsertsWindow(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		lines := []SmilesStatementLine{
			smilesLine("2026-09-10", -27400, SmilesLineRedemption, "IOTFQF"),
			smilesLine("2026-09-11", 300000, SmilesLinePurchase, "TX-9001"),
		}
		if err := ReplaceSmilesStatementWindow(tx, account.ID, windowStart, lines); err != nil {
			t.Fatalf("ReplaceSmilesStatementWindow: %v", err)
		}

		got := smilesLinesFor(t, tx, account.ID)
		if len(got) != 2 {
			t.Fatalf("stored %d lines, want 2", len(got))
		}
		for _, line := range got {
			if line.ID == "" {
				t.Error("stored line has no id")
			}
			if line.AccountID != account.ID {
				t.Errorf("line account = %q, want %q", line.AccountID, account.ID)
			}
		}
	})
}

func TestReplaceSmilesStatementWindow_ReplacesFromStartAndKeepsOlder(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		// History from before the window, which a replacement must not touch.
		old := smilesLine("2026-07-01", 3000, SmilesLineAccrual, "TX-OLD")
		old.AccountID = account.ID
		mustCreateRow(t, tx, &old)

		if err := ReplaceSmilesStatementWindow(tx, account.ID, windowStart,
			[]SmilesStatementLine{smilesLine("2026-08-20", 1000, SmilesLinePurchase, "TX-1")}); err != nil {
			t.Fatalf("first replace: %v", err)
		}

		// A second round whose content differs: the airline withdrew TX-1 and published two
		// others. Replacing is what lets the withdrawal propagate - an upsert would leave our
		// copy of TX-1 behind forever.
		second := []SmilesStatementLine{
			smilesLine("2026-09-10", -27400, SmilesLineRedemption, "IOTFQF"),
			smilesLine("2026-09-11", 5000, SmilesLineRefund, "CANCEL"),
		}
		if err := ReplaceSmilesStatementWindow(tx, account.ID, windowStart, second); err != nil {
			t.Fatalf("second replace: %v", err)
		}

		got := smilesLinesFor(t, tx, account.ID)
		if len(got) != 3 {
			t.Fatalf("stored %d lines, want 3 (1 preserved + 2 fresh)", len(got))
		}
		if !got[0].OccurredOn.Equal(azulDay("2026-07-01")) {
			t.Errorf("oldest line = %v, want the preserved 2026-07-01", got[0].OccurredOn)
		}
		for _, line := range got {
			if line.ExternalID != nil && *line.ExternalID == "TX-1" {
				t.Error("a movement the airline no longer publishes survived the replacement")
			}
		}
	})
}

func TestReplaceSmilesStatementWindow_EmptyClearsWindow(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		if err := ReplaceSmilesStatementWindow(tx, account.ID, windowStart,
			[]SmilesStatementLine{smilesLine("2026-09-10", -27400, SmilesLineRedemption, "IOTFQF")}); err != nil {
			t.Fatalf("seeding a window: %v", err)
		}
		if err := ReplaceSmilesStatementWindow(tx, account.ID, windowStart, nil); err != nil {
			t.Fatalf("clearing the window: %v", err)
		}
		if got := smilesLinesFor(t, tx, account.ID); len(got) != 0 {
			t.Fatalf("window not cleared: %d lines remain", len(got))
		}
	})
}

func TestAccountEncryptsTheSmilesSessionAtRest(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)

		token := "refresh-token-em-claro"
		account.SmilesRefreshToken = &token
		if err := tx.Save(&account).Error; err != nil {
			t.Fatalf("saving the session: %v", err)
		}

		// What is on disk must not be the token itself. Read raw, bypassing the AfterFind
		// hook that would decrypt it.
		var stored string
		if err := tx.Raw("SELECT smiles_refresh_token FROM accounts WHERE id = ?", account.ID).
			Scan(&stored).Error; err != nil {
			t.Fatalf("reading the raw column: %v", err)
		}
		if stored == token {
			t.Fatal("the refresh token was stored in clear")
		}

		// And what the application reads back must be the token again.
		var reloaded Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("reloading the account: %v", err)
		}
		if reloaded.SmilesRefreshToken == nil || *reloaded.SmilesRefreshToken != token {
			t.Fatalf("read back %v, want the token", reloaded.SmilesRefreshToken)
		}
	})
}
