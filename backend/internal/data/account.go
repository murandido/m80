package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/utils"
	"gorm.io/gorm"
)

// The categories an account can be in. Stored in English like every other enum in the
// schema (one_way, outbound, created); the Portuguese labels live only on the screen.
const (
	// AccountCategoryUnidentified is the default for everything that predates this
	// classification: nobody has said what the account is, so no cap is asserted.
	AccountCategoryUnidentified = "unidentified"
	// AccountCategoryStandard is an account the loyalty program's beneficiary cap
	// applies to. The only category that gets a CPF counter.
	AccountCategoryStandard = "standard"
	// AccountCategoryInjunction is an account under a court order ("liminar") that
	// suspends the cap. It has no limit to show.
	AccountCategoryInjunction = "injunction"
)

type Account struct {
	ID               string            `gorm:"primaryKey;type:uuid" json:"id"`
	LoyaltyProgramID string            `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram   `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	Login            *string           `json:"login"`
	Password         *string           `json:"password"`
	HolderID         *string           `gorm:"column:holder_id;type:uuid" json:"holder_id"`
	Holder           *Person           `gorm:"foreignKey:HolderID;references:ID" json:"holder,omitempty"`
	AdministratorID  *string           `gorm:"column:administrator_id;type:uuid" json:"administrator_id"`
	Administrator    *Person           `gorm:"foreignKey:AdministratorID;references:ID" json:"administrator,omitempty"`
	Nicknames        []AccountNickname `gorm:"foreignKey:AccountID;references:ID" json:"nicknames,omitempty"`
	// Category decides whether the program's beneficiary cap applies at all.
	// "injunction" is an account under a court order suspending the cap, and
	// "unidentified" is one nobody has classified yet - only "standard" is counted.
	// See AccountCategory* below and migration 37.
	Category string `gorm:"type:account_category;not null;default:'unidentified'" json:"category"`
	// WaivesRedemptionFee says this account pays no redemption fee at all - on Azul that
	// is the Diamante Unique tier. Phrased as the effect rather than the tier because the
	// effect is what the calculation needs, and because the fee itself is not tier-specific:
	// every Azul level below Unique pays the same domestic amount today. See migration 40.
	WaivesRedemptionFee bool `gorm:"column:waives_redemption_fee;not null;default:false" json:"waives_redemption_fee"`
	// OwnsPointInventory says the points on this account were bought by us, so their cost
	// is an acquisition cost tracked in batches rather than a transfer price paid to an
	// administrator. It is what decides which of the two meanings of "cost" applies:
	// with it off, issuances.cost_of_the_thousand is typed on /sales/suppliers as it
	// always was; with it on, the FIFO costing derives it and the field is read-only.
	//
	// Not the same as Category == AccountCategoryInjunction, even though the two overlap
	// today: the category answers whether the beneficiary cap applies, this one answers
	// who paid for the points. See migration 48.
	OwnsPointInventory bool `gorm:"column:owns_point_inventory;not null;default:false" json:"owns_point_inventory"`
	// DefaultProfitSharePercent seeds the split of a new batch on this account. Only a
	// default: each batch freezes its own copy, so changing this never rewrites a month
	// that was already settled. Zero means the profit stays whole with whoever funded
	// the batch, which is the right answer for an account with no arrangement at all.
	DefaultProfitSharePercent int       `gorm:"column:default_profit_share_percent;not null;default:0" json:"default_profit_share_percent"`
	Complete                  bool      `gorm:"not null;default:false" json:"complete"`
	CreatedAt                 time.Time `json:"created_at"`
	UpdatedAt                 time.Time `json:"updated_at"`
	// Archiving, not a trash can to restore from. issuances.account_id is ON DELETE
	// RESTRICT and issuances are themselves soft-deleted, so a real delete on an
	// account that ever issued is refused by Postgres forever. Turning the delete
	// into an UPDATE is what makes the account disappear from the panel without
	// erasing which account a past issuance was made on - see migration 29.
	//
	// Exposed as deleted_at rather than hidden: the grid receives archived accounts
	// embedded in old issuances (to label them) and has to keep them out of the
	// pickers, which it can only do if it can tell them apart.
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}

func (a *Account) BeforeCreate(tx *gorm.DB) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	return nil
}

func (a *Account) BeforeSave(tx *gorm.DB) error {
	if a.Password != nil && !strings.HasPrefix(*a.Password, "enc:") {
		encrypted, err := utils.Encrypt(*a.Password)
		if err != nil {
			return err
		}
		a.Password = &encrypted
	}
	return nil
}

func (a *Account) AfterFind(tx *gorm.DB) error {
	if a.Password != nil {
		decrypted, err := utils.Decrypt(*a.Password)
		if err != nil {
			return err
		}
		a.Password = &decrypted
	}
	return nil
}

// ComputeComplete reports whether all essential fields of an account are filled in.
func (a *Account) ComputeComplete() bool {
	return a.LoyaltyProgramID != "" &&
		a.Login != nil &&
		a.Password != nil &&
		a.HolderID != nil &&
		a.AdministratorID != nil
}
