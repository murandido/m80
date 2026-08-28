package bot

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/promotion"
	"go.mau.fi/whatsmeow/types/events"
	"gorm.io/gorm"
)

// handlePromo enrols the agency's CPFs in the campaign a link points at.
//
// One trigger: a bare link, in a chat with promo_mode on, for a domain some active
// recipe covers. Somebody sees a promotion, pastes it in the group, and the enrolment
// happens - which is what the operators asked for, and it replaced a "!promo <link>"
// command that was only ceremony around the same link.
//
// Nothing here guesses. When no recipe covers the domain, or the page yields no form,
// the group is told and nothing is submitted: a wrong guess still looks like success,
// and an enrolment that quietly went to the wrong form is the same loss as forgetting
// to enrol at all - which is the whole problem this exists to solve.
func (b *Bot) handlePromo(ctx context.Context, evt *events.Message, text string, startedAt time.Time) {
	user, err := b.resolveSenderUser(evt)
	if err != nil {
		b.Logger.Errorf("promo: failed to resolve sender %s: %v", evt.Info.Sender, err)
		return
	}
	if user == nil {
		// Same silence as every other gate: someone typing in a chat the bot is
		// merely present in must not learn that it reads.
		b.Logger.Infof("promo: sender %s is not a known user, ignoring", evt.Info.Sender)
		return
	}

	chatJID := evt.Info.Chat.String()

	link, found := promotion.FindLink(text)
	if !found {
		// Unreachable through intake: a message with no link is never classified as a
		// promotion. Silence rather than a refusal, because the handler stays callable
		// directly and this says nothing a group needs to hear.
		b.Logger.Warnf("promo: %q reached the handler with no link in it", text)
		return
	}

	// The handler keeps its own gate because it is still reachable directly, and
	// because neither check writes anything. Everything it turns away here, mayAct has
	// already turned away in silence.
	listened, promoOn := b.promoChatState(ctx, chatJID)
	if !listened || !promoOn {
		b.Logger.Infof("promo: chat %s is not set up for enrolments, ignoring", chatJID)
		return
	}

	if b.Promo == nil {
		// Same tolerance the Traveos client gets: without PROMO_SERVICE_URL the bot
		// still does everything else, and this path says so rather than panicking.
		b.Logger.Warnf("promo: no promo service configured, refusing %s", link)
		b.refuse(ctx, evt, startedAt, "O serviço de cadastro em promoções não está configurado, verifique com o suporte")
		return
	}

	// ⏳, not 🕐. A promotion job skips the chat queue, so intake never put a clock
	// on it - and a clock would be the wrong fact anyway: this is not a wait, it is
	// work. A browser run is roughly twelve seconds per CPF, long enough that silence
	// reads as a crash. No humanizer delay: it is a progress indicator, and delaying it
	// would defeat the point. Every path below replaces it.
	b.ReactMessage(ctx, evt, reactionFetching)
	b.SendPresenceTyping(ctx, evt)

	result, err := promotion.Run(ctx, b.DB.WithContext(ctx), b.Promo, promotion.RunRequest{
		URL:         link,
		ChatJID:     &chatJID,
		CreatedByID: &user.ID,
	})
	if err != nil {
		b.refusePromo(ctx, evt, startedAt, link, err)
		return
	}

	// After the rows are written, never before: an event about work that then failed
	// to commit is worse than no event.
	b.notifyPromoEnrolled()

	b.sendPromoProofs(ctx, evt, result)
	b.answerPromoSummary(ctx, evt, startedAt, result)
}

// refusePromo turns a typed failure into a sentence the group can act on. The domain
// and the reason are named on purpose: a refusal that says only "deu erro" leaves
// nobody able to fix the recipe before the campaign closes.
func (b *Bot) refusePromo(ctx context.Context, evt *events.Message, startedAt time.Time, link string, err error) {
	switch {
	case errors.Is(err, promotion.ErrNoRecipe):
		domain, _ := promotion.DomainOf(link)
		b.Logger.Infof("promo: no recipe for %s", domain)
		b.refuse(ctx, evt, startedAt, fmt.Sprintf(
			"Ainda não sei cadastrar em *%s*. Cadastre a receita desse site no painel, em Automação de Promoções.", domain))

	case errors.Is(err, promotion.ErrFormNotResolved):
		b.Logger.Warnf("promo: form not resolved for %s: %v", link, err)
		b.refuse(ctx, evt, startedAt,
			"Não encontrei o formulário de cadastro nessa página, então não submeti nada. "+
				"Use o botão *Testar* no painel para ver o que a página mostra e ajustar a receita.")

	case errors.Is(err, promotion.ErrNoDocuments):
		b.refuse(ctx, evt, startedAt,
			"Não há nenhum CPF ativo cadastrado para o programa dessa promoção. Cadastre em CPFs das Promoções.")

	default:
		b.Logger.Errorf("promo: enrolment failed for %s: %v", link, err)
		b.refuse(ctx, evt, startedAt, "Não consegui completar o cadastro nessa promoção, verifique com o suporte")
	}
}

