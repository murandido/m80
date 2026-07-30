package bot

import (
	"context"
	"errors"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// interruptedJobGrace bounds how long DELETAR waits for the job it just
// cancelled to unwind before it looks at the database. It only ever waits for a
// rollback, so it is generous by design rather than tuned - and expiring it
// costs nothing, because the database check below is what actually decides.
const interruptedJobGrace = 10 * time.Second

// handleDeletar removes the issuance a DELETAR message names, wherever it
// happens to be. Same gates as the other three triggers - see authorizeSender.
//
// The product doc gives the locator three possible states, and this covers all
// of them in one pass:
//
//   - queued or running: cancel it. A queued job never starts; a running one has
//     its context pulled, which aborts its statement and rolls its transaction
//     back. Nothing is saved, and the cancelled job answers nothing - this
//     message is the answer.
//   - already saved: soft delete, exactly as before.
//   - nowhere at all: "não encontrei".
//
// Cancelling first and checking the database afterwards is what makes the race
// harmless. A job cancelled a moment too late - after its commit but before it
// returned - leaves a row behind, and the check finds it and deletes it like any
// other. Reporting success off the cancellation alone would have claimed the
// issuance was stopped while it sat in the table.
//
// DELETAR only ever targets the active issuance for a locator - never an
// older, already-cancelled one that happens to share it after the locator was
// reused. A locator can legitimately have more than one historical row once a
// cancellation frees it up, and there is no way to tell which of them "PNR
// digitado errado" meant without asking, so the bot doesn't guess.
func (b *Bot) handleDeletar(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	user, allowed := b.authorizeSender(evt, "deletar")
	if !allowed {
		return
	}

	msg, err := parseDeletar(text)
	if err != nil {
		b.Logger.Infof("deletar: could not parse a message from %s: %v", user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText("", err))
		return
	}

	interrupted := b.jobs().cancelLocator(msg.ProgramLocator, string(evt.Info.ID))
	for _, stopped := range interrupted {
		b.Logger.Infof("deletar: interrupted the job started by message %s for %s",
			stopped.evt.Info.ID, msg.ProgramLocator)
		stopped.awaitFinish(interruptedJobGrace)
	}

	var deleted bool
	err = b.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		issuance, err := findIssuanceToDelete(tx, msg.ProgramLocator)
		if err != nil {
			// Nothing in the table and a job stopped a moment ago: the message
			// was caught before it ever got written, which is the whole point.
			// (operatorError unwraps, so the sentinel is still visible through
			// the friendly text findIssuanceToDelete wrapped it in.)
			if errors.Is(err, gorm.ErrRecordNotFound) && len(interrupted) > 0 {
				return nil
			}
			return err
		}

		deleted = true
		return issuancesvc.Delete(tx, issuance, user.ID)
	})

	if err != nil {
		b.Logger.Errorf("deletar: failed to delete %s for %s: %v", msg.ProgramLocator, user.Name, err)
		b.refuse(ctx, evt, startedAt, refusalText(msg.ProgramLocator, err))
		return
	}

	if deleted {
		b.Logger.Infof("deletar: deleted issuance %s from %s", msg.ProgramLocator, user.Name)
	} else {
		b.Logger.Infof("deletar: %s was stopped before it reached the database, nothing to delete", msg.ProgramLocator)
	}

	humanizer.DelayReaction()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReactMessage(ctx, evt, "🚀")

	// Only a row that actually changed is worth waking the panel's grid for.
	if deleted {
		b.notifyIssuanceDeleted()
	}
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
