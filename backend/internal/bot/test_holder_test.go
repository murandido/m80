package bot

import (
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// testHolder creates a person to hold a test account. accounts.holder_id is NOT NULL since
// migration 75, so every account a test seeds needs one even when the test is not about it.
func testHolder(t *testing.T, tx *gorm.DB) *string {
	t.Helper()
	holder := data.Person{Name: "Holder " + uuid.New().String()[:8]}
	if err := tx.Create(&holder).Error; err != nil {
		t.Fatalf("failed to seed a holder: %v", err)
	}
	return &holder.ID
}
