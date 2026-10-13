package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// IssuanceAdjustment is a change to a ticket after it was issued: extra miles or money the
// airline charged to move a segment, or miles and money it gave back. See migration 82 for
// why it is a dated movement and not an edit to the issuance's own points and taxes.
type IssuanceAdjustment struct {
	ID         string    `gorm:"primaryKey;type:uuid" json:"id"`
	IssuanceID string    `gorm:"column:issuance_id;type:uuid;not null" json:"issuance_id"`
	Issuance   *Issuance `gorm:"foreignKey:IssuanceID;references:ID" json:"issuance,omitempty"`
	// OccurredOn is the day the airline charged or returned. It is what puts the movement
	// in a period, the same way cancelled_at does for a refund.
	OccurredOn time.Time `gorm:"column:occurred_on;type:date;not null" json:"occurred_on"`
	// Points and Taxes are signed: positive is a charge, negative a return.
	Points int `gorm:"not null;default:0" json:"points"`
	Taxes  int `gorm:"not null;default:0" json:"taxes"`
	// ValueOfTheThousand is what the client pays (or gets back) per thousand of these
	// points. Zero means the agency absorbed the charge or kept the refund.
	ValueOfTheThousand  int            `gorm:"column:value_of_the_thousand;not null;default:0" json:"value_of_the_thousand"`
	TaxesPassedToClient bool           `gorm:"column:taxes_passed_to_client;not null;default:false" json:"taxes_passed_to_client"`
	Reason              string         `gorm:"type:text;not null" json:"reason"`
	CreatedByID         string         `gorm:"column:created_by_id;type:uuid;not null" json:"created_by_id"`
	CreatedBy           *User          `gorm:"foreignKey:CreatedByID;references:ID" json:"created_by,omitempty"`
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	DeletedAt           gorm.DeletedAt `gorm:"index" json:"-"`
}

func (a *IssuanceAdjustment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}
