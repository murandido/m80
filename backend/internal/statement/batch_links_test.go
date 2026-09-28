package statement

import (
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// seedBatch records a batch on the fixture's account, carrying the given credits as its
// provenance (the day/points the auto match keys on).
func seedBatch(t *testing.T, tx *gorm.DB, f linkFixture, acquiredOn string, points, costCents int, credits []BatchLine) data.PointBatch {
	t.Helper()

	origin := data.PointBatchOrigin{Name: "Livelo " + uuid.New().String()[:8]}
	mustCreate(t, tx, &origin)

	encoded, err := EncodeBatchLines(credits)
	if err != nil {
		t.Fatalf("encoding batch lines: %v", err)
	}
	batch := data.PointBatch{
		AccountID:       f.account.ID,
		OriginID:        &origin.ID,
		AcquiredOn:      day(acquiredOn),
		Points:          points,
		CostCents:       costCents,
		FundingAgencyID: f.agency.ID,
		StatementLines:  encoded,
	}
	mustCreate(t, tx, &batch)
	return batch
}

func TestResolveBatchLinks_AutoMatchesByDayAndPoints(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		// A batch summed from two credits; the purchase line matches one of them.
		batch := seedBatch(t, tx, f, "2026-08-27", 200000, 300000, []BatchLine{
			{OccurredOn: day("2026-08-27"), Points: 158175},
			{OccurredOn: day("2026-08-27"), Points: 41825},
		})

		resolved, err := ResolveBatchLinks(tx, f.account.ID, []BatchLineRef{
			{Key: "sig:2026-08-27:ER:158175", OccurredOn: day("2026-08-27"), Points: 158175},
		})
		if err != nil {
			t.Fatalf("ResolveBatchLinks: %v", err)
		}
		got, ok := resolved["sig:2026-08-27:ER:158175"]
		if !ok {
			t.Fatal("the purchase line was not matched to its batch")
		}
		if got.Batch.ID != batch.ID {
			t.Errorf("matched batch = %q, want %q", got.Batch.ID, batch.ID)
		}
		if got.Manual {
			t.Error("an auto match reported Manual = true")
		}
		if got.Batch.Origin == nil {
			t.Error("the batch's origin was not preloaded")
		}
	})
}

func TestResolveBatchLinks_UnmatchedIsAbsent(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		seedBatch(t, tx, f, "2026-08-27", 158175, 200000, []BatchLine{
			{OccurredOn: day("2026-08-27"), Points: 158175},
		})

		resolved, err := ResolveBatchLinks(tx, f.account.ID, []BatchLineRef{
			{Key: "sig:2026-08-28:ER:999999", OccurredOn: day("2026-08-28"), Points: 999999},
		})
		if err != nil {
			t.Fatalf("ResolveBatchLinks: %v", err)
		}
		if _, ok := resolved["sig:2026-08-28:ER:999999"]; ok {
			t.Error("an unmatched line was resolved; it should be absent")
		}
	})
}

func TestResolveBatchLinks_ManualPinWinsAndClears(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedLinkFixture(t, tx)
		auto := seedBatch(t, tx, f, "2026-08-27", 158175, 200000, []BatchLine{
			{OccurredOn: day("2026-08-27"), Points: 158175},
		})
		other := seedBatch(t, tx, f, "2026-09-01", 50000, 80000, nil)

		key := "sig:2026-08-27:ER:158175"
		ref := BatchLineRef{Key: key, OccurredOn: day("2026-08-27"), Points: 158175}

		if err := UpsertBatchLink(tx, f.account.ID, key, other.ID, &f.user.ID); err != nil {
			t.Fatalf("UpsertBatchLink: %v", err)
		}
		resolved, err := ResolveBatchLinks(tx, f.account.ID, []BatchLineRef{ref})
		if err != nil {
			t.Fatalf("ResolveBatchLinks after pin: %v", err)
		}
		if got := resolved[key]; got.Batch.ID != other.ID || !got.Manual {
			t.Errorf("after pin = %q manual=%v, want %q manual=true", got.Batch.ID, got.Manual, other.ID)
		}

		if err := DeleteBatchLink(tx, f.account.ID, key); err != nil {
			t.Fatalf("DeleteBatchLink: %v", err)
		}
		resolved, err = ResolveBatchLinks(tx, f.account.ID, []BatchLineRef{ref})
		if err != nil {
			t.Fatalf("ResolveBatchLinks after clear: %v", err)
		}
		if got := resolved[key]; got.Batch.ID != auto.ID || got.Manual {
			t.Errorf("after clear = %q manual=%v, want the auto %q manual=false", got.Batch.ID, got.Manual, auto.ID)
		}
	})
}
