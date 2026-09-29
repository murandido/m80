package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/murandido/m-80-backend/internal/bot/humanizer"
	"github.com/murandido/m-80-backend/internal/promotion"
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

// EventHandler handles the necessary events from whatsmeow
func (b *Bot) EventHandler(evt interface{}) {
	switch v := evt.(type) {
	case *events.Message:
		if v.Info.IsFromMe {
			return
		}

		// One line per stanza that claims to change an earlier message - an edit,
		// a pin, a delete. Ordinary traffic logs nothing, and when a correction by
		// edit does not land this is what says whether it ever reached the bot.
		if v.Info.Edit != "" {
			b.Logger.Infof("received a stanza with edit=%q for message %s from %s",
				v.Info.Edit, v.Info.ID, v.Info.Sender)
		}

		// Read synchronously, right as the event arrives: whatsmeow
		// dispatches OfflineSyncPreview/Completed and the backlog of
		// *events.Message in between them in order, so this is the accurate
		// point to decide whether this particular message is part of that
		// backlog - not whenever the queue below gets to it.
		isBacklog := b.offlineSyncing.Load()

		// startedAt anchors the humanizer's response floor - captured here,
		// as close to the message's actual arrival as possible.
		startedAt := time.Now()

		// Appended to the chat's queue rather than handed to a goroutine of
		// its own. A goroutine per message left ordering to the scheduler and
		// to a random 0.5-1.5s sleep, which is how an ALTERACAO could overtake
		// the EMISSAO it corrects; a channel keeps whatsmeow's arrival order.
		// The send never blocks and never does I/O - see dispatcher.submit.
		b.queue().submit(queuedMessage{evt: v, isBacklog: isBacklog, startedAt: startedAt})

	case *events.OfflineSyncPreview:
		if v.Total > 0 {
			b.offlineSyncing.Store(true)
			b.Logger.Infof("catching up on %d message(s) missed while offline", v.Total)
		}

	case *events.OfflineSyncCompleted:
		b.offlineSyncing.Store(false)
		b.Logger.Infof("caught up on missed events (%d)", v.Count)

	case *events.Connected:
		jid := ""
		if id := b.currentSession().GetJID(); id != nil {
			jid = id.String()
		}
		b.Status.Set(Status{State: StateConnected, JID: jid})

	case *events.Disconnected:
		b.Status.Set(Status{State: StateDisconnected})

	case *events.LoggedOut:
		b.Logger.Infof("session logged out by the phone, reason: %v", v.Reason)
		b.Status.Set(Status{State: StateDisconnected})

		// whatsmeow dispatches this event from its own goroutine and only
		// deletes the device store afterwards, in the goroutine that
		// triggered the logout - the two aren't synchronized. Racing
		// RecreateSession against that delete is unreliable: asking for a
		// new device before the old row is actually gone hands back a
		// client that still thinks it has a paired ID (GetQRChannel then
		// refuses with "no user ID in the client's Store"). Deleting it
		// ourselves first - idempotent even if whatsmeow's own delete also
		// runs - gives RecreateSession a guaranteed-clean slate. Backgrounded
		// so a slow DB round-trip never blocks the event dispatch goroutine.
		go func() {
			ctx := context.Background()
			if err := b.currentSession().Session.DeleteStore(ctx); err != nil {
				b.Logger.Warnf("failed to delete stale device store: %v", err)
			}
			if err := b.RecreateSession(ctx); err != nil {
				b.Logger.Errorf("failed to set up a fresh session after remote logout: %v", err)
			}
		}()

	case *events.PairSuccess:
		// The socket reconnects right after this; *events.Connected is what
		// actually flips the status to StateConnected.
		b.Logger.Infof("pair success for %s, waiting for reconnect", v.ID)
	}

}

