package legacyimport

import (
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// dayWith builds a one-block day around the given rows, so each test states only what it
// is about.
func dayWith(issuerName string, rows ...Row) Day {
	return Day{
		Date:      time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC),
		SheetName: "01-08-26",
		Blocks:    []Block{{Index: 0, IssuerName: issuerName, Rows: rows}},
	}
}

func sampleRow(programLabel, locator, accountLabel string) Row {
	return Row{
		Number:             3,
		ProgramLabel:       programLabel,
		ProgramLocator:     locator,
		PassengerName:      "RENAN RIBEIRO",
		Points:             18000,
		ValueOfTheThousand: 1625,
		AccountLabel:       accountLabel,
	}
}

// onlyRow is the single resolved row of a single-block preview.
func onlyRow(t *testing.T, preview DayPreview) ResolvedRow {
	t.Helper()
	if len(preview.Blocks) != 1 || len(preview.Blocks[0].Rows) != 1 {
		t.Fatalf("expected exactly one row, got %+v", preview.Blocks)
	}
	return preview.Blocks[0].Rows[0]
}

func TestResolve_MatchesTheAccountAliasIgnoringCaseAndAccents(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Wilmara Conceição", "Conceição")

		// The spreadsheet spells it without the cedilla and in caps, which before the
		// unaccent work would have read as an account nobody registered.
		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "CONCEICAO"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", row.Verdict, row.Reason)
		}
		if row.AccountID != account.ID {
			t.Errorf("account = %q, want %q", row.AccountID, account.ID)
		}
		if row.AccountNicknameID == nil {
			t.Error("account nickname id = nil, want the alias the row matched")
		}
		if len(preview.PendingAccounts) != 0 {
			t.Errorf("pending accounts = %+v, want none", preview.PendingAccounts)
		}
	})
}

func TestResolve_MatchesTheAccountByItsHoldersName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "SMILES")
		// No alias registered: many accounts only ever had the holder's name, which is
		// what the operators wrote in the Conta column.
		account := seedAccount(t, tx, w, "Marise Andrade", "")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "LNS6MD", "marise andrade"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.AccountID != account.ID {
			t.Fatalf("account = %q, want %q (matched by holder name)", row.AccountID, account.ID)
		}
		if row.AccountNicknameID != nil {
			t.Errorf("account nickname id = %v, want nil - no alias was involved", row.AccountNicknameID)
		}
	})
}

func TestResolve_ReportsAnUnknownAliasAsPendingRatherThanCreatingIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")

		day := dayWith(w.user.Name,
			sampleRow(w.program.Code, "AJSD2Z", "WESLEY"),
			sampleRow(w.program.Code, "HSK87B", "WESLEY"),
		)

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if len(preview.PendingAccounts) != 1 {
			t.Fatalf("pending accounts = %+v, want exactly one entry", preview.PendingAccounts)
		}
		pending := preview.PendingAccounts[0]
		if pending.AccountLabel != "WESLEY" {
			t.Errorf("label = %q, want WESLEY", pending.AccountLabel)
		}
		// Two rows use the alias, and the operator maps it once.
		if pending.RowCount != 2 {
			t.Errorf("row count = %d, want 2", pending.RowCount)
		}
		for _, row := range preview.Blocks[0].Rows {
			if row.Verdict != VerdictNeedsMapping {
				t.Errorf("verdict = %s, want needs_mapping", row.Verdict)
			}
		}

		// And nothing was created behind the operator's back - which is the whole
		// difference from how the bot handles an unknown alias.
		var accounts int64
		tx.Model(&data.Account{}).Where("loyalty_program_id = ?", w.program.ID).Count(&accounts)
		if accounts != 0 {
			t.Errorf("accounts created = %d, want 0", accounts)
		}
	})
}

func TestResolve_TakesTheOperatorsAccountMapping(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Wesley Lima", "")

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "AJSD2Z", "WESLEY (ELI)"))

		preview, err := Resolve(tx, day, Mappings{
			AgencyID: w.agency.ID,
			Accounts: []AccountMapping{{
				ProgramCode:  w.program.Code,
				AccountLabel: "WESLEY (ELI)",
				AccountID:    account.ID,
			}},
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", row.Verdict, row.Reason)
		}
		if row.AccountID != account.ID {
			t.Errorf("account = %q, want %q", row.AccountID, account.ID)
		}
		if len(preview.PendingAccounts) != 0 {
			t.Errorf("pending accounts = %+v, want none once mapped", preview.PendingAccounts)
		}
	})
}

func TestResolve_AsksAboutAnIssuerItCannotName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// A block titled after two people is not a lookup, it is a decision.
		day := dayWith("WILMARA/CLARA", sampleRow(w.program.Code, "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if len(preview.PendingIssuers) != 1 || preview.PendingIssuers[0].IssuerName != "WILMARA/CLARA" {
			t.Fatalf("pending issuers = %+v, want WILMARA/CLARA", preview.PendingIssuers)
		}
		if row := onlyRow(t, preview); row.Verdict != VerdictNeedsMapping {
			t.Errorf("verdict = %s, want needs_mapping", row.Verdict)
		}

		// Mapped, the same day resolves.
		preview, err = Resolve(tx, day, Mappings{
			AgencyID: w.agency.ID,
			Issuers:  []IssuerMapping{{IssuerName: "WILMARA/CLARA", UserID: w.user.ID}},
		})
		if err != nil {
			t.Fatalf("Resolve after mapping: %v", err)
		}
		row := onlyRow(t, preview)
		if row.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", row.Verdict, row.Reason)
		}
		if row.IssuerID != w.user.ID {
			t.Errorf("issuer = %q, want %q", row.IssuerID, w.user.ID)
		}
	})
}

