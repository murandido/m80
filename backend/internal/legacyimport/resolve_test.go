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

// estornoRow is the shape the operators write a cancellation in: the issuance again, with
// its signs flipped, on a row of its own.
func estornoRow(programLabel, locator, accountLabel string, points int) Row {
	row := sampleRow(programLabel, locator, accountLabel)
	row.Points = -points
	return row
}

// seedIssuance puts an issuance in the database the way the panel would have, so a
// cancellation row has something to point at.
func seedIssuance(t *testing.T, tx *gorm.DB, w world, account data.Account, locator string, points int) data.Issuance {
	t.Helper()

	issuance := data.Issuance{
		LoyaltyProgramID:   w.program.ID,
		AirlineID:          w.airline.ID,
		AgencyID:           w.agency.ID,
		AccountID:          account.ID,
		IssuerID:           w.user.ID,
		ProgramLocator:     locator,
		Points:             points,
		ValueOfTheThousand: 1625,
		Active:             true,
	}
	mustCreate(t, tx, &issuance)
	return issuance
}

func TestResolve_ReadsANegativeLineAsACancellationOfTheIssuanceItNames(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "SMILES")
		account := seedAccount(t, tx, w, "Gustavo Pinheiro", "GUSTAVO PINHEIRO")
		issuance := seedIssuance(t, tx, w, account, "OZNHZS", 62400)

		row := estornoRow(w.program.Code, "OZNHZS", "GUSTAVO PINHEIRO", 62400)
		taxes := -10288
		row.Taxes = &taxes

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictCancellation {
			t.Fatalf("verdict = %s (%s), want cancellation", resolved.Verdict, resolved.Reason)
		}
		plan := resolved.Cancellation
		if plan == nil {
			t.Fatal("cancellation = nil, want the plan")
		}
		if plan.Action != CancelExisting {
			t.Errorf("action = %s, want cancel_existing", plan.Action)
		}
		if plan.TargetIssuanceID != issuance.ID {
			t.Errorf("target = %q, want %q", plan.TargetIssuanceID, issuance.ID)
		}
		if plan.RefundedPoints != 62400 {
			t.Errorf("refunded points = %d, want 62400", plan.RefundedPoints)
		}
		if plan.RefundedTaxes == nil || *plan.RefundedTaxes != 10288 {
			t.Errorf("refunded taxes = %v, want 10288", plan.RefundedTaxes)
		}
		if plan.Partial {
			t.Error("partial = true, want false - the estorno returns the whole booking")
		}
		// The row itself reads as the issuance, which is what the operator is looking at.
		if resolved.Points != 62400 {
			t.Errorf("points = %d, want the positive 62400", resolved.Points)
		}
	})
}

func TestResolve_CreatesAndCancelsWhenTheIssuanceWasNeverImported(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Helles Moura", "HELLES")

		// The day this booking was issued was never imported - it is only in the
		// spreadsheet - so there is nothing in the database to cancel.
		row := estornoRow(w.program.Code, "EWI4NK", "HELLES", 24000)

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictCancellation {
			t.Fatalf("verdict = %s (%s), want cancellation", resolved.Verdict, resolved.Reason)
		}
		if resolved.Cancellation.Action != CreateAndCancel {
			t.Errorf("action = %s, want create_and_cancel", resolved.Cancellation.Action)
		}
		if resolved.Cancellation.TargetIssuanceID != "" {
			t.Errorf("target = %q, want none", resolved.Cancellation.TargetIssuanceID)
		}
		if resolved.Cancellation.OriginalPoints != nil {
			t.Errorf("original points = %v, want nil", resolved.Cancellation.OriginalPoints)
		}
	})
}

func TestResolve_FlagsAPartialEstornoWithoutRefusingIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Cleidiane Souza", "CLEIDIANE")
		seedIssuance(t, tx, w, account, "CNPZHA", 140000)

		// One leg of a round trip came back. The panel has no half-cancelled state, so
		// the whole issuance is cancelled and the operator is told what they are doing.
		row := estornoRow(w.program.Code, "CNPZHA", "CLEIDIANE", 40000)

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictCancellation {
			t.Fatalf("verdict = %s (%s), want cancellation", resolved.Verdict, resolved.Reason)
		}
		if !resolved.Cancellation.Partial {
			t.Error("partial = false, want true")
		}
		if got := resolved.Cancellation.OriginalPoints; got == nil || *got != 140000 {
			t.Errorf("original points = %v, want 140000", got)
		}
		if !hasNote(resolved.Notes, "parcial") {
			t.Errorf("notes = %v, want one about the partial estorno", resolved.Notes)
		}
	})
}

func TestResolve_SkipsAnEstornoWhoseIssuanceIsAlreadyCancelled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Luis Prado", "LUIS")
		issuance := seedIssuance(t, tx, w, account, "JLCTWZ", 12000)

		cancelledAt := time.Now()
		issuance.Active = false
		issuance.CancelledAt = &cancelledAt
		if err := tx.Save(&issuance).Error; err != nil {
			t.Fatalf("cancelling the seeded issuance: %v", err)
		}

		row := estornoRow(w.program.Code, "JLCTWZ", "LUIS", 12000)

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		// This is what makes running the same day twice safe: cancelling releases the
		// locator from the partial unique index, so nothing in the database would stop
		// a second cancelled issuance from being written.
		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictAlreadyImported {
			t.Fatalf("verdict = %s, want already_imported", resolved.Verdict)
		}
		if !strings.Contains(resolved.Reason, "cancelada") {
			t.Errorf("reason = %q, want it to say the issuance is already cancelled", resolved.Reason)
		}
	})
}

