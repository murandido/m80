package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/statement"
	"gorm.io/gorm"
)

// day is the statement's own granularity: acquired_on is a date, and the batches this test
// writes are compared by it.
func day(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("bad date in test: %v", err)
	}
	return parsed
}

// seedStatementTarget creates what a commit needs: an account that owns its point
// inventory, an origin and the agency that funds the batches.
func seedStatementTarget(t *testing.T, tx *gorm.DB) (data.Account, data.PointBatchOrigin, data.Agency) {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "Test Air", Code: "T" + suffix}
	create(t, tx, &airline)
	program := data.LoyaltyProgram{Name: "Test Program", Code: "L" + suffix, DefaultAirlineID: airline.ID}
	create(t, tx, &program)
	account := data.Account{
		HolderID:                  testHolder(t, tx),
		LoyaltyProgramID:          program.ID,
		OwnsPointInventory:        true,
		DefaultProfitSharePercent: 50,
	}
	create(t, tx, &account)
	origin := data.PointBatchOrigin{Name: "Inter " + suffix}
	create(t, tx, &origin)
	agency := data.Agency{Name: "Test Agency " + suffix}
	create(t, tx, &agency)

	return account, origin, agency
}

// A purchase reaches the statement in instalments - four credits of the same day for the
// Inter batch - and the operator wants back the one acquisition they made, not four rows.
func TestRecordBatchesSumsCreditsOfTheSameOrigin(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)
		handler := NewStatementHandler(tx)

		credits := []statement.CreditLine{
			{OccurredOn: day(t, "2026-09-04"), Points: 168750, Description: "Resgate pontos Inter", Verdict: statement.VerdictMissingInPanel},
			{OccurredOn: day(t, "2026-09-04"), Points: 168750, Description: "Resgate pontos Inter", Verdict: statement.VerdictMissingInPanel},
			{OccurredOn: day(t, "2026-09-03"), Points: 168750, Description: "Resgate pontos Inter", Verdict: statement.VerdictMissingInPanel},
		}
		selections := make([]schemas.StatementBatchSelection, 0, len(credits))
		for index := range credits {
			selections = append(selections, schemas.StatementBatchSelection{
				Index:           index,
				OriginID:        origin.ID,
				CostCents:       978750,
				FundingAgencyID: agency.ID,
			})
		}

		result, err := handler.recordBatches(account, credits, selections)
		if err != nil {
			t.Fatalf("recordBatches failed: %v", err)
		}
		if result.Created != 1 || result.Credits != 3 {
			t.Fatalf("expected 1 batch from 3 credits, got %d from %d", result.Created, result.Credits)
		}

		var batches []data.PointBatch
		if err := tx.Where("account_id = ?", account.ID).Find(&batches).Error; err != nil {
			t.Fatalf("failed to read the batches back: %v", err)
		}
		if len(batches) != 1 {
			t.Fatalf("expected one batch in the database, got %d", len(batches))
		}
		batch := batches[0]
		if batch.Points != 506250 {
			t.Errorf("points: want 506250, got %d", batch.Points)
		}
		if batch.CostCents != 2936250 {
			t.Errorf("cost: want 2936250, got %d", batch.CostCents)
		}
		// The earliest credit dates the batch: the points are then in the FIFO queue from
		// the day the first of them landed, so nothing issued in between goes uncovered.
		if got := batch.AcquiredOn.Format("2006-01-02"); got != "2026-09-03" {
			t.Errorf("acquired_on: want 2026-09-03, got %s", got)
		}
		if batch.ProfitSharePercent != 50 {
			t.Errorf("profit share: want the account's 50, got %d", batch.ProfitSharePercent)
		}
		// Where the batch came from, so the same file reconciled again finds these credits
		// already recorded instead of offering to write them a second time.
		recorded := statement.DecodeBatchLines(batch.StatementLines)
		if len(recorded) != 3 {
			t.Fatalf("provenance: want the 3 credits, got %+v", recorded)
		}
		total := 0
		for _, line := range recorded {
			total += line.Points
		}
		if total != 506250 {
			t.Errorf("provenance points: want 506250, got %d", total)
		}
	})
}

// Two origins in the same commit stay two batches: the grouping is per acquisition, not
// per upload.
func TestRecordBatchesKeepsOriginsApart(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		account, origin, agency := seedStatementTarget(t, tx)
		other := data.PointBatchOrigin{Name: "Acúmulo de voo " + uuid.New().String()[:8]}
		create(t, tx, &other)
		handler := NewStatementHandler(tx)

		credits := []statement.CreditLine{
			{OccurredOn: day(t, "2026-09-03"), Points: 1340, Description: "Acúmulo", Verdict: statement.VerdictMissingInPanel},
			{OccurredOn: day(t, "2026-09-04"), Points: 168750, Description: "Inter", Verdict: statement.VerdictMissingInPanel},
			{OccurredOn: day(t, "2026-09-05"), Points: 4456, Description: "Acúmulo", Verdict: statement.VerdictMissingInPanel},
			// Already recorded: the same file sent twice must not write it again.
			{OccurredOn: day(t, "2026-09-06"), Points: 3984, Verdict: statement.VerdictMatched},
		}
		selections := []schemas.StatementBatchSelection{
			{Index: 0, OriginID: other.ID, FundingAgencyID: agency.ID},
			{Index: 1, OriginID: origin.ID, CostCents: 978750, FundingAgencyID: agency.ID},
			{Index: 2, OriginID: other.ID, FundingAgencyID: agency.ID},
			{Index: 3, OriginID: other.ID, FundingAgencyID: agency.ID},
		}

		result, err := handler.recordBatches(account, credits, selections)
		if err != nil {
			t.Fatalf("recordBatches failed: %v", err)
		}
		if result.Created != 2 || result.Skipped != 1 || result.Credits != 3 {
			t.Fatalf("want 2 created, 1 skipped, 3 credits; got %d, %d, %d",
				result.Created, result.Skipped, result.Credits)
		}

		var accrual data.PointBatch
		if err := tx.Where("account_id = ? AND origin_id = ?", account.ID, other.ID).
			First(&accrual).Error; err != nil {
			t.Fatalf("failed to read the accrual batch: %v", err)
		}
		if accrual.Points != 5796 {
			t.Errorf("accrual points: want 5796, got %d", accrual.Points)
		}
		if accrual.CostCents != 0 {
			t.Errorf("accrual cost: want 0, got %d", accrual.CostCents)
		}
	})
}