func TestResolve_SuggestsAUserWhoseFirstNameMatches(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// The sheet writes the first name in caps; the user is registered in full. A
		// suggestion, not an answer - the operator still confirms it.
		day := dayWith("MARCUS", sampleRow(w.program.Code, "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if len(preview.PendingIssuers) != 1 {
			t.Fatalf("pending issuers = %+v, want one", preview.PendingIssuers)
		}
		if preview.PendingIssuers[0].SuggestedUserID != w.user.ID {
			t.Errorf("suggestion = %q, want %q", preview.PendingIssuers[0].SuggestedUserID, w.user.ID)
		}
	})
}

func TestResolve_SkipsALocatorTheDatabaseAlreadyHas(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// Stands for a booking somebody already registered through the panel - which
		// is exactly the case the operators asked not to import twice.
		mustCreate(t, tx, &data.Issuance{
			LoyaltyProgramID:   w.program.ID,
			AirlineID:          w.airline.ID,
			ProgramLocator:     "EQF8VK",
			Points:             18000,
			ValueOfTheThousand: 1625,
			AccountID:          account.ID,
			IssuerID:           w.user.ID,
			AgencyID:           w.agency.ID,
			Active:             true,
		})

		day := dayWith(w.user.Name, sampleRow(w.program.Code, "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictAlreadyImported {
			t.Fatalf("verdict = %s (%s), want already_imported", row.Verdict, row.Reason)
		}
	})
}

func TestResolve_OffersOnlyTheFirstOfARepeatedLocator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		day := dayWith(w.user.Name,
			sampleRow(w.program.Code, "NVUBSV", "MARISE"),
			sampleRow(w.program.Code, "NVUBSV", "MARISE"),
		)

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		rows := preview.Blocks[0].Rows
		if rows[0].Verdict != VerdictOk {
			t.Errorf("first row = %s (%s), want ok", rows[0].Verdict, rows[0].Reason)
		}
		// The unique index over active locators would refuse the second anyway; saying
		// so up front beats a failed write halfway through the day.
		if rows[1].Verdict != VerdictDuplicateInFile {
			t.Errorf("second row = %s, want duplicate_in_file", rows[1].Verdict)
		}
	})
}

func TestResolve_KeepsARowWhoseAirportIsNotRegistered(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		row := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		// One cell in the real workbook reads "MEXICO". The route is not what makes an
		// issuance, so it costs the cell and not the row.
		row.OriginLabel = "MEXICO"
		day := dayWith(w.user.Name, row)

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", resolved.Verdict, resolved.Reason)
		}
		if resolved.OriginIATACode != "" {
			t.Errorf("origin = %q, want empty", resolved.OriginIATACode)
		}
		if len(resolved.Notes) == 0 {
			t.Error("notes are empty: the dropped airport has to be visible somewhere")
		}
	})
}

func TestResolve_RefusesANegativeLineAsACancellation(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "SMILES")
		seedAccount(t, tx, w, "Gustavo Pinheiro", "GUSTAVO PINHEIRO")

		// Straight out of the real workbook: the operators write an estorno as the
		// issuance with its signs flipped, on a row of its own ("CANCELAMENTO / CAINDO
		// EM ATÉ 72 HORAS"). The panel has no negative issuance - a cancellation is
		// something done to an existing one.
		row := sampleRow(w.program.Code, "OZNHZS", "GUSTAVO PINHEIRO")
		row.Points = -62400
		taxes := -10288
		row.Taxes = &taxes
		day := dayWith(w.user.Name, row)

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictInvalid {
			t.Fatalf("verdict = %s, want invalid", resolved.Verdict)
		}
		// The reason has to name the cancellation: "no points" would send the operator
		// looking for a missing cell that is not missing.
		if !strings.Contains(resolved.Reason, "cancelamento") {
			t.Errorf("reason = %q, want it to name the cancellation", resolved.Reason)
		}
	})
}

func TestResolve_RefusesARowWithNoProgram(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")

		day := dayWith(w.user.Name, sampleRow("PROGRAMA INEXISTENTE", "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if row := onlyRow(t, preview); row.Verdict != VerdictInvalid {
			t.Fatalf("verdict = %s, want invalid", row.Verdict)
		}
	})
}

func TestResolve_FallsBackToTheDefaultAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")

		// The spreadsheet has no agency column at all, so "no choice" has to land
		// somewhere sensible - the same default issuance.Create falls back to.
		main := data.Agency{Name: "Principal " + w.suffix, IsDefault: true}
		mustCreate(t, tx, &main)

		preview, err := Resolve(tx, dayWith(w.user.Name), Mappings{})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if preview.AgencyID != main.ID {
			t.Errorf("agency = %q, want the default %q", preview.AgencyID, main.ID)
		}
	})
}

func TestProgramKey(t *testing.T) {
	tests := []struct{ label, want string }{
		{"A.A", "AA"},
		{"a.a.", "AA"},
		{"AZUL", "AZUL"},
		{" latam pass ", "LATAMPASS"},
	}
	for _, tc := range tests {
		if got := programKey(tc.label); got != tc.want {
			t.Errorf("programKey(%q) = %q, want %q", tc.label, got, tc.want)
		}
	}
}
