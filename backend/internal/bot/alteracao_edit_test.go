package bot

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/utils"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// editOf builds the event WhatsApp actually delivers for a live edit: two nested
// envelopes, under the id of the edit stanza rather than of the message being
// replaced.
//
// client is primed to reveal the inner envelope, so the test exercises the unwrap
// rather than the crypto. IsEdit stays false on purpose - nothing upstream raises
// it for this form, and recognising the edit anyway is what unwrapEdit is being
// tested on.
func editOf(client *fakeClientBase, base *events.Message, originalID, newText string) *events.Message {
	edited := *base
	edited.Info.ID = "edit-stanza-id"
	edited.Info.Edit = types.EditAttributeMessageEdit
	edited.Message = &waE2E.Message{
		SecretEncryptedMessage: &waE2E.SecretEncryptedMessage{
			TargetMessageKey: &waCommon.MessageKey{ID: proto.String(originalID)},
			SecretEncType:    waE2E.SecretEncryptedMessage_MESSAGE_EDIT.Enum(),
			EncPayload:       []byte("sealed"),
			EncIV:            []byte("iv"),
		},
	}
	client.primeOpenedEdit(originalID, newText)
	return &edited
}

// botWithClient is the smallest Bot that can drive unwrapEdit: it opens the
// envelope through the session's client, so there has to be one.
func botWithClient(client *fakeClientBase) *Bot {
	return &Bot{
		Logger:  waLog.Noop,
		Status:  NewStatusStore(func() {}),
		session: deviceSession{Client: client, GetJID: func() *types.JID { return nil }},
	}
}

func TestUnwrapEdit_ExposesTheNewTextUnderTheOriginalID(t *testing.T) {
	client := &fakeClientBase{}
	wrapped := editOf(client, &events.Message{}, "original-id", "EMISSAO - SMILES - IWBEQP")

	botWithClient(client).unwrapEdit(wrapped)

	if got := string(wrapped.Info.ID); got != "original-id" {
		t.Fatalf("Info.ID = %q, want the id of the message being edited", got)
	}
	// Read the way the router reads it: WhatsApp seals the new text as an
	// ExtendedTextMessage, not a Conversation.
	if got := utils.GetText(wrapped.Message); got != "EMISSAO - SMILES - IWBEQP" {
		t.Fatalf("text = %q, want the edited content", got)
	}
	// The flag the router downstream switches on. Nothing upstream raises it for
	// this form, so unwrapEdit raising it is the whole reason edits get routed at
	// all - see processIncomingMessage.
	if !wrapped.IsEdit {
		t.Fatal("IsEdit is false, want unwrapEdit to have recognised the edit on its own")
	}
}

// An edit that stays sealed must not be routed: handleMessageEdit would read an
// empty text and answer about a message nobody can see.
func TestUnwrapEdit_LeavesTheEditAloneWhenItCannotBeOpened(t *testing.T) {
	client := &fakeClientBase{openEditErr: errors.New("no message secret")}
	wrapped := editOf(client, &events.Message{}, "original-id", "EMISSAO - SMILES - IWBEQP")

	botWithClient(client).unwrapEdit(wrapped)

	if wrapped.IsEdit {
		t.Fatal("unwrapEdit routed an edit it could not open")
	}
	if string(wrapped.Info.ID) != "edit-stanza-id" {
		t.Fatalf("Info.ID = %q, want the event left untouched", wrapped.Info.ID)
	}
}

// The history-sync shape: no edit attribute on the stanza, no second envelope,
// and the payload names itself. Both forms have to work, so no single signal may
// be required on its own.
func TestUnwrapEdit_RecognisesAProtocolMessageEdit(t *testing.T) {
	wrapped := &events.Message{
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type:          waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
				Key:           &waCommon.MessageKey{ID: proto.String("original-id")},
				EditedMessage: &waE2E.Message{Conversation: proto.String("EMISSAO - SMILES - IWBEQP")},
			},
		},
	}
	wrapped.Info.ID = "edit-stanza-id"

	botWithClient(&fakeClientBase{}).unwrapEdit(wrapped)

	if !wrapped.IsEdit || string(wrapped.Info.ID) != "original-id" {
		t.Fatalf("IsEdit = %v, Info.ID = %q, want the edit recognised by its payload alone",
			wrapped.IsEdit, wrapped.Info.ID)
	}
}

