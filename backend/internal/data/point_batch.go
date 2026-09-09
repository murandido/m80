package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// PointBatch is one acquisition of points into one account, and the unit the FIFO costing
// prices against. See migration 49 for why the cost belongs here rather than on the
// issuance that later spends it.
type PointBatch struct {
	ID        string   `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID string   `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	// OriginID is nil only on an opening balance - a remainder carried from a closed
	// period, which was not bought from anybody.
	OriginID *string           `gorm:"column:origin_id;type:uuid" json:"origin_id"`
	Origin   *PointBatchOrigin `gorm:"foreignKey:OriginID;references:ID" json:"origin,omitempty"`
	// AcquiredOn orders the FIFO queue, and is the date points refunded into this batch
	// rejoin it at: they are the same points as always, so they leave before newer ones.
	AcquiredOn time.Time `gorm:"column:acquired_on;type:date;not null" json:"acquired_on"`
	Points     int       `gorm:"not null" json:"points"`
	// CostCents is zero for a club subscription or flight accrual - a real value, not a
	// missing one. The whole sale counts as profit on those.
	CostCents int `gorm:"column:cost_cents;not null" json:"cost_cents"`
	// FundingAgencyID is the agency that paid. A settlement exists only when the agency
	// that issued is a different one, so an account whose bookings all belong to its
	// funder nets to zero without anything being configured.
	FundingAgencyID string  `gorm:"column:funding_agency_id;type:uuid;not null" json:"funding_agency_id"`
	FundingAgency   *Agency `gorm:"foreignKey:FundingAgencyID;references:ID" json:"funding_agency,omitempty"`
	// ProfitSharePercent is frozen at creation from the account's default. Reading it live
	// from the account would make a change to the arrangement rewrite every past month,
	// settled ones included.
	ProfitSharePercent int  `gorm:"column:profit_share_percent;not null" json:"profit_share_percent"`
	Opening            bool `gorm:"not null;default:false" json:"opening"`
	// StatementLines is where this batch came from, when it came from a statement: the
	// credits that were summed into it. It is what lets the same file be reconciled twice
	// without offering to record everything again - a summed batch no longer looks like
	// any single credit, so date and amount alone cannot find it. NULL on a batch entered
	// by hand. See migration 52.
	StatementLines datatypes.JSON `gorm:"column:statement_lines;type:jsonb" json:"statement_lines,omitempty"`
	// SettledAt is the spreadsheet's "ACERTADO", dated instead of yes/no.
	SettledAt *time.Time `gorm:"column:settled_at" json:"settled_at"`
	Note      *string    `json:"note"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	// Archiving rather than a trash can, for the same reason accounts has one: a batch
	// that priced an issuance is referenced ON DELETE RESTRICT by its allocations, so a
	// real delete on it is refused forever. There is no restore screen.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (b *PointBatch) BeforeCreate(tx *gorm.DB) error {
	if b.ID == "" {
		b.ID = uuid.New().String()
	}
	return nil
}

// CostOfTheThousand is what one thousand of this batch's points cost, in centavos, as a
// float because the rate is genuinely fractional: 8941,72 over 632.700 points is
// 14,132638 per thousand, and rounding that to a whole centavo is precisely the error the
// allocations table exists to avoid.
//
// For display and for reconciling against the spreadsheet only. Never price a consumed
// slice by multiplying this - internal/inventory does it from the running total so the
// slices of a fully consumed batch add back up to CostCents exactly.
//
// Zero points cannot happen - the CHECK in migration 49 refuses them - but a zero return
// is still the right answer to an empty batch rather than a panic.
func (b *PointBatch) CostOfTheThousand() float64 {
	if b.Points == 0 {
		return 0
	}
	return float64(b.CostCents) * 1000 / float64(b.Points)
}