// sendPromoProofs posts one screenshot per CPF. A failure to send an image is logged
// and never escalated: the enrolment already happened and is already recorded, and
// losing the picture must not turn a good run into a bad one.
func (b *Bot) sendPromoProofs(ctx context.Context, evt *events.Message, result promotion.RunResult) {
	for _, outcome := range result.Outcomes {
		if len(outcome.Screenshot) == 0 {
			continue
		}
		caption := fmt.Sprintf("%s — %s", outcome.Document.Name, promoStatusLabel(outcome.Status))
		if err := b.SendImage(ctx, evt, outcome.Screenshot, caption); err != nil {
			b.Logger.Errorf("promo: failed to send proof for document %s: %v", outcome.Document.ID, err)
		}
	}
}

// answerPromoSummary closes the run with a count and, when something went wrong, the
// names behind it. Partial success is reported as a warning rather than a success:
// a CPF silently left out of a transfer bonus is exactly the loss this feature exists
// to prevent, so it can never be rounded up into a 🚀.
func (b *Bot) answerPromoSummary(ctx context.Context, evt *events.Message, startedAt time.Time, result promotion.RunResult) {
	succeeded := result.Succeeded()
	total := len(result.Outcomes)

	var lines []string
	lines = append(lines, fmt.Sprintf("*%s* — %d de %d cadastrados.",
		programName(result.Recipe), succeeded, total))

	var problems []string
	for _, outcome := range result.Outcomes {
		if outcome.Status != data.PromoEnrollmentSuccess {
			problems = append(problems, fmt.Sprintf("• %s — %s",
				outcome.Document.Name, promoStatusLabel(outcome.Status)))
		}
	}
	if len(problems) > 0 {
		lines = append(lines, "", "Não entraram:")
		lines = append(lines, problems...)
	}

	humanizer.DelayComposing()
	humanizer.EnforceResponseFloor(startedAt)
	b.ReplyMessage(ctx, evt, strings.Join(lines, "\n"))

	if succeeded == total {
		b.ReactMessage(ctx, evt, "🚀")
		return
	}
	b.ReactMessage(ctx, evt, "⚠️")
}

func promoStatusLabel(status string) string {
	switch status {
	case data.PromoEnrollmentSuccess:
		return "cadastrado"
	case data.PromoEnrollmentFailure:
		return "recusado pelo site"
	case data.PromoEnrollmentUnknown:
		return "sem resposta reconhecível"
	case data.PromoEnrollmentRefused:
		return "não cadastrado"
	default:
		return "erro"
	}
}

func programName(recipe *data.PromoRecipe) string {
	if recipe == nil {
		return "promoção"
	}
	if recipe.LoyaltyProgram != nil && recipe.LoyaltyProgram.Name != "" {
		return recipe.LoyaltyProgram.Name
	}
	return recipe.Name
}

// promoChatState answers the two questions the promo gate needs to keep apart.
//
// listened: the bot works for this chat at all - approved and active. A chat that is
// not one of ours gets silence, the same as every other flow, and for the same reason:
// answering tells whoever typed that the bot is reading. That matters concretely when a
// number is paired in more than one environment, where the chat is registered in one and
// unknown in the other and both used to reply.
//
// promoOn: enrolments are switched on there. A registered chat with promotions off is
// worth an answer, because whoever asked can go turn it on.
func (b *Bot) promoChatState(ctx context.Context, chatJID string) (listened, promoOn bool) {
	var chat data.ListenedChat
	err := b.DB.WithContext(ctx).
		Select("promo_mode").
		First(&chat, "chat_jid = ? AND status = ? AND active = ?", chatJID, "approved", true).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			b.Logger.Errorf("promo: failed to read promo_mode for %s: %v", chatJID, err)
		}
		return false, false
	}
	return true, chat.PromoMode
}

// promoRecipeExistsFor reports whether any active recipe covers this link's domain.
//
// It runs in the gate rather than in the handler because the gate is upstream of every
// reaction: a promotion link for a site we do not drive has to be as invisible to the
// bot as any other message, and reaching the handler would mean a ✅ and a refusal for
// every promotion anybody shares in the group.
//
// ResolveRecipe owns the matching rule - suffix match, most specific domain wins - and
// is reused rather than reimplemented here.
func (b *Bot) promoRecipeExistsFor(ctx context.Context, link string) bool {
	_, err := promotion.ResolveRecipe(b.DB.WithContext(ctx), link)
	if err == nil {
		return true
	}
	if !errors.Is(err, promotion.ErrNoRecipe) {
		// A database that is down must not look like a domain we do not cover, so it is
		// at least loud in the log. The message still gets no answer: guessing in the
		// permissive direction would spend enrolments.
		b.Logger.Errorf("promo: failed to resolve a recipe for %s: %v", link, err)
	}
	return false
}

// isPromoLinkMessage reports whether a message is nothing but a link.
//
// Deliberately strict. A link inside a sentence is somebody discussing a promotion, and
// enrolling on that would spend the agency's CPFs on chatter. Only a message that *is*
// the link counts - which is also how these get shared in practice, forwarded straight
// from wherever they were found.
func isPromoLinkMessage(text string) bool {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) != 1 {
		return false
	}
	_, found := promotion.FindLink(fields[0])
	return found
}
