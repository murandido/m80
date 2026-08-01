package bot

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// auditActionUpdated is the audit_logs action a correction records - the same
// value the enum has carried since migration 000018, and the same one the panel
// would use if it logged its own edits.
const auditActionUpdated = "updated"

// handleAlteracao corrects an issuance from an ALTERACAO message. It passes
// through exactly the same gates as an EMISSAO, for the same reasons - see
// authorizeSender.
func (b *Bot) handleAlteracao(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	user, allowed := b.authorizeSender(evt, "alteracao")
	if !allowed {
		return
	}

	msg, err := parseAlteracao(text)
	if err != nil {
		b.Logger.Infof("alteracao: could not parse a message from %s: %v", user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText("", err))
		return
	}

	// Resolution and the write share one transaction, exactly as in handleEmissao:
	// an account conjured from an unknown alias must not outlive a correction that
	// fails afterwards.
	var outcome correctionOutcome
	err = b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		issuance, err := findIssuanceToCorrect(tx, msg.ProgramLocator)
		if err != nil {
			return err
		}

		fields, err := resolveAlteracao(tx, msg, issuance)
		if err != nil {
			return err
		}

		outcome, err = applyCorrection(tx, issuance, fields, user.ID)
		return err
	})

	if err != nil {
		b.Logger.Errorf("alteracao: failed to correct %s for %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	b.answerCorrection(ctx, evt, startedAt, msg.ProgramLocator, outcome)
}

// correctionOutcome is what actually happened, which is not always what the
// message asked for.
type correctionOutcome struct {
	// Changed is false when the merged issuance came out identical to the one
	// already stored - a message sent twice, or one restating what was there.
	Changed bool

	// IssuanceID is the row the correction landed on, so the caller can hand it
	// to Traveos without looking it up again.
	IssuanceID string

	// RetryTraveos is true when the correction moved one of the fields Traveos is
	// asked with, on an issuance that is still incomplete. That combination is
	// the operator fixing the very input that made the fetch fail, so it earns a
	// second attempt; anything else does not.
	RetryTraveos bool
}

// answerCorrection replies to a correction that went through. Same vocabulary as
// the EMISSAO: 🚀 alone, no text, and only after the transaction committed.
//
// The reaction is the same whether or not anything moved. From the group's side
// a message that draws no reaction is indistinguishable from a bot that is down,
// and "what you asked for is already true" is a success, not a failure - it just
// has nothing to write down, so no audit entry and no SSE event follow it.
func (b *Bot) answerCorrection(ctx context.Context, evt *events.Message, startedAt time.Time, locator string, outcome correctionOutcome) {
	if outcome.Changed {
		b.Logger.Infof("alteracao: corrected issuance %s", locator)
	} else {
		b.Logger.Infof("alteracao: %s was already in the state the message asked for, nothing to do", locator)
	}

	// The operator fixed one of the fields Traveos is asked with, on an issuance
	// that never got its itinerary - so this is the retry, and it is why the
	// correction path reacts ⚠️ too. force stays false: the ticket may or may not
	// already exist at the vendor, and that is precisely what the search-first
	// path is there to work out.
	fetched := traveosSkipped
	if outcome.RetryTraveos {
		fetched = b.syncTraveos(ctx, "alteracao", outcome.IssuanceID, locator, false)
	}

	// One event for both writes. The panel reloads the row either way, so firing
	// twice would only make it do it twice.
	if outcome.Changed || fetched == traveosFetched {
		b.notifyIssuanceUpdated()
	}

	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(ctx, evt, successReaction(fetched))
}

// findIssuanceToCorrect locates the issuance a locator names. The partial unique
// index idx_issuances_program_locator_active guarantees at most one, so there is
// nothing to disambiguate.
func findIssuanceToCorrect(tx *gorm.DB, locator string) (data.Issuance, error) {
	var issuance data.Issuance

	err := tx.Where("program_locator = ? AND active = ?", locator, true).First(&issuance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return issuance, operatorErrorf(err, "Não encontrei nenhuma emissão com o localizador %q", locator)
	}
	if err != nil {
		return issuance, err
	}

	// A cancelled issuance is a settled fact with money attached to it. Silently
	// editing one would leave the operator believing a correction landed on a row
	// nobody looks at any more.
	if issuance.CancelledAt != nil {
		return issuance, operatorErrorf(nil, "A emissão %s está cancelada e não pode ser corrigida", locator)
	}
	return issuance, nil
}

// applyCorrection merges the resolved fields into the issuance and writes the
// difference - the row itself plus one audit log entry describing exactly what
// moved. When nothing moved it writes nothing at all, so a message sent twice
// does not leave two identical entries in the history.
func applyCorrection(tx *gorm.DB, issuance data.Issuance, fields alteracaoFields, actorID string) (correctionOutcome, error) {
	corrected := issuance
	if err := mergeCorrection(&corrected, fields); err != nil {
		return correctionOutcome{}, err
	}

	details := diffIssuance(issuance, corrected)
	if details == nil {
		return correctionOutcome{IssuanceID: issuance.ID}, nil
	}

	// Same completion boundary the panel enforces in UpdateIssuance: what is
	// checked is the merged state, not the correction that arrived.
	if corrected.Complete {
		var airline data.Airline
		if err := tx.First(&airline, "id = ?", corrected.AirlineID).Error; err != nil {
			return correctionOutcome{}, err
		}
		if err := issuancesvc.ValidateAirlineRequirements(airline, corrected.MainPassengerName, corrected.LatamPurchaseNumber); err != nil {
			return correctionOutcome{}, err
		}
	}

	if err := tx.Save(&corrected).Error; err != nil {
		return correctionOutcome{}, issuancesvc.TranslateWriteError(err)
	}

	encoded, err := json.Marshal(details)
	if err != nil {
		return correctionOutcome{}, err
	}

	entry := data.AuditLog{
		IssuanceID:  corrected.ID,
		ChangedByID: actorID,
		Action:      auditActionUpdated,
		Details:     datatypes.JSON(encoded),
	}
	if err := tx.Create(&entry).Error; err != nil {
		return correctionOutcome{}, err
	}

	return correctionOutcome{
		Changed:      true,
		IssuanceID:   corrected.ID,
		RetryTraveos: !corrected.Complete && movedATraveosInput(details),
	}, nil
}

// traveosInputColumns are the columns whose values are handed to Traveos when the
// itinerary is fetched - see issuance.findOrEmit. A correction that moves any of
// them is a correction that could turn a failed fetch into a working one.
//
// airline_locator and airline_id are in the list for the same reason as the three
// obvious ones: the locator sent to the vendor is the airline's PNR whenever we
// have it, and the airline decides which vendor-side airline id is used at all.
var traveosInputColumns = []string{
	"airline_id",
	"airline_locator",
	"origin_iata_code",
	"main_passenger_name",
	"latam_purchase_number",
}

// movedATraveosInput reads the audit details this correction is about to write,
// rather than re-deriving the comparison: whatever ends up in the history is
// exactly what changed, so the two can never drift apart.
func movedATraveosInput(details map[string]any) bool {
	after, ok := details["after"].(map[string]any)
	if !ok {
		return false
	}
	for _, column := range traveosInputColumns {
		if _, moved := after[column]; moved {
			return true
		}
	}
	return false
}

// mergeCorrection writes the resolved fields onto the issuance.
//
// Route fields are the one thing a correction cannot always do. Once an issuance
// is complete those values have been confirmed against the source, and "fixing"
// them would replace confirmed data with whatever someone retyped from memory.
// Everything else stays editable, because a wrong price is wrong whether or not
// the issuance is finished.
//
// The two triggers part ways there, and deliberately. A route line in an
// ALTERACAO is a request someone typed on purpose, so refusing the whole message
// is the honest answer - a 🚀 would claim the origin moved when it did not. In an
// edited EMISSAO line 2 is present because the message restates everything, not
// because anyone asked, so it is dropped in silence and the rest of the
// correction still lands; otherwise fixing the price of a completed issuance from
// WhatsApp would be impossible.
func mergeCorrection(issuance *data.Issuance, fields alteracaoFields) error {
	if fields.LoyaltyProgramID != nil {
		issuance.LoyaltyProgramID = *fields.LoyaltyProgramID
	}
	if fields.AirlineID != nil {
		issuance.AirlineID = *fields.AirlineID
	}
	if fields.Points != nil {
		issuance.Points = *fields.Points
	}
	if fields.ValueOfTheThousand != nil {
		issuance.ValueOfTheThousand = *fields.ValueOfTheThousand
	}
	if fields.Taxes != nil {
		issuance.Taxes = fields.Taxes
	}
	if fields.AccountID != nil {
		issuance.AccountID = *fields.AccountID
		issuance.AccountNicknameID = fields.AccountNicknameID
	}
	if fields.AgencyID != nil {
		issuance.AgencyID = *fields.AgencyID
	}

	// In a full restatement a nil is the operator having deleted something, so it
	// clears the column. In a partial one a nil only means the message was silent
	// about it.
	if fields.Full {
		issuance.AirlineLocator = fields.AirlineLocator
		issuance.Observation = fields.Observation
	} else if fields.Observation != nil {
		issuance.Observation = fields.Observation
	}

	if !fields.HasRoute {
		return nil
	}
	if issuance.Complete {
		if fields.Full {
			return nil
		}
		return operatorErrorf(nil,
			"A emissão %s já está completa, então a origem e o passageiro não podem ser corrigidos por mensagem",
			issuance.ProgramLocator)
	}

	issuance.OriginIATACode = fields.OriginIATACode
	if fields.Full {
		issuance.MainPassengerName = fields.MainPassengerName
		issuance.LatamPurchaseNumber = fields.LatamPurchaseNumber
		return nil
	}
	if fields.MainPassengerName != nil {
		issuance.MainPassengerName = fields.MainPassengerName
	}
	if fields.LatamPurchaseNumber != nil {
		issuance.LatamPurchaseNumber = fields.LatamPurchaseNumber
	}
	return nil
}

// diffIssuance returns the before/after snapshot for audit_logs.details, or nil
// when the correction changed nothing. Only the columns that actually moved are
// listed: a history entry restating thirteen unchanged fields buries the one
// that matters.
func diffIssuance(before, after data.Issuance) map[string]any {
	was, now := correctableColumns(before), correctableColumns(after)

	changedBefore := make(map[string]any)
	changedAfter := make(map[string]any)
	for column, oldValue := range was {
		if now[column] != oldValue {
			changedBefore[column] = oldValue
			changedAfter[column] = now[column]
		}
	}

	if len(changedAfter) == 0 {
		return nil
	}
	return map[string]any{"before": changedBefore, "after": changedAfter}
}

// correctableColumns is every column a correction can touch, flattened so two
// snapshots compare with ==. Pointers are dereferenced rather than compared as
// pointers, which would report a change every single time.
func correctableColumns(issuance data.Issuance) map[string]any {
	return map[string]any{
		"loyalty_program_id":    issuance.LoyaltyProgramID,
		"airline_id":            issuance.AirlineID,
		"airline_locator":       valueOf(issuance.AirlineLocator),
		"points":                issuance.Points,
		"value_of_the_thousand": issuance.ValueOfTheThousand,
		"taxes":                 valueOf(issuance.Taxes),
		"origin_iata_code":      valueOf(issuance.OriginIATACode),
		"main_passenger_name":   valueOf(issuance.MainPassengerName),
		"latam_purchase_number": valueOf(issuance.LatamPurchaseNumber),
		"account_id":            issuance.AccountID,
		"account_nickname_id":   valueOf(issuance.AccountNicknameID),
		"agency_id":             issuance.AgencyID,
		"observation":           valueOf(issuance.Observation),
	}
}

func valueOf[T comparable](pointer *T) any {
	if pointer == nil {
		return nil
	}
	return *pointer
}
