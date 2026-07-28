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

// handleDeletar deletes the issuance a DELETAR message names. Same gates as
// the other three triggers - see authorizeSender.
//
// Only the "already saved" branch of the doc's DELETAR table is implemented
// here: interrupting a job still queued or in flight against the Traveos
// needs the chat queue and PNR lock the product doc describes, and neither
// exists yet - the bot processes every message synchronously today, so there
// is no in-flight job to interrupt in the first place. When that
// infrastructure lands, this is where the other branch gets added.
//
// DELETAR only ever targets the active issuance for a locator - never an
// older, already-cancelled one that happens to share it after the locator was
// reused. A locator can legitimately have more than one historical row once a
// cancellation frees it up, and there is no way to tell which of them "PNR
// digitado errado" meant without asking, so the bot doesn't guess.
func (b *Bot) handleDeletar(evt *events.Message, text string, startedAt time.Time) {
	user, allowed := b.authorizeSender(evt, "deletar")
	if !allowed {
		return
	}

	msg, err := parseDeletar(text)
	if err != nil {
		b.Logger.Infof("deletar: could not parse a message from %s: %v", user.Name, err)
		b.refuse(evt, startedAt, refusalText("", err))
		return
	}

	err = b.DB.Transaction(func(tx *gorm.DB) error {
		issuance, err := findIssuanceToDelete(tx, msg.ProgramLocator)
		if err != nil {
			return err
		}
		return issuancesvc.Delete(tx, issuance, user.ID)
	})

	if err != nil {
		b.Logger.Errorf("deletar: failed to delete %s for %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	b.Logger.Infof("deletar: deleted issuance %s from %s", msg.ProgramLocator, user.Name)

	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(evt, "🚀")

	b.notifyIssuanceDeleted()
}

// findIssuanceToDelete locates the active issuance a locator names - see the
// handler's doc comment for why only the active one is ever a candidate.
func findIssuanceToDelete(tx *gorm.DB, locator string) (data.Issuance, error) {
	var issuance data.Issuance

	err := tx.Where("program_locator = ? AND active = ?", locator, true).First(&issuance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return issuance, operatorErrorf(err, "Não encontrei nenhuma emissão ativa com o localizador %q", locator)
	}
	return issuance, err
}