// processIncomingMessage runs the whole pipeline inline, without the queue:
// admit the message, then execute it. It is the same two halves the chat queue
// drives from its two goroutines (see chatQueue), collapsed into one call for
// tests that want the full path without a dispatcher behind it.
func (b *Bot) processIncomingMessage(v *events.Message, isBacklog bool, startedAt time.Time) {
	j := b.admit(queuedMessage{evt: v, isBacklog: isBacklog, startedAt: startedAt})

	<-j.acked
	if j.kind != kindNone {
		b.executeJob(j)
	}
}

// admit is the intake half: it unwraps the message and decides what it is. A
// job of kindNone is everything the bot has no business executing - backlog
// redelivery and ordinary conversation - and the worker skips it.
//
// What admit does *not* do is sleep. Everything the group can see - marking as
// read, and later the acknowledgement reactions - is fired off in a goroutine of
// this message's own and awaited through job.acked. That matters because intake
// is serial per chat: with the humanizer's 0.5-1.5s read delay inline, a burst
// of twenty messages would take twenty of those before the last one was even
// looked at. Only ordering-sensitive work belongs on this goroutine, and none of
// the visible layer is ordering-sensitive.
func (b *Bot) admit(m queuedMessage) *job {
	// Ahead of marking as read, which already needs the id of the message the
	// group can see rather than the id of the edit stanza.
	b.unwrapEdit(m.evt)

	text := utils.GetText(m.evt.Message)

	kind := kindNone
	if !m.isBacklog {
		kind = b.classify(m.evt, text)
	}
	// The gate decides whether the bot may act at all, and it runs here rather
	// than in the handlers because ✅ must never be sent to someone the bot is
	// going to ignore: a rejected gate produces no reaction whatsoever, so
	// nobody typing in a group the bot merely happens to be in learns that it
	// reads. The handlers keep their own gate as well - they are still reachable
	// directly, and neither check writes anything.
	if kind != kindNone && !b.mayAct(m.evt, kind, text) {
		kind = kindNone
	}

	// An edit of a message whose job has not finished yet is not a correction of
	// anything saved - there is nothing saved. It rewrites the work instead, and
	// when that work was still queued it does so without producing a job of its
	// own: the same job, with new content, in the same place in line.
	acknowledgeOnly := false
	if kind == kindEdit && b.supersede(m.evt, text) {
		kind, acknowledgeOnly = kindNone, true
	}

	j := &job{
		evt:       m.evt,
		text:      text,
		kind:      kind,
		locator:   locatorOf(kind, text),
		startedAt: m.startedAt,
		state:     jobQueued,
		acked:     make(chan struct{}),
		finished:  make(chan struct{}),
	}
	if kind != kindNone {
		// Tracked from here rather than from the worker, so an edit or a
		// deletion arriving while the job is still queued can find it.
		b.jobs().track(j)
	}

	b.queue().spawn(func() {
		defer close(j.acked)

		// Deliberately not the job's context: these run before the job starts,
		// and they are what tells the group the bot is on it. A DELETAR that
		// cancels the job a moment later must not retroactively swallow the read
		// receipt and the ✅ the sender already saw.
		ctx := context.Background()

		// Read receipts are about the bot having seen the chat, not about it
		// having decided to act, so this happens for every message - backlog,
		// small talk and all - exactly as it did before the queue existed.
		humanizer.DelayMarkAsRead()
		b.MarkMessageAsRead(ctx, m.evt)

		if j.kind == kindNone && !acknowledgeOnly {
			return
		}

		// A deletion is the one admitted kind that says nothing: the message the
		// bot would react to is the one that was just removed from the chat.
		if j.kind == kindRevoke {
			return
		}

		// ✅ first: the bot saw the message. Then 🕐, unless this is the one
		// trigger that never waits - a clock on a DELETAR would claim a queue it
		// deliberately skipped. A reaction replaces the previous one, so what the
		// group sees is a single indicator moving forward, ending on the 🚀 or ❌
		// the handler sends.
		b.acknowledge(ctx, m.evt)
		if !j.skipsQueue() {
			b.markQueued(ctx, m.evt)
		}
	})

	return j
}

