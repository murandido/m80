package bot

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// cmdPDF turns the ticket PDF on or off for the chat it was sent in.
//
// `!pdf` on its own flips the state, which is what an operator typing a switch
// into a group expects. Nobody in the group can see the current state before
// typing, and the answer is what closes that gap: it names the state the chat
// ended in, so a flip in the wrong direction is visible immediately and one more
// `!pdf` undoes it. `!pdf on` / `!pdf off` stay for whoever wants a final state
// that does not depend on the current one.
//
// This is the only command besides !ping that replies with text, for the same
// reason: an emoji cannot say "on" or "off".
func (b *Bot) cmdPDF(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	user, err := b.resolveSenderUser(evt)
	if err != nil {
		b.Logger.Errorf("pdf: failed to resolve sender %s: %v", evt.Info.Sender, err)
		return
	}
	if user == nil {
		// Same silence as !registrar-chat: someone typing in a chat the bot is
		// merely present in must not learn that it reads.
		b.Logger.Infof("pdf: sender %s is not a known user, ignoring", evt.Info.Sender)
		return
	}

	chatJID := evt.Info.Chat.String()

	var chat data.ListenedChat
	err = b.DB.WithContext(ctx).First(&chat, "chat_jid = ? AND status = ? AND active = ?", chatJID, "approved", true).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Not a chat the bot writes for, so there is no EMISSAO here to attach
			// a PDF to. Unlike the gate on an EMISSAO this one answers: "!pdf" is
			// addressed at the bot, and the sender is a known user, so they already
			// know it is listening.
			b.Logger.Infof("pdf: chat %s is not being monitored, refusing", chatJID)
			b.refuse(ctx, evt, startedAt, "Esse chat não está sendo monitorado, então não há emissão aqui para gerar PDF")
			return
		}
		b.Logger.Errorf("pdf: failed to load listened_chats for %s: %v", chatJID, err)
		b.refuse(ctx, evt, startedAt, "Não consegui ler a configuração desse chat, verifique com o suporte")
		return
	}

	var wanted bool
	switch parsePDFIntent(text) {
	case pdfIntentUnreadable:
		b.answerPDFState(ctx, evt, startedAt, chat.PDFMode)
		return
	case pdfIntentToggle:
		wanted = !chat.PDFMode
	case pdfIntentOn:
		wanted = true
	case pdfIntentOff:
		wanted = false
	}

	if wanted != chat.PDFMode {
		if err := b.DB.WithContext(ctx).Model(&chat).Update("pdf_mode", wanted).Error; err != nil {
			b.Logger.Errorf("pdf: failed to set pdf_mode=%v for %s: %v", wanted, chatJID, err)
			b.refuse(ctx, evt, startedAt, "Não consegui salvar essa configuração, verifique com o suporte")
			return
		}
		b.Logger.Infof("pdf: %s set pdf_mode=%v for %s", user.Name, wanted, chatJID)
	}

	// 🚀 even when the state was already what the message asked for - "what you
	// asked for is already true" is a success, the same rule a no-op correction
	// and a duplicate !registrar-chat already follow.
	b.answerPDFState(ctx, evt, startedAt, wanted)
}

// pdfIntent is what the sender asked for. "said nothing" and "said something I
// cannot read" are separate on purpose: the first is a deliberate flip, the
// second must not be guessed at in either direction.
type pdfIntent int

const (
	pdfIntentToggle     pdfIntent = iota // "!pdf" - flip whatever the chat is on now
	pdfIntentOn                          // on / ligar / ativar
	pdfIntentOff                         // off / desligar / desativar
	pdfIntentUnreadable                  // "!pdf sim" - report the state, change nothing
)

// parsePDFIntent reads the argument of a !pdf message.
func parsePDFIntent(text string) pdfIntent {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(fields) < 2 {
		return pdfIntentToggle
	}
	switch fields[1] {
	case "on", "ligar", "ativar":
		return pdfIntentOn
	case "off", "desligar", "desativar":
		return pdfIntentOff
	default:
		// Guessing at "!pdf sim" would be worse than showing the state and letting
		// the operator try again - unlike a bare !pdf, this one carries an
		// intention the bot failed to read, so flipping could be the opposite of
		// what was asked.
		return pdfIntentUnreadable
	}
}

func (b *Bot) answerPDFState(ctx context.Context, evt *events.Message, startedAt time.Time, on bool) {
	message := "Modo PDF *desligado* neste chat."
	if on {
		message = "Modo PDF *ligado* neste chat: vou mandar o bilhete em PDF junto de cada emissão completa."
	}

	b.SendPresenceTyping(ctx, evt)
	humanizer.DelayComposing()

	humanizer.EnforceResponseFloor(startedAt)
	b.ReplyMessage(ctx, evt, message)
	// Closes the 🕐 the pipeline opened. A command that answers with text alone
	// leaves the clock on screen forever, because a reaction is only ever
	// replaced, never cleared - see cmdPing.
	b.ReactMessage(ctx, evt, "🚀")
}

// pdfModeEnabled reports whether this chat wants the ticket as a file. A failure
// to read it is not a failure of the emission: the issuance is already saved and
// the group already has its 🚀, so the answer is "no PDF" plus a log line.
func (b *Bot) pdfModeEnabled(ctx context.Context, chatJID string) bool {
	var chat data.ListenedChat
	if err := b.DB.WithContext(ctx).Select("pdf_mode").First(&chat, "chat_jid = ?", chatJID).Error; err != nil {
		b.Logger.Errorf("pdf: failed to read pdf_mode for %s: %v", chatJID, err)
		return false
	}
	return chat.PDFMode
}
