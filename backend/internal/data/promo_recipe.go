package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PromoRecipe describes how to drive one site's enrolment form.
//
// It is data rather than code on purpose: promotion windows last about two days, so a
// campaign that changes its markup has to be fixable from the panel in minutes instead
// of through a branch and a deploy.
//
// Every selector is optional. A nil selector means "resolve this piece heuristically",
// which lets a conventional site work with no selectors at all and an awkward one be
// pinned down a field at a time.
type PromoRecipe struct {
	ID string `gorm:"primaryKey;type:uuid" json:"id"`
	// Registrable domain without scheme or www, e.g. "voeazul.com.br". This is how an
	// incoming link finds its recipe.
	Domain           string          `gorm:"not null;uniqueIndex" json:"domain"`
	Name             string          `gorm:"not null" json:"name"`
	LoyaltyProgramID string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram   *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	CPFSelector      *string         `gorm:"column:cpf_selector" json:"cpf_selector"`
	CheckboxSelector *string         `gorm:"column:checkbox_selector" json:"checkbox_selector"`
	SubmitSelector   *string         `gorm:"column:submit_selector" json:"submit_selector"`
	// Regular expressions matched against what the site shows after submitting. Nil
	// falls back to the wording built into the promo service.
	SuccessPattern *string `gorm:"column:success_pattern" json:"success_pattern"`
	FailurePattern *string `gorm:"column:failure_pattern" json:"failure_pattern"`
	WaitSeconds    int     `gorm:"column:wait_seconds;not null;default:5" json:"wait_seconds"`
	// No `default` tag, for the same reason ListenedChat.Active carries none: GORM
	// treats a Go zero value as "unset" on a column with a database default, so an
	// explicit Active:false would be overwritten back to true on create.
	Active    bool      `gorm:"not null" json:"active"`
	Notes     *string   `json:"notes"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (pr *PromoRecipe) BeforeCreate(tx *gorm.DB) error {
	if pr.ID == "" {
		pr.ID = uuid.New().String()
	}
	return nil
}