func TestResolve_PairsAnIssuanceWithItsEstornoInTheSameDay(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Carlos Costa", "CARLOS")

		// Issued and cancelled on the same sheet, which is how a same-day cancellation
		// is written. The second row is not a repeat of the first - it undoes it.
		issued := sampleRow(w.program.Code, "UM1FHZ", "CARLOS")
		estorno := estornoRow(w.program.Code, "UM1FHZ", "CARLOS", issued.Points)
		estorno.Number = 4

		preview, err := Resolve(tx, dayWith(w.user.Name, issued, estorno),
			Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		rows := preview.Blocks[0].Rows
		if rows[0].Verdict != VerdictOk {
			t.Fatalf("first row = %s (%s), want ok", rows[0].Verdict, rows[0].Reason)
		}
		if rows[1].Verdict != VerdictCancellation {
			t.Fatalf("second row = %s (%s), want cancellation", rows[1].Verdict, rows[1].Reason)
		}
		if rows[1].Cancellation.Action != CancelExisting {
			t.Errorf("action = %s, want cancel_existing", rows[1].Cancellation.Action)
		}
		if rows[1].Cancellation.TargetIssuanceID != "" {
			t.Error("target id set, want none - the issuance does not exist yet")
		}
	})
}

func TestResolve_StillRefusesANegativeMilheiroOnAPositiveRow(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// Points are what says "estorno". A negative price next to positive points is a
		// typo, and importing it would put a negative sale in the books.
		row := sampleRow(w.program.Code, "EQF8VK", "MARISE")
		row.ValueOfTheThousand = -1625

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictInvalid {
			t.Fatalf("verdict = %s, want invalid", resolved.Verdict)
		}
		if resolved.Cancellation != nil {
			t.Error("cancellation set, want nil")
		}
	})
}

func TestResolve_KeepsAPositiveRowWhoseObservationMentionsACancellation(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		seedAccount(t, tx, w, "Denilson Rocha", "DENILSON")

		// The operators write the word on the original line too, and about 100 rows of
		// the four workbooks look like this. The row is a normal issuance.
		row := sampleRow(w.program.Code, "KK4SQM", "DENILSON")
		row.Observation = "EMITIDA EM 02/09/25 E CANCELADA EM 02/09/25"

		preview, err := Resolve(tx, dayWith(w.user.Name, row), Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		resolved := onlyRow(t, preview)
		if resolved.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", resolved.Verdict, resolved.Reason)
		}
		if !hasNote(resolved.Notes, "observação fala em cancelamento") {
			t.Errorf("notes = %v, want one about the observation", resolved.Notes)
		}
	})
}

func TestResolve_ImportsALocatorWhoseOnlyIssuanceIsCancelled(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Isaias Melo", "ISAIAS")
		issuance := seedIssuance(t, tx, w, account, "YJEUPS", 12000)

		cancelledAt := time.Now()
		issuance.Active = false
		issuance.CancelledAt = &cancelledAt
		if err := tx.Save(&issuance).Error; err != nil {
			t.Fatalf("cancelling the seeded issuance: %v", err)
		}

		// A cancellation frees the locator - the unique index is partial on purpose -
		// so a row carrying it is a new booking, not a re-import.
		day := dayWith(w.user.Name, sampleRow(w.program.Code, "YJEUPS", "ISAIAS"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		if resolved := onlyRow(t, preview); resolved.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", resolved.Verdict, resolved.Reason)
		}
	})
}

func hasNote(notes []string, fragment string) bool {
	for _, note := range notes {
		if strings.Contains(note, fragment) {
			return true
		}
	}
	return false
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

func TestResolve_TakesTheDefaultProgramWhenTheRowHasNone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")
		account := seedAccount(t, tx, w, "Marise Andrade", "MARISE")

		// Two of the operators' workbooks have no Companhia column at all, so every row
		// of theirs arrives with an empty label and the program comes from the screen.
		day := dayWith(w.user.Name, sampleRow("", "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{
			AgencyID:                w.agency.ID,
			DefaultLoyaltyProgramID: w.program.ID,
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictOk {
			t.Fatalf("verdict = %s (%s), want ok", row.Verdict, row.Reason)
		}
		if row.LoyaltyProgramID != w.program.ID {
			t.Errorf("program = %q, want the default %q", row.LoyaltyProgramID, w.program.ID)
		}
		// The alias resolution reads the program too, and a row that got its program
		// only in resolveRow would come back needing a mapping nobody can make.
		if row.AccountID != account.ID {
			t.Errorf("account = %q, want %q", row.AccountID, account.ID)
		}
	})
}

func TestResolve_RefusesARowWithNeitherProgramNorDefault(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")

		day := dayWith(w.user.Name, sampleRow("", "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{AgencyID: w.agency.ID})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictInvalid {
			t.Fatalf("verdict = %s, want invalid", row.Verdict)
		}
		if !strings.Contains(row.Reason, "programa padrão") {
			t.Errorf("reason = %q, want it to point at the missing default", row.Reason)
		}
	})
}

func TestResolve_DoesNotRescueAnUnknownProgramWithTheDefault(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		w := seedWorld(t, tx, "AZUL")

		// A label that is filled in and names nothing is a typo. Filing it under the
		// default would put the issuance on the wrong program with nothing to notice it.
		day := dayWith(w.user.Name, sampleRow("AZUUL", "EQF8VK", "MARISE"))

		preview, err := Resolve(tx, day, Mappings{
			AgencyID:                w.agency.ID,
			DefaultLoyaltyProgramID: w.program.ID,
		})
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}

		row := onlyRow(t, preview)
		if row.Verdict != VerdictInvalid {
			t.Fatalf("verdict = %s, want invalid", row.Verdict)
		}
		if row.LoyaltyProgramID != "" {
			t.Errorf("program = %q, want none", row.LoyaltyProgramID)
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