// classify decides which handler owns a message, and returns kindNone for
// everything the bot will not answer.
//
// None of the four business messages carries a "!" prefix - all are ordinary
// text the operators were already writing among themselves - so unlike a command
// they cannot be recognized by addressing alone. isEmissaoMessage,
// isAlteracaoMessage, isCancelamentoMessage and isDeletarMessage are the cheap
// first filter; whether the bot may actually act on the message is mayAct's
// question, not this one's.
//
// Everything admitted here must end in a reaction from its handler. Intake
// answers ✅ and then 🕐 before any work starts, so a kind whose handler can
// decide to do nothing at all would leave a clock on screen forever - which is
// why an unknown command and an edit of ordinary chatter are turned away here
// rather than ignored further down.
//
// The edit case comes first because an edited EMISSAO still looks like an
// EMISSAO: what makes it a correction rather than a new issuance is only that
// it replaced a message already sent.
func (b *Bot) classify(v *events.Message, text string) triggerKind {
	switch {
	case isMessageRevoke(v):
		// Ahead of the edit case: a revoke rides the same ProtocolMessage shape,
		// and carries no text for anything downstream to classify on.
		return kindRevoke
	case v.IsEdit:
		if isEmissaoMessage(text) || isAlteracaoMessage(text) {
			return kindEdit
		}
		// Ignoring is right, but doing it without a word is what once made a
		// broken unwrap indistinguishable from an edit of ordinary chatter. The
		// two are logged apart, and neither quotes the group's text into the log.
		if text == "" {
			b.Logger.Warnf("edicao: edit %s carried no readable text - the payload was not unwrapped", v.Info.ID)
		} else {
			b.Logger.Infof("edicao: edit %s is neither an EMISSAO nor an ALTERACAO, ignoring", v.Info.ID)
		}
		return kindNone
	default:
		return classifyText(text)
	}
}

// classifyText is classify without the edit question: what this text would be if
// it had been typed fresh. It is also what an edit of work still in progress is
// read as, since replacing a queued message's content is not a correction of
// anything - there is nothing saved yet to correct.
func classifyText(text string) triggerKind {
	switch {
	case utils.IsCommand(text):
		if isKnownCommand(text) {
			return kindCommand
		}
		return kindNone
	case isEmissaoMessage(text):
		return kindEmissao
	case isAlteracaoMessage(text):
		return kindAlteracao
	case isCancelamentoMessage(text):
		return kindCancelamento
	case isDeletarMessage(text):
		return kindDeletar
	case isPromoLinkMessage(text):
		// Last, so a link inside an EMISSAO can never take the message over. Whether
		// this chat may actually act on the link is decided by mayAct, which has the
		// chat to check promo_mode against and the database to check the domain.
		//
		// Its own kind rather than kindCommand: an enrolment run is minutes of browser
		// work, and kindPromo is what lets it skip the chat queue instead of holding
		// every EMISSAO behind it.
		return kindPromo
	default:
		return kindNone
	}
}

