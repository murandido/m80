package bot

import (
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// handleMessageEdit routes an edited message. Editing the message you already
// sent is what an operator reaches for first when they spot a typo - it is more
// natural than composing an ALTERACAO, and it leaves the group's scrollback
// showing the corrected text instead of a correction trailing the mistake.
//
// An edit of ordinary conversation is ignored, exactly as the original would
// have been.
func (b *Bot) handleMessageEdit(evt *events.Message, text string, startedAt time.Time) {
	switch {
	case isAlteracaoMessage(text):
		// An edited ALTERACAO needs no special treatment: its own line 1 says
		// which issuance it corrects, edit or not.
		b.handleAlteracao(evt, text, startedAt)
	case isEmissaoMessage(text):
		b.handleEditedEmissao(evt, text, startedAt)
	default:
		// Ignoring is right, but doing it without a word is what made a broken
		// unwrap indistinguishable from an edit of ordinary chatter. The two are
		// logged apart, and neither quotes the group's text back into the log.
		if text == "" {
			b.Logger.Warnf("edicao: edit %s carried no readable text - the payload was not unwrapped", evt.Info.ID)
		} else {
			b.Logger.Infof("edicao: edit %s is neither an EMISSAO nor an ALTERACAO, ignoring", evt.Info.ID)
		}
	}
}

// handleEditedEmissao treats the rewritten EMISSAO as a restatement of the whole
// issuance: every field the message carries is applied, and one it no longer
// carries is cleared - see alteracaoFields.Full.
func (b *Bot) handleEditedEmissao(evt *events.Message, text string, startedAt time.Time) {
	user, allowed := b.authorizeSender(evt, "edicao")
	if !allowed {
		return
	}

	msg, err := parseEmissao(text)
	if err != nil {
		b.Logger.Infof("edicao: could not parse an edited message from %s: %v", user.Name, err)
		b.refuse(evt, startedAt, refusalText("", err))
		return
	}

	remembered, known := b.issued.lookup(evt.Info.ID)

	// This is the whole reason the bot remembers which message produced which
	// issuance. The locator is the issuance's identity, and an issuance that
	// exists cannot be handed a new one - not least because the airline and the
	// loyalty program know it under the old one.
	if known && remembered.ProgramLocator != msg.ProgramLocator {
		b.Logger.Infof("edicao: %s tried to change the locator of %s to %s",
			user.Name, remembered.ProgramLocator, msg.ProgramLocator)
		b.refuse(evt, startedAt, "Não é possível alterar o PNR de uma emissão já salva")
		return
	}

	issuance, found, err := b.locateEditedIssuance(remembered, known, msg.ProgramLocator)
	if err != nil {
		b.Logger.Errorf("edicao: failed to look up the issuance behind an edit from %s: %v", user.Name, err)
		b.refuse(evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	// Nothing to correct: the message being edited never became an issuance, so
	// this is someone fixing an EMISSAO the bot had refused. Registering it is
	// exactly what they are asking for.
	if !found {
		b.Logger.Infof("edicao: no issuance behind the edited message, registering %s as a new one", msg.ProgramLocator)
		b.handleEmissao(evt, text, startedAt)
		return
	}

	if issuance.CancelledAt != nil {
		b.refuse(evt, startedAt, operatorErrorf(nil,
			"A emissão %s está cancelada e não pode ser corrigida", msg.ProgramLocator).Error())
		return
	}

	var outcome correctionOutcome
	err = b.DB.Transaction(func(tx *gorm.DB) error {
		// resolveEmissao already turns a full message into resolved ids, aliases
		// and all - reusing it is what keeps an edited EMISSAO meaning exactly
		// what the original one meant.
		input, err := resolveEmissao(tx, msg)
		if err != nil {
			return err
		}

		outcome, err = applyCorrection(tx, issuance, correctionFromEmissao(input), user.ID)
		return err
	})

	if err != nil {
		b.Logger.Errorf("edicao: failed to correct %s for %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	b.answerCorrection(evt, startedAt, msg.ProgramLocator, outcome)
}

// locateEditedIssuance finds the issuance an edit refers to, preferring the one
// the bot remembers creating and falling back to the locator written in the
// message.
//
// The fallback is what covers an edit of a message older than the map - sent
// before the last restart, or evicted since. It cannot tell a rewritten locator
// from an unknown one, which is the one thing the map buys; everything else
// behaves the same.
func (b *Bot) locateEditedIssuance(remembered issuedMessage, known bool, locator string) (data.Issuance, bool, error) {
	var issuance data.Issuance

	if known {
		err := b.DB.Where("id = ? AND active = ?", remembered.IssuanceID, true).First(&issuance).Error
		if err == nil {
			return issuance, true, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return issuance, false, err
		}
		// Remembered but gone - deleted from the panel since. The locator is free
		// again, so fall through and let the message stand on its own.
	}

	err := b.DB.Where("program_locator = ? AND active = ?", locator, true).First(&issuance).Error
	switch {
	case err == nil:
		return issuance, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return issuance, false, nil
	default:
		return issuance, false, err
	}
}

// correctionFromEmissao reads a fully resolved EMISSAO as a correction covering
// every field at once.
//
// AgencyID is the one exception to Full's "absent means cleared" rule: an
// EMISSAO that names no agency does not mean "no agency", it means the default
// one - and quietly moving an issuance back to the default because line 4 lost
// its parentheses would be a change nobody asked for.
func correctionFromEmissao(in issuancesvc.CreateInput) alteracaoFields {
	return alteracaoFields{
		Full:             true,
		LoyaltyProgramID: &in.LoyaltyProgramID,
		AirlineID:        in.AirlineID,
		AirlineLocator:   in.AirlineLocator,

		Points:             &in.Points,
		ValueOfTheThousand: &in.ValueOfTheThousand,
		Taxes:              in.Taxes,

		HasRoute:            true,
		OriginIATACode:      in.OriginIATACode,
		MainPassengerName:   in.MainPassengerName,
		LatamPurchaseNumber: in.LatamPurchaseNumber,

		AccountID:         &in.AccountID,
		AccountNicknameID: in.AccountNicknameID,
		AgencyID:          in.AgencyID,

		Observation: in.Observation,
	}
}
