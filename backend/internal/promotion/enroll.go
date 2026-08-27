// Package promotion holds the rules for enrolling the agency's CPFs in loyalty
// transfer-bonus promotions.
//
// It lives outside the HTTP handlers for the same reason internal/issuance does: the
// WhatsApp bot needs the same rules and has no *http.Request to answer with. Errors are
// typed here and each consumer translates them into its own medium - a status code in
// the API, a sentence in a group.
package promotion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

var (
	// ErrNoRecipe means no active recipe covers the link's domain. The bot refuses and
	// says which domain, so somebody can author one in the panel.
	ErrNoRecipe = errors.New("promotion: no recipe for this domain")
	// ErrFormNotResolved means the page offered nothing that looks like an enrolment
	// form. Nothing was submitted.
	ErrFormNotResolved = errors.New("promotion: enrolment form not found on the page")
	// ErrNoDocuments means the recipe's loyalty programme has no active CPFs.
	ErrNoDocuments = errors.New("promotion: no active documents for this loyalty programme")
)

// ResolveRecipe finds the active recipe covering a link's host.
//
// Matching is by host suffix rather than equality so that a campaign served from
// promo.voeazul.com.br still finds the voeazul.com.br recipe, and the longest match
// wins so a subdomain can be given its own row when it needs different selectors.
// The set of recipes is a handful of rows, so filtering in Go beats a clever query.
func ResolveRecipe(db *gorm.DB, rawURL string) (*data.PromoRecipe, error) {
	host, err := DomainOf(rawURL)
	if err != nil {
		return nil, err
	}

	var recipes []data.PromoRecipe
	if err := db.Preload("LoyaltyProgram").Where("active = ?", true).Find(&recipes).Error; err != nil {
		return nil, fmt.Errorf("promotion: loading recipes: %w", err)
	}

	var matches []data.PromoRecipe
	for _, recipe := range recipes {
		domain := strings.ToLower(strings.TrimPrefix(recipe.Domain, "www."))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			matches = append(matches, recipe)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoRecipe, host)
	}
	sort.Slice(matches, func(i, j int) bool {
		return len(matches[i].Domain) > len(matches[j].Domain)
	})
	return &matches[0], nil
}