// supersede applies a native edit to the job the edited message already started,
// and reports whether that settled it - i.e. whether the edit needs no job of its
// own.
//
// The three states of the product doc's edit table are decided here:
//
//  1. still queued: the job is rewritten in place, keeping its position in the
//     chat's queue, and this edit produces nothing further. The old content
//     never runs.
//  2. already running: the job is cancelled, which rolls back whatever it had
//     started, and the edit goes on to be handled as an ordinary correction.
//     That path re-registers the issuance if nothing is there and corrects it if
//     something is - which is exactly right, because cancelling a transaction a
//     moment too late leaves a committed row behind.
//  3. finished, or never a job at all: nothing live to supersede, and the edit
//     is a correction of a saved issuance like any other.
func (b *Bot) supersede(evt *events.Message, text string) bool {
	original, found := b.jobs().lookupByMessage(string(evt.Info.ID))
	if !found {
		return false
	}

	replacement := classifyText(text)
	if replacement == kindNone {
		// Edited into something the bot does not act on. Acting on text the
		// operator has replaced would be wrong, so the job is dropped - and said
		// so, because the message is still sitting there under a 🕐.
		// false: the refusal just below answers on this very message.
		if original.cancel(false) {
			b.Logger.Infof("edicao: message %s was edited into something that is not a bot message, dropping its job", evt.Info.ID)
			b.queue().spawn(func() {
				b.refuse(context.Background(), evt, time.Now(),
					"A mensagem editada não é mais uma EMISSAO, ALTERACAO, CANCELAMENTO ou DELETAR, então cancelei o que estava pendente")
			})
			return true
		}
		return false
	}

	if b.jobs().replaceIfQueued(original, text, replacement, locatorOf(replacement, text)) {
		b.Logger.Infof("edicao: message %s was edited while still queued, its job now carries the new content", evt.Info.ID)
		return true
	}

	// false: the edit is about to be processed in this job's place, and it
	// acknowledges and answers on the same message all over again.
	if original.cancel(false) {
		b.Logger.Infof("edicao: message %s was edited while its job was running, cancelling it", evt.Info.ID)
		original.awaitFinish(interruptedJobGrace)
	}
	return false
}

// mayAct applies the gate that fits the trigger. They are not the same gate,
// because !registrar-chat is the one thing that has to work in a chat the bot is
// not yet allowed to listen to - demanding an approved chat there would make the
// chat impossible to ever register.
//
// !ping stays ungated, as it has always been: it is a liveness probe that reads
// nothing and writes nothing, and it already announced the bot's presence to
// anyone who sent it long before this pipeline existed.
func (b *Bot) mayAct(evt *events.Message, kind triggerKind, text string) bool {
	if kind == kindPromo {
		user, err := b.resolveSenderUser(evt)
		if err != nil {
			b.Logger.Errorf("promo: failed to resolve sender %s: %v", evt.Info.Sender, err)
			return false
		}
		if user == nil {
			return false
		}
		listened, promoOn := b.promoChatState(context.Background(), evt.Info.Chat.String())
		if !listened {
			// Not a chat the bot works for. Silence, like every other flow: an answer
			// here would tell whoever typed that the bot is reading. It also stops two
			// environments sharing one paired number from both reacting to the same
			// link when only one of them knows the chat.
			return false
		}
		// A bare link is only a command where the chat opted in. Everywhere else it
		// stays what it looks like: somebody sharing a link.
		if !promoOn {
			return false
		}
		// And only for a site we actually drive. We always know the domains we handle,
		// so anything else is somebody sharing a promotion, not asking for one.
		link, found := promotion.FindLink(text)
		return found && b.promoRecipeExistsFor(context.Background(), link)
	}

	if kind == kindCommand {
		if commandName(text) == cmdNamePing {
			return true
		}
		user, err := b.resolveSenderUser(evt)
		if err != nil {
			b.Logger.Errorf("comando: failed to resolve sender %s: %v", evt.Info.Sender, err)
			return false
		}
		if user == nil {
			b.Logger.Infof("comando: sender %s is not a known user, ignoring", evt.Info.Sender)
			return false
		}
		return true
	}

	_, _, allowed := b.authorizeSender(evt, flowNames[kind])
	return allowed
}

// flowNames label the gate's log lines with the flow the message belongs to.
var flowNames = map[triggerKind]string{
	kindEmissao:      "emissao",
	kindAlteracao:    "alteracao",
	kindCancelamento: "cancelamento",
	kindDeletar:      "deletar",
	kindEdit:         "edicao",
	kindRevoke:       "delecao",
	kindPromo:        "promo",
}

