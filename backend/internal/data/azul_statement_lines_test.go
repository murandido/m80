package data

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// seedAzulAccount creates the minimal chain an account needs (airline, program, holder) and
// returns the account, for the window-replacement tests.
func seedAzulAccount(t *testing.T, tx *gorm.DB) Account {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := Airline{Name: "Test Air " + suffix, Code: "T" + suffix}
	mustCreateRow(t, tx, &airline)

	program := LoyaltyProgram{Name: "Programa " + suffix, Code: "P" + suffix, DefaultAirlineID: airline.ID}
	mustCreateRow(t, tx, &program)

	holder := Person{Name: "Titular " + suffix}
	mustCreateRow(t, tx, &holder)

	account := Account{LoyaltyProgramID: program.ID, HolderID: &holder.ID, OwnsPointInventory: true}
	mustCreateRow(t, tx, &account)
	return account
}

func azulDay(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func line(day string, points int, kind string) AzulStatementLine {
	return AzulStatementLine{OccurredOn: azulDay(day), Points: points, Kind: kind}
}

func linesFor(t *testing.T, tx *gorm.DB, accountID string) []AzulStatementLine {
	t.Helper()
	var out []AzulStatementLine
	if err := tx.Where("account_id = ?", accountID).
		Order("occurred_on, points").Find(&out).Error; err != nil {
		t.Fatalf("reading back lines: %v", err)
	}
	return out
}

func runInAzulTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("beginning the test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })
	fn(tx)
}

func TestAzulKindFromTypeCode(t *testing.T) {
	type tc struct {
		code    string
		context string
		comment string
		want    string
	}
	cases := []tc{
		{"AN", "", "Booking: ABC123", AzulLineRedemption},
		{"ai", "", "", AzulLineAccrual}, // case-insensitive
		{"RF", "", "PNR LP52SA", AzulLineRefund},
		{" PE ", "", "", AzulLineExpiration}, // trimmed
		// ER is a "Compra": a bank transfer and a campaign credit are the same acquisition...
		{"ER", "Parceiro Banco Inter", "Resgate pontos Inter", AzulLinePurchase},
		{"ER", "Pontos Livelo", "", AzulLinePurchase},
		{"ER", "ALLOYAL", "crédito de pontos em campanha bonificada", AzulLinePurchase},
		// ...unless the partner is a club/card, which is an accrual.
		{"ER", "Clube TudoAzul", "", AzulLineAccrual},
		{"ER", "Cartão Azul Itaú", "", AzulLineAccrual},
		// PC: a correction of a promotion is a Compra (points back in); an ordinary one is other.
		{"PC", "Azul", "Liberalidade - promoção B088OPC690", AzulLinePurchase},
		{"PC", "Azul", "ajuste operacional", AzulLineOther},
		// OI: its comment is empty, so it stays other even though its description names a campaign.
		{"OI", "Aceite de Campanha Promocional", "", AzulLineOther},
		{"ZZ", "", "", AzulLineOther},
		{"", "", "", AzulLineOther},
	}
	for _, c := range cases {
		if got := AzulKindFromTypeCode(c.code, c.context, c.comment); got != c.want {
			t.Errorf("AzulKindFromTypeCode(%q, %q, %q) = %q, want %q", c.code, c.context, c.comment, got, c.want)
		}
	}
}

func TestReplaceAzulStatementWindow_InsertsWindow(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		lines := []AzulStatementLine{
			line("2026-09-10", -20000, AzulLineRedemption),
			line("2026-09-10", 5000, AzulLineAccrual),
			line("2026-08-20", -1000, AzulLineExpiration),
		}
		if err := ReplaceAzulStatementWindow(tx, account.ID, windowStart, lines); err != nil {
			t.Fatalf("ReplaceAzulStatementWindow: %v", err)
		}

		got := linesFor(t, tx, account.ID)
		if len(got) != 3 {
			t.Fatalf("stored %d lines, want 3", len(got))
		}
		for _, l := range got {
			if l.ID == "" {
				t.Error("stored line has no id")
			}
			if l.AccountID != account.ID {
				t.Errorf("line account = %q, want %q", l.AccountID, account.ID)
			}
		}
	})
}

func TestReplaceAzulStatementWindow_ReplacesFromStartAndKeepsOlder(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		// An old line, before the window, written directly so it predates windowStart. A real
		// wider window would have inserted it; here it stands in for "already stored history".
		old := line("2026-07-01", 3000, AzulLineAccrual)
		old.AccountID = account.ID
		mustCreateRow(t, tx, &old)

		// First sync of the window itself.
		if err := ReplaceAzulStatementWindow(tx, account.ID, windowStart,
			[]AzulStatementLine{line("2026-08-20", -1000, AzulLineExpiration)}); err != nil {
			t.Fatalf("first replace: %v", err)
		}

		// Second sync of the same window: different content. Everything >= windowStart is
		// swapped; the 2026-07-01 line, older than windowStart, must survive.
		second := []AzulStatementLine{
			line("2026-09-10", -20000, AzulLineRedemption),
			line("2026-09-10", -20000, AzulLineRedemption), // identical row kept, not deduped
		}
		if err := ReplaceAzulStatementWindow(tx, account.ID, windowStart, second); err != nil {
			t.Fatalf("second replace: %v", err)
		}

		got := linesFor(t, tx, account.ID)
		if len(got) != 3 {
			t.Fatalf("stored %d lines, want 3 (1 preserved + 2 fresh)", len(got))
		}

		// The oldest survivor is the pre-window line; the two identical redemptions both remain.
		if !got[0].OccurredOn.Equal(azulDay("2026-07-01")) {
			t.Errorf("oldest line = %v, want the preserved 2026-07-01", got[0].OccurredOn)
		}
		redemptions := 0
		for _, l := range got {
			if l.Kind == AzulLineRedemption {
				redemptions++
			}
		}
		if redemptions != 2 {
			t.Errorf("kept %d identical redemptions, want 2 (no dedup)", redemptions)
		}
	})
}

func TestReplaceAzulStatementWindow_EmptyClearsWindow(t *testing.T) {
	runInAzulTx(t, func(tx *gorm.DB) {
		account := seedAzulAccount(t, tx)
		windowStart := azulDay("2026-08-13")

		if err := ReplaceAzulStatementWindow(tx, account.ID, windowStart,
			[]AzulStatementLine{line("2026-09-10", -20000, AzulLineRedemption)}); err != nil {
			t.Fatalf("seeding a window: %v", err)
		}
		if err := ReplaceAzulStatementWindow(tx, account.ID, windowStart, nil); err != nil {
			t.Fatalf("clearing the window: %v", err)
		}
		if got := linesFor(t, tx, account.ID); len(got) != 0 {
			t.Fatalf("window not cleared: %d lines remain", len(got))
		}
	})
}