// A deleted message rides the same ProtocolMessage shape as an edit and carries no
// replacement text. Treating it as a correction would hand handleMessageEdit an
// empty string; deleting an issuance is a command that does not exist yet.
func TestUnwrapEdit_LeavesADeletedMessageAlone(t *testing.T) {
	revoked := &events.Message{
		Message: &waE2E.Message{
			ProtocolMessage: &waE2E.ProtocolMessage{
				Type: waE2E.ProtocolMessage_REVOKE.Enum(),
				Key:  &waCommon.MessageKey{ID: proto.String("original-id")},
			},
		},
	}
	revoked.Info.ID = "revoke-stanza-id"
	revoked.Info.Edit = types.EditAttributeSenderRevoke

	botWithClient(&fakeClientBase{}).unwrapEdit(revoked)

	if revoked.IsEdit {
		t.Fatal("unwrapEdit read a deleted message as an edit")
	}
	if string(revoked.Info.ID) != "revoke-stanza-id" {
		t.Fatalf("Info.ID = %q, want the revoke stanza left untouched", revoked.Info.ID)
	}
}

// An ordinary message must come out of unwrapEdit exactly as it went in.
func TestUnwrapEdit_LeavesANormalMessageAlone(t *testing.T) {
	plain := &events.Message{Message: &waE2E.Message{Conversation: proto.String("bom dia")}}
	plain.Info.ID = "msg-id"

	botWithClient(&fakeClientBase{}).unwrapEdit(plain)

	if string(plain.Info.ID) != "msg-id" || plain.Message.GetConversation() != "bom dia" {
		t.Fatal("unwrapEdit rewrote a message that was never edited")
	}
	if plain.IsEdit {
		t.Fatal("unwrapEdit flagged an ordinary message as an edit")
	}
}

// The whole path, the way EventHandler drives it: unwrap, then route. An edited
// EMISSAO still reads as an EMISSAO, so only the edit flag distinguishes a
// correction from a second issuance.
func TestProcessIncomingMessage_RoutesAnEditedEmissaoAsACorrection(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		original := s.eventWithID("original-id")
		s.Bot.handleEmissao(context.Background(), original, s.text(), time.Now())
		s.Client.reset()

		corrected := strings.Replace(s.text(), "31k * 16,5 + 189,22 tx = 700,72",
			"50k * 33,00 + 100 tx = 1.750,00", 1)

		// Driven through processIncomingMessage, which is where the unwrap now
		// happens - opening the envelope reads the stored secret, so it cannot
		// sit on whatsmeow's dispatch goroutine.
		edit := editOf(s.Client, original, "original-id", corrected)
		s.Bot.processIncomingMessage(edit, false, time.Now())

		var issuances []data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).Find(&issuances).Error; err != nil {
			t.Fatalf("failed to read the issuances: %v", err)
		}
		if len(issuances) != 1 {
			t.Fatalf("found %d issuance(s), want the original one corrected in place", len(issuances))
		}
		if issuances[0].Points != 50000 {
			t.Fatalf("points = %d, want the edited 50000", issuances[0].Points)
		}
		if got := s.Updated.Load(); got != 1 {
			t.Fatalf("notifyIssuanceUpdated called %d time(s), want 1", got)
		}
	})
}

func TestHandleMessageEdit_RefusesANewLocator(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		// newCorrectionScenario sent the base EMISSAO under the default id, so
		// that is the message being edited.
		retyped := strings.Replace(s.text(), "PL"+s.Suffix, "OUTRO"+s.Suffix, 1)
		s.Bot.handleMessageEdit(context.Background(), s.event(), retyped, time.Now())

		assertRefused(t, s, "Não é possível alterar o PNR")

		var count int64
		if err := tx.Model(&data.Issuance{}).
			Where("program_locator = ?", "OUTRO"+s.Suffix).Count(&count).Error; err != nil {
			t.Fatalf("failed to count issuances: %v", err)
		}
		if count != 0 {
			t.Fatal("editing the locator created a second issuance instead of being refused")
		}
	})
}

// A message older than the map - sent before the last restart, or evicted since
// - still corrects the issuance its locator names. The only thing lost is the
// ability to recognise a rewritten locator, which the case above covers.
func TestHandleMessageEdit_FallsBackToTheLocatorWhenTheMapForgot(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.issued = issuedMessages{}

		corrected := strings.Replace(s.text(), "31k * 16,5 + 189,22 tx = 700,72",
			"50k * 33,00 + 100 tx = 1.750,00", 1)
		s.Bot.handleMessageEdit(context.Background(), s.eventWithID("forgotten-id"), corrected, time.Now())

		assertRocket(t, s)
		if points := s.reload(t, tx).Points; points != 50000 {
			t.Fatalf("points = %d, want the edited 50000", points)
		}
	})
}

// Editing a message the bot refused is how an operator fixes a typo that stopped
// the issuance from being registered at all. There is nothing to correct, so the
// edit registers it.
func TestHandleMessageEdit_RegistersAMessageTheBotHadRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		// The bot never saw this id succeed, and no issuance carries the locator.
		s.Bot.handleMessageEdit(context.Background(), s.eventWithID("refused-id"), s.text(), time.Now())

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("the corrected message was not registered: %v", err)
		}
		if got := s.Notify.Load(); got != 1 {
			t.Fatalf("notifyIssuanceCreated called %d time(s), want 1 - this is a creation", got)
		}
		if got := s.Updated.Load(); got != 0 {
			t.Fatalf("notifyIssuanceUpdated called %d time(s), want 0 - nothing was corrected", got)
		}
	})
}