// locatorOf reads the PNR off the message's first line, reusing the very header
// parsers the handlers use so a locator can never be read one way here and
// another way there.
//
// A message that does not parse gets no locator and therefore no lock. That is
// correct rather than lenient: the handler is about to refuse it anyway, and
// locking a PNR extracted from a line the parser rejected would block a
// well-formed message on the strength of a typo.
func locatorOf(kind triggerKind, text string) string {
	lines := splitNonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}

	var (
		locator string
		err     error
	)
	switch kind {
	case kindEmissao:
		var msg emissaoMessage
		if err = parseEmissaoHeader(lines[0], &msg); err == nil {
			locator = msg.ProgramLocator
		}
	case kindAlteracao:
		locator, err = parseAlteracaoHeader(lines[0])
	case kindCancelamento:
		locator, err = parseCancelamentoHeader(lines[0])
	case kindDeletar:
		locator, err = parseDeletarHeader(lines[0])
	case kindEdit:
		// An edit is whichever of the two it looks like; classify already
		// established it is one of them.
		if isEmissaoMessage(text) {
			return locatorOf(kindEmissao, text)
		}
		return locatorOf(kindAlteracao, text)
	case kindNone, kindCommand, kindRevoke:
		// A revoke carries no text at all - only the id of the message it
		// removed, which is how handleMessageRevoke finds its target instead.
		return ""
	}

	if err != nil {
		return ""
	}
	return locator
}

// executeJob is the worker half: it takes the locator's lock, hands the job to
// its handler, and lets go of both afterwards.
//
// startedAt travels with the job so the humanizer enforces its response floor
// against the message's real arrival time, not against whenever the worker got
// around to it - a job that waited in the queue has already outlived the floor
// and answers immediately.
func (b *Bot) executeJob(j *job) {
	defer b.jobs().untrack(j)

	// Cancelled while it sat in the queue: a DELETAR or a native deletion got
	// here first. Nothing was written, so there is nothing to undo and nothing to
	// say beyond taking the clock off the message.
	ctx, live := j.begin(context.Background())
	if !live {
		b.Logger.Infof("fila: job for message %s was cancelled before it started", j.evt.Info.ID)
		b.closeInterrupted(j)
		return
	}
	// A cancellation landing mid-run gets the same closing mark. The handlers
	// stay quiet on a cancelled context - refuse checks for exactly this - so
	// without it the message would keep its 🕐 whether the job was stopped before
	// or during its run.
	//
	// Registered before j.finish so it runs after it: whoever cancelled the job
	// is waiting on finished to read the database, and must not be held up behind
	// the humanizer's pause on a reaction.
	defer b.closeInterrupted(j)
	defer j.finish()

	// Read once, now that the job is running and a native edit can no longer
	// rewrite it underneath - see jobRegistry.replaceIfQueued.
	text, kind, locator := j.payload()

	// DELETAR never takes the lock: it exists to interrupt whoever is holding
	// it, so contending for it would deadlock the one command meant to break
	// the deadlock.
	if !j.skipsQueue() && !b.jobs().acquire(j, locator) {
		b.Logger.Infof("lock: %s is already being processed by another chat, refusing %s",
			locator, j.evt.Info.ID)
		b.refuse(ctx, j.evt, j.startedAt, fmt.Sprintf("A emissão %s já está sendo processada", locator))
		return
	}

	switch kind {
	case kindEdit:
		b.handleMessageEdit(ctx, j.evt, text, j.startedAt)
	case kindCommand:
		b.RouteCommand(ctx, j.evt, text, j.startedAt)
	case kindPromo:
		b.handlePromo(ctx, j.evt, text, j.startedAt)
	case kindEmissao:
		b.handleEmissao(ctx, j.evt, text, j.startedAt)
	case kindAlteracao:
		b.handleAlteracao(ctx, j.evt, text, j.startedAt)
	case kindCancelamento:
		b.handleCancelamento(ctx, j.evt, text, j.startedAt)
	case kindDeletar:
		b.handleDeletar(ctx, j.evt, text, j.startedAt)
	case kindRevoke:
		b.handleMessageRevoke(ctx, j.evt, j.startedAt)
	case kindNone:
		// Never admitted; listed so a new kind cannot be forgotten here.
	}
}

