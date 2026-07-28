package issuance

import (
	"errors"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func TestDelete_SoftDeletesAndRemovesSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedFixture(t, tx, data.Airline{})
		issuance, err := Create(tx, f.input(), f.User.ID)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := Delete(tx, issuance, f.User.ID); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}

		var persisted data.Issuance
		err = tx.First(&persisted, "id = ?", issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("expected the default scope to hide the deleted issuance, got err=%v", err)
		}

		var deletedAt struct {
			DeletedAt *string
		}
		if err := tx.Unscoped().Table("issuances").Select("deleted_at").Where("id = ?", issuance.ID).Scan(&deletedAt).Error; err != nil {
			t.Fatalf("failed to read deleted_at: %v", err)
		}
		if deletedAt.DeletedAt == nil {
			t.Error("expected deleted_at to be set")
		}

		var sale data.Sale
		err = tx.First(&sale, "issuance_id = ?", issuance.ID).Error
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("expected the sale to be deleted, got err=%v sale=%+v", err, sale)
		}

		assertCount(t, tx, "audit_logs", "issuance_id = ? AND action = 'deleted'", issuance.ID, 1)
	})
}
