package data

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Outcome of one enrolment attempt.
const (
	// PromoEnrollmentSuccess is the only status the idempotency index counts. A CPF with
	// a success row for a URL is never submitted again.
	PromoEnrollmentSuccess = "success"
	// PromoEnrollmentFailure is the site saying no - an invalid document, an ineligible
	// account, a closed campaign.
	PromoEnrollmentFailure = "failure"
	// PromoEnrollmentUnknown is a submission that went through with nothing recognisable
	// coming back. Worth retrying, and worth a human looking at the screenshot.
	PromoEnrollmentUnknown = "unknown"
	// PromoEnrollmentRefused is us declining to act: no form could be resolved, or the
	// page looked like a sign-in screen. Nothing was submitted.
	PromoEnrollmentRefused = "refused"
	// PromoEnrollmentError is our side breaking - the browser died, the service was
	// unreachable, the page never loaded.
	PromoEnrollmentError = "error"
)

// PromoEnrollment records one attempt to enrol one document in one campaign.
//
// This table is the only source of truth about who is already enrolled, which is a
// design decision rather than a convenience: the Azul campaign this was built against
// accepts the same CPF repeatedly and reports success every time, so the site cannot be
// asked. Other campaigns refuse, or warn, or stay silent - the behaviour varies per
// campaign, so relying on it would be building on sand.
//
// The proof screenshot is deliberately not a column. It goes to the WhatsApp group and
// is discarded; images here would bloat the nightly dump for nothing.
type PromoEnrollment struct {
	ID string `gorm:"primaryKey;type:uuid" json:"id"`
	// The link exactly as it arrived, for the audit trail.
	URL string `gorm:"column:url;not null" json:"url"`
	// The link canonicalised by promotion.CanonicalURLKey - scheme, host and path, with
	// the query string dropped. Promotion links circulate with tracking parameters, so
	// without this the same campaign pasted twice reads as two campaigns and everybody
	// is enrolled again.
	URLKey     string           `gorm:"column:url_key;not null;index" json:"url_key"`
	DocumentID string           `gorm:"column:document_id;type:uuid;not null" json:"document_id"`
	Document   *LoyaltyDocument `gorm:"foreignKey:DocumentID;references:ID" json:"document,omitempty"`
	// Nullable so that deleting a recipe does not erase the history of what it did.
	RecipeID *string      `gorm:"column:recipe_id;type:uuid" json:"recipe_id"`
	Recipe   *PromoRecipe `gorm:"foreignKey:RecipeID;references:ID" json:"recipe,omitempty"`
	Status   string       `gorm:"not null" json:"status"`
	Message  *string      `json:"message"`
	// Who dropped the link and where. Both nil for an enrolment started from the panel.
	CreatedByID *string   `gorm:"column:created_by_id;type:uuid" json:"created_by_id"`
	CreatedBy   *User     `gorm:"foreignKey:CreatedByID;references:ID" json:"created_by,omitempty"`
	ChatJID     *string   `gorm:"column:chat_jid" json:"chat_jid"`
	CreatedAt   time.Time `json:"created_at"`
}

func (pe *PromoEnrollment) BeforeCreate(tx *gorm.DB) error {
	if pe.ID == "" {
		pe.ID = uuid.New().String()
	}
	return nil
}