// isMessageEdit reports whether this stanza is a live edit of an earlier message.
//
// Info.Edit is the authoritative signal: whatsmeow fills it straight from the
// stanza's "edit" attribute in parseMessageInfo, before decrypting anything, for
// every incoming message. It is the only marker that does not depend on how the
// payload happens to be wrapped - and it is what separates an edit ("1") from the
// other things that ride the same ProtocolMessage shape: pinning ("2") and the two
// flavours of delete ("7"/"8"), none of which are corrections.
//
// Neither of the signals tried before holds on its own. v.IsEdit is raised only
// for the EditedMessage wrapper (UnwrapRaw), and the ProtocolMessage type enum is
// worse than useless when absent: MESSAGE_EDIT is 14, so a missing type reads back
// as REVOKE - and neither form is even what a live edit arrives in today.
func isMessageEdit(v *events.Message) bool {
	if v.Info.Edit == types.EditAttributeMessageEdit {
		return true
	}
	if v.Info.Edit != types.EditAttributeEmpty {
		return false
	}

	// No edit attribute at all: history sync builds the event without one, so fall
	// back to the payload - but only when it actually carries replacement content,
	// which a revoke never does.
	return v.Message.GetProtocolMessage().GetEditedMessage() != nil ||
		v.Message.GetSecretEncryptedMessage().GetSecretEncType() == waE2E.SecretEncryptedMessage_MESSAGE_EDIT
}

// unwrapEdit rewrites an edited message into the message it became, in place.
//
// An edit arrives under the id of the edit stanza rather than of the message
// everyone can see in the chat, and its payload is wrapped in something that is
// not the new text. Left as is, utils.GetText finds nothing at all and a reaction
// would be attached to an id nobody's client displays.
//
// Today's WhatsApp nests two envelopes, and both have to come off:
//
//   - a SecretEncryptedMessage, sealed with the message secret of the message
//     being edited. Only that message's recipients hold the secret, so nothing
//     downstream can read a word of the edit until this is opened;
//   - inside it, a ProtocolMessage of type MESSAGE_EDIT, whose editedMessage is
//     the new content and whose key names the message being replaced. whatsmeow
//     peels this one itself on the history-sync path (ParseWebMessage) but never
//     on the live one - and an edit that arrives that way has no outer envelope,
//     which is why each layer is optional rather than assumed.
//
// Doing it here, at the single point where messages enter, means every handler
// downstream - marking as read, reacting, replying, reading the text - keeps
// working without knowing edits exist. It is also why this runs inside
// processIncomingMessage rather than in the event dispatch: opening the envelope
// reads the stored secret, and that must never block whatsmeow's dispatch loop.
func (b *Bot) unwrapEdit(v *events.Message) {
	if !isMessageEdit(v) {
		return
	}

	if sealed := v.Message.GetSecretEncryptedMessage(); sealed != nil {
		opened, err := b.currentSession().Client.DecryptSecretEncryptedMessage(context.Background(), v)
		if err != nil {
			// Nothing downstream can do with an edit that stays sealed, and
			// answering the wrong thing would be worse than answering nothing.
			b.Logger.Errorf("edicao: failed to open the edit of message %s: %v",
				sealed.GetTargetMessageKey().GetID(), err)
			return
		}
		if original := sealed.GetTargetMessageKey().GetID(); original != "" {
			v.Info.ID = original
		}
		v.Message = opened
	}

	// Unwrapped whenever the content is there, never gated on the type enum.
	if protocol := v.Message.GetProtocolMessage(); protocol != nil {
		if original := protocol.GetKey().GetID(); original != "" {
			v.Info.ID = original
		}
		if edited := protocol.GetEditedMessage(); edited != nil {
			v.Message = edited
		}
	}

	// Raised here rather than read here, so processIncomingMessage keeps routing
	// on one flag whichever form the edit came in.
	v.IsEdit = true
}