// wireRecipe converts a stored recipe into what the service expects, turning nil
// selectors into empty strings - the service reads those as "resolve heuristically".
func wireRecipe(recipe *data.PromoRecipe) *Recipe {
	if recipe == nil {
		return nil
	}
	return &Recipe{
		CPFSelector:      deref(recipe.CPFSelector),
		CheckboxSelector: deref(recipe.CheckboxSelector),
		SubmitSelector:   deref(recipe.SubmitSelector),
		SuccessPattern:   deref(recipe.SuccessPattern),
		FailurePattern:   deref(recipe.FailurePattern),
		WaitSeconds:      recipe.WaitSeconds,
	}
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// PendingDocuments returns the active documents of a programme that have not already
// succeeded for this campaign, plus the ones that have.
//
// This is where idempotency lives, and it has to: the campaign this was built against
// accepts the same CPF repeatedly and answers "Solicitação enviada" every time, so the
// site cannot tell us who is already in.
func PendingDocuments(db *gorm.DB, urlKey, programID string) (pending, already []data.LoyaltyDocument, err error) {
	var documents []data.LoyaltyDocument
	if err := db.
		Where("loyalty_program_id = ? AND active = ?", programID, true).
		Order("name").
		Find(&documents).Error; err != nil {
		return nil, nil, fmt.Errorf("promotion: loading documents: %w", err)
	}

	var enrolledIDs []string
	if err := db.Model(&data.PromoEnrollment{}).
		Where("url_key = ? AND status = ?", urlKey, data.PromoEnrollmentSuccess).
		Distinct().
		Pluck("document_id", &enrolledIDs).Error; err != nil {
		return nil, nil, fmt.Errorf("promotion: loading enrolment history: %w", err)
	}
	enrolled := make(map[string]bool, len(enrolledIDs))
	for _, id := range enrolledIDs {
		enrolled[id] = true
	}

	for _, document := range documents {
		if enrolled[document.ID] {
			already = append(already, document)
		} else {
			pending = append(pending, document)
		}
	}
	return pending, already, nil
}

// RunRequest is one campaign run.
type RunRequest struct {
	URL         string
	ChatJID     *string
	CreatedByID *string
}

// Outcome is what happened to one document.
type Outcome struct {
	Document   data.LoyaltyDocument
	Status     string
	Message    string
	Screenshot []byte
}

// RunResult is the whole run, shaped for a caller that has to report it to humans.
type RunResult struct {
	Recipe   *data.PromoRecipe
	URLKey   string
	Resolved *Resolution
	Outcomes []Outcome
	// AlreadyEnrolled were skipped because a success is already on record.
	AlreadyEnrolled []data.LoyaltyDocument
}

// Succeeded counts the outcomes worth celebrating.
func (r RunResult) Succeeded() int {
	count := 0
	for _, outcome := range r.Outcomes {
		if outcome.Status == data.PromoEnrollmentSuccess {
			count++
		}
	}
	return count
}

// Run enrols every pending document of the campaign's loyalty programme and records
// what happened.
//
// Nothing here guesses. If no recipe covers the domain, or the page yields no form, it
// returns a typed error and submits nothing - a wrong guess still looks like success,
// and an enrolment that quietly went to the wrong form is the same loss as forgetting.
func Run(ctx context.Context, db *gorm.DB, client *Client, req RunRequest) (RunResult, error) {
	recipe, err := ResolveRecipe(db, req.URL)
	if err != nil {
		return RunResult{}, err
	}

	urlKey, err := CanonicalURLKey(req.URL)
	if err != nil {
		return RunResult{}, err
	}

	pending, already, err := PendingDocuments(db, urlKey, recipe.LoyaltyProgramID)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{Recipe: recipe, URLKey: urlKey, AlreadyEnrolled: already}

	if len(pending) == 0 {
		if len(already) == 0 {
			return result, ErrNoDocuments
		}
		return result, nil
	}

	cpfs := make([]string, 0, len(pending))
	for _, document := range pending {
		cpfs = append(cpfs, document.DigitsOnlyCPF())
	}

	response, err := client.Enroll(ctx, ServiceRequest{
		URL:    req.URL,
		CPFs:   cpfs,
		Recipe: wireRecipe(recipe),
	})
	if err != nil {
		return result, err
	}
	if response.Resolved == nil {
		// The service refused. Nothing was submitted, so nothing is recorded: the
		// enrolment history is a log of attempts, and there was no attempt.
		return result, fmt.Errorf("%w: %s", ErrFormNotResolved, response.Error)
	}
	result.Resolved = response.Resolved

	// The service answers in the order it was asked, but pairing by CPF rather than by
	// position means a short or reordered response cannot silently attribute one
	// person's outcome to another.
	byCPF := make(map[string]ServiceResult, len(response.Results))
	for _, item := range response.Results {
		byCPF[item.CPF] = item
	}

	for _, document := range pending {
		item, answered := byCPF[document.DigitsOnlyCPF()]
		outcome := Outcome{Document: document}
		if answered {
			outcome.Status = item.Status
			outcome.Message = item.Message
			outcome.Screenshot = DecodeScreenshot(item.Screenshot)
		} else {
			// The run stopped early - the browser died, or the form stopped resolving
			// partway through. Recording this is the point: it is how the group finds
			// out who was left out instead of assuming everyone got in.
			outcome.Status = data.PromoEnrollmentError
			outcome.Message = "o serviço não retornou resultado para este CPF"
		}
		result.Outcomes = append(result.Outcomes, outcome)

		if err := record(db, req, recipe, urlKey, document, outcome); err != nil {
			return result, err
		}
	}
	return result, nil
}

func record(
	db *gorm.DB, req RunRequest, recipe *data.PromoRecipe,
	urlKey string, document data.LoyaltyDocument, outcome Outcome,
) error {
	message := outcome.Message
	enrollment := data.PromoEnrollment{
		URL:         req.URL,
		URLKey:      urlKey,
		DocumentID:  document.ID,
		RecipeID:    &recipe.ID,
		Status:      outcome.Status,
		CreatedByID: req.CreatedByID,
		ChatJID:     req.ChatJID,
	}
	if message != "" {
		enrollment.Message = &message
	}
	if err := db.Create(&enrollment).Error; err != nil {
		return fmt.Errorf("promotion: recording enrolment: %w", err)
	}
	return nil
}
