package schemas

import "time"

// CreatePromoRecipeRequest defines the payload to register how a site's enrolment form
// is driven.
//
// Every selector is optional, and that is the design rather than leniency: an empty one
// means "resolve this piece heuristically", so a conventional site needs none at all
// and an awkward one is pinned down a field at a time.
type CreatePromoRecipeRequest struct {
	// Domain is matched against the link's host, so it carries no scheme and no www:
	// "voeazul.com.br". A subdomain may have its own row when it needs its own
	// selectors; otherwise the site's row covers it.
	Domain           string  `json:"domain" validate:"required"`
	Name             string  `json:"name" validate:"required"`
	LoyaltyProgramID string  `json:"loyalty_program_id" validate:"required,uuid"`
	CPFSelector      *string `json:"cpf_selector" validate:"omitempty"`
	CheckboxSelector *string `json:"checkbox_selector" validate:"omitempty"`
	SubmitSelector   *string `json:"submit_selector" validate:"omitempty"`
	// SuccessPattern and FailurePattern are regular expressions matched against what
	// the site shows after submitting. Empty falls back to the wording built into the
	// promo service.
	SuccessPattern *string `json:"success_pattern" validate:"omitempty"`
	FailurePattern *string `json:"failure_pattern" validate:"omitempty"`
	// WaitSeconds is how long to let the page settle before reading the form. Single
	// page apps render the promotion widget after the first paint.
	WaitSeconds *int    `json:"wait_seconds" validate:"omitempty,gte=0,lte=120"`
	Active      *bool   `json:"active" validate:"omitempty"`
	Notes       *string `json:"notes" validate:"omitempty"`
}

// UpdatePromoRecipeRequest patches a recipe. Nil means "leave this alone" for the
// required columns, and "clear it" for the nullable selectors and patterns - otherwise
// a selector pinned by mistake could never be handed back to the heuristic.
type UpdatePromoRecipeRequest struct {
	Domain           *string `json:"domain" validate:"omitempty"`
	Name             *string `json:"name" validate:"omitempty"`
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	CPFSelector      *string `json:"cpf_selector" validate:"omitempty"`
	CheckboxSelector *string `json:"checkbox_selector" validate:"omitempty"`
	SubmitSelector   *string `json:"submit_selector" validate:"omitempty"`
	SuccessPattern   *string `json:"success_pattern" validate:"omitempty"`
	FailurePattern   *string `json:"failure_pattern" validate:"omitempty"`
	WaitSeconds      *int    `json:"wait_seconds" validate:"omitempty,gte=0,lte=120"`
	Active           *bool   `json:"active" validate:"omitempty"`
	Notes            *string `json:"notes" validate:"omitempty"`
}

type PromoRecipeResponse struct {
	ID                 string    `json:"id"`
	Domain             string    `json:"domain"`
	Name               string    `json:"name"`
	LoyaltyProgramID   string    `json:"loyalty_program_id"`
	LoyaltyProgramName string    `json:"loyalty_program_name"`
	CPFSelector        *string   `json:"cpf_selector"`
	CheckboxSelector   *string   `json:"checkbox_selector"`
	SubmitSelector     *string   `json:"submit_selector"`
	SuccessPattern     *string   `json:"success_pattern"`
	FailurePattern     *string   `json:"failure_pattern"`
	WaitSeconds        int       `json:"wait_seconds"`
	Active             bool      `json:"active"`
	Notes              *string   `json:"notes"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// TestPromoRecipeRequest is the dry run. The recipe travels in the body rather than
// being read by id, so a recipe can be tried *before* it is saved - which is what makes
// a broken campaign fixable inside its two-day window.
type TestPromoRecipeRequest struct {
	URL              string  `json:"url" validate:"required,url"`
	CPFSelector      *string `json:"cpf_selector" validate:"omitempty"`
	CheckboxSelector *string `json:"checkbox_selector" validate:"omitempty"`
	SubmitSelector   *string `json:"submit_selector" validate:"omitempty"`
	SuccessPattern   *string `json:"success_pattern" validate:"omitempty"`
	FailurePattern   *string `json:"failure_pattern" validate:"omitempty"`
	WaitSeconds      *int    `json:"wait_seconds" validate:"omitempty,gte=0,lte=120"`
}

// PromoRecipeTestResponse reports what the browser found without submitting anything.
type PromoRecipeTestResponse struct {
	// Resolved is nil when the page offered nothing that looks like an enrolment form.
	Resolved *PromoResolutionResponse `json:"resolved"`
	// Error carries the refusal reason when Resolved is nil.
	Error string `json:"error"`
	// Screenshot is the page as the browser saw it, base64 PNG. It is the whole point
	// of the dry run: the recipe author sees what the bot would see.
	Screenshot string `json:"screenshot_base64"`
}

type PromoResolutionResponse struct {
	CPFField string `json:"cpf_field"`
	Checkbox bool   `json:"checkbox"`
	Submit   string `json:"submit"`
	// Source is "recipe", "heuristic" or "mixed".
	Source string `json:"source"`
}

// CreateLoyaltyDocumentRequest registers one CPF in a loyalty programme's pool.
type CreateLoyaltyDocumentRequest struct {
	Name             string `json:"name" validate:"required"`
	CPF              string `json:"cpf" validate:"required"`
	LoyaltyProgramID string `json:"loyalty_program_id" validate:"required,uuid"`
	Active           *bool  `json:"active" validate:"omitempty"`
}

type UpdateLoyaltyDocumentRequest struct {
	Name             *string `json:"name" validate:"omitempty"`
	CPF              *string `json:"cpf" validate:"omitempty"`
	LoyaltyProgramID *string `json:"loyalty_program_id" validate:"omitempty,uuid"`
	Active           *bool   `json:"active" validate:"omitempty"`
}

// LoyaltyDocumentResponse returns the CPF decrypted, the same decision already taken
// for AccountResponse.Password: the operator has to see the number to reconcile it, and
// hiding it in the panel while a dump holds it would protect nobody.
type LoyaltyDocumentResponse struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	CPF                string    `json:"cpf"`
	LoyaltyProgramID   string    `json:"loyalty_program_id"`
	LoyaltyProgramName string    `json:"loyalty_program_name"`
	Active             bool      `json:"active"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// PromoEnrollmentResponse is one line of the enrolment history.
type PromoEnrollmentResponse struct {
	ID           string    `json:"id"`
	URL          string    `json:"url"`
	URLKey       string    `json:"url_key"`
	DocumentID   string    `json:"document_id"`
	DocumentName string    `json:"document_name"`
	RecipeID     *string   `json:"recipe_id"`
	RecipeName   string    `json:"recipe_name"`
	Status       string    `json:"status"`
	Message      *string   `json:"message"`
	CreatedByID  *string   `json:"created_by_id"`
	CreatedBy    string    `json:"created_by"`
	ChatJID      *string   `json:"chat_jid"`
	CreatedAt    time.Time `json:"created_at"`
}

// PromoEnrollmentPaginatedData wraps the history listing, which is paginated because it
// only grows.
type PromoEnrollmentPaginatedData struct {
	Enrollments []PromoEnrollmentResponse `json:"enrollments"`
	Total       int64                     `json:"total"`
	Page        int                       `json:"page"`
	Limit       int                       `json:"limit"`
}

// SetPromoModeRequest is the body of PATCH /bot/chats/{id}/promo-mode. Explicit on/off
// rather than a toggle, even though the panel is the only writer of this column - a
// toggle is a request whose meaning depends on a state the caller may not have.
type SetPromoModeRequest struct {
	Enabled *bool `json:"enabled" validate:"required" example:"true"`
}
