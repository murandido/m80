package bot

import (
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// handleCancelamento cancels the issuance a CANCELAMENTO message names. Same
// gates as EMISSAO and ALTERACAO - see authorizeSender - and the same
// silence on a failed gate.
func (b *Bot) handleCancelamento(evt *events.Message, text string, startedAt time.Time) {
	user, allowed := b.authorizeSender(evt, "cancelamento")
	if !allowed {
		return
	}

	msg, err := parseCancelamento(text)
	if err != nil {
		b.Logger.Infof("cancelamento: could not parse a message from %s: %v", user.Name, err)
		b.refuse(evt, startedAt, refusalText("", err))
		return
	}

	// Finding the issuance and cancelling it share one transaction: nothing
	// about this can be split across two, the way EMISSAO's resolve+create
	// sometimes conjures an account that has to survive on its own.
	err = b.DB.Transaction(func(tx *gorm.DB) error {
		issuance, err := findIssuanceToCancel(tx, msg.ProgramLocator)
		if err != nil {
			return err
		}

		_, err = issuancesvc.Cancel(tx, issuance, cancelInputFor(msg, issuance), user.ID)
		return err
	})

	if err != nil {
		b.Logger.Errorf("cancelamento: failed to cancel %s for %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	b.Logger.Infof("cancelamento: cancelled issuance %s from %s", msg.ProgramLocator, user.Name)

	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(evt, "🚀")

	b.notifyIssuanceCancelled()
}

// cancelInputFor builds the refund the message asked for. Without a line 2,
// the doc's rule is a full refund - whatever the issuance actually charged
// comes back in full, with no fee - so the values are read off the issuance
// itself rather than left nil, which would instead record "nothing refunded".
func cancelInputFor(msg *cancelamentoMessage, issuance data.Issuance) issuancesvc.CancelInput {
	if !msg.HasRefundLine {
		points := issuance.Points
		return issuancesvc.CancelInput{RefundedPoints: &points, RefundedTaxes: issuance.Taxes}
	}
	return issuancesvc.CancelInput{
		RefundedPoints:  &msg.RefundedPoints,
		RefundedTaxes:   msg.RefundedTaxes,
		CancellationFee: msg.CancellationFee,
	}
}

// findIssuanceToCancel locates the active issuance a locator names - same
// query as ALTERACAO's findIssuanceToCorrect, kept separate here rather than
// shared so this file doesn't reach into alteracao.go's already-tested code
// for a query this small.
func findIssuanceToCancel(tx *gorm.DB, locator string) (data.Issuance, error) {
	var issuance data.Issuance

	err := tx.Where("program_locator = ? AND active = ?", locator, true).First(&issuance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return issuance, operatorErrorf(err, "Não encontrei nenhuma emissão ativa com o localizador %q", locator)
	}
	return issuance, err
}