// An edit restates the entire message, so a line the operator deleted is a
// deletion - unlike an ALTERACAO, where silence means "not mentioned".
func TestHandleMessageEdit_ClearsWhatTheEditRemoved(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		preset := data.ObservationPreset{Code: "ACORDO" + s.Suffix, Text: "Acordo comercial"}
		seed(t, tx, &preset)

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(preset.Code), time.Now())
		s.Client.reset()

		var before data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&before).Error; err != nil {
			t.Fatalf("the base EMISSAO did not persist: %v", err)
		}
		if before.Observation == nil {
			t.Fatal("the base message did not record the observation, so there is nothing to clear")
		}

		s.Bot.handleMessageEdit(context.Background(), s.event(), s.text(), time.Now())

		var after data.Issuance
		if err := tx.First(&after, "id = ?", before.ID).Error; err != nil {
			t.Fatalf("failed to reload the issuance: %v", err)
		}
		if after.Observation != nil {
			t.Fatalf("observation = %q, want it cleared along with the line that set it", *after.Observation)
		}
	})
}

// Editing ordinary conversation is as uninteresting as the original was.
func TestHandleMessageEdit_IgnoresOrdinaryText(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newCorrectionScenario(t, tx, data.Airline{})

		s.Bot.handleMessageEdit(context.Background(), s.event(), "quis dizer amanhã, não hoje", time.Now())

		assertSilence(t, s)
	})
}

// State 1 of the edit table: the message was edited while its job was still
// queued. The job carries the new content and keeps its place in line; the edit
// itself produces no second job, so the issuance is written once, from the
// corrected text.
func TestSupersede_EditOfAQueuedJobRewritesItInPlace(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		queued := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-queued.acked
		s.Client.reset()

		corrected := strings.Replace(s.text(), "31k * 16,5 + 189,22 tx = 700,72",
			"50k * 33,00 + 100 tx = 1.750,00", 1)
		edit := editOf(s.Client, s.messageEvent(corrected), "test-emissao-id", corrected)

		replacement := s.Bot.admit(queuedMessage{evt: edit, startedAt: time.Now()})
		<-replacement.acked

		if replacement.kind != kindNone {
			t.Fatalf("the edit produced a job of kind %v, want none - it rewrote the queued one", replacement.kind)
		}
		if got := s.Client.reactions(); !reflect.DeepEqual(got, []string{"✅", "🕐"}) {
			t.Fatalf("reactions: got %v, want [✅ 🕐] on the message being edited", got)
		}

		text, kind, gotLocator := queued.payload()
		if kind != kindEmissao || gotLocator != locator {
			t.Fatalf("queued job now: kind=%v locator=%q, want an EMISSAO for %s", kind, gotLocator, locator)
		}
		if !strings.Contains(text, "50k") {
			t.Fatalf("queued job text was not replaced: %q", text)
		}

		// Running it writes the corrected numbers, once.
		s.Bot.executeJob(queued)

		var issuances []data.Issuance
		if err := tx.Where("program_locator = ?", locator).Find(&issuances).Error; err != nil {
			t.Fatalf("failed to read the issuances: %v", err)
		}
		if len(issuances) != 1 {
			t.Fatalf("found %d issuance(s), want exactly one", len(issuances))
		}
		if issuances[0].Points != 50000 {
			t.Fatalf("points = %d, want the edited 50000 - the original content must never have run", issuances[0].Points)
		}
	})
}

// State 2: the job was already running. There is no place in line left to keep,
// so it is cancelled and the edit is handled as an ordinary correction - which
// registers the issuance when the rollback got there first, and corrects it when
// the commit did.
func TestSupersede_EditOfARunningJobCancelsIt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		running := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-running.acked
		if _, live := running.begin(context.Background()); !live {
			t.Fatal("the job should have begun")
		}

		if superseded := s.Bot.supersede(running.evt, s.text()); superseded {
			t.Fatal("an edit of a running job must not be swallowed - it has to run as a correction")
		}
		if !running.isCancelled() {
			t.Fatal("the running job should have been cancelled")
		}
	})
}

// State 3, unchanged: no live job behind the message, so the edit is a
// correction of what is saved. Guards the fall-through.
func TestSupersede_EditWithNoLiveJobIsLeftToTheCorrectionPath(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		if s.Bot.supersede(s.event(), s.text()) {
			t.Fatal("nothing is in flight, so there is nothing to supersede")
		}
	})
}
