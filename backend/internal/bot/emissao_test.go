package bot

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// emissaoScenario is everything handleEmissao needs to get as far as writing:
// an approved chat, a known sender, and the FK chain behind the message.
type emissaoScenario struct {
	emissaoFixture

	Bot     *Bot
	Client  *fakeClientBase
	User    data.User
	ChatJID string
	Notify  *atomic.Int32
	// Updated counts the issuance.updated bridge calls, which only a correction
	// makes - see alteracao_test.go.
	Updated *atomic.Int32
	// Cancelled and Deleted count the issuance.cancelled/issuance.deleted bridge
	// calls, which only CANCELAMENTO/DELETAR make - see cancelamento_test.go and
	// deletar_test.go.
	Cancelled *atomic.Int32
	Deleted   *atomic.Int32
}

func newEmissaoScenario(t *testing.T, tx *gorm.DB, airline data.Airline) emissaoScenario {
	t.Helper()

	fixture := seedEmissaoFixture(t, tx, airline)
	user := createUser(t, tx, "")
	chatJID := "1203630000" + fixture.Suffix[:4] + "@g.us"

	seed(t, tx, &data.ListenedChat{
		ChatJID:       chatJID,
		RequestedByID: user.ID,
		Status:        "approved",
		Active:        true,
	})

	// An agency flagged is_default has to exist: the message carries no agency,
	// so the issuance service falls back to it.
	seedDefaultAgency(t, tx)

	client := &fakeClientBase{}
	notify := &atomic.Int32{}
	updated := &atomic.Int32{}
	cancelled := &atomic.Int32{}
	deleted := &atomic.Int32{}

	bot := &Bot{
		Logger:                  waLog.Noop,
		Status:                  NewStatusStore(func() {}),
		DB:                      tx,
		notifyChatRequested:     func() {},
		notifyIssuanceCreated:   func() { notify.Add(1) },
		notifyIssuanceUpdated:   func() { updated.Add(1) },
		notifyIssuanceCancelled: func() { cancelled.Add(1) },
		notifyIssuanceDeleted:   func() { deleted.Add(1) },
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}

	return emissaoScenario{
		emissaoFixture: fixture,
		Bot:            bot,
		Client:         client,
		User:           user,
		ChatJID:        chatJID,
		Notify:         notify,
		Updated:        updated,
		Cancelled:      cancelled,
		Deleted:        deleted,
	}
}

// seedDefaultAgency makes sure exactly one agency is flagged is_default inside
// this test's transaction, whether or not the database already had one.
func seedDefaultAgency(t *testing.T, tx *gorm.DB) {
	t.Helper()

	var count int64
	if err := tx.Model(&data.Agency{}).Where("is_default = ?", true).Count(&count).Error; err != nil {
		t.Fatalf("failed to count default agencies: %v", err)
	}
	if count > 0 {
		return
	}
	seed(t, tx, &data.Agency{Name: "Agência Padrão", IsDefault: true})
}

// text builds a full EMISSAO message for the scenario's loyalty program. The
// numbers are the SMILES example's, so the checksum holds.
func (s emissaoScenario) text(extraLines ...string) string {
	lines := []string{
		fmt.Sprintf("EMISSAO - %s - %s", s.Program.Code, "PL"+s.Suffix),
		"CNF",
		"31k * 16,5 + 189,22 tx = 700,72",
		"Heraldo" + s.Suffix,
	}
	return strings.Join(append(lines, extraLines...), "\n")
}

func (s emissaoScenario) event() *events.Message {
	return s.eventWithID("test-emissao-id")
}

// eventWithID is the same message from the same sender under a chosen id, which
// the edit tests need: an edit is recognised by the id of the message it
// replaced.
func (s emissaoScenario) eventWithID(id string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:           types.NewJID(strings.TrimSuffix(s.ChatJID, "@g.us"), types.GroupServer),
				Sender:         types.NewJID("55"+s.User.Phone, types.DefaultUserServer),
				AddressingMode: types.AddressingModePN,
			},
			ID: types.MessageID(id),
		},
	}
}

// messageEvent is the same event carrying its text as an actual WhatsApp
// payload, which the handlers never need (they are handed the text) but the
// full pipeline does - admit reads it off the message itself.
func (s emissaoScenario) messageEvent(text string) *events.Message {
	evt := s.event()
	evt.Message = &waE2E.Message{Conversation: proto.String(text)}
	return evt
}

func TestHandleEmissao_PersistsTheWholeIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("expected the issuance to have been created: %v", err)
		}

		if issuance.Points != 31000 {
			t.Fatalf("points = %d, want 31000", issuance.Points)
		}
		if issuance.ValueOfTheThousand != 1650 {
			t.Fatalf("value_of_the_thousand = %d, want 1650", issuance.ValueOfTheThousand)
		}
		if issuance.Taxes == nil || *issuance.Taxes != 18922 {
			t.Fatalf("taxes = %v, want 18922", issuance.Taxes)
		}
		if issuance.Complete {
			t.Fatal("an issuance from WhatsApp must be born incomplete, like a grid draft")
		}
		if issuance.IssuerID != s.User.ID {
			t.Fatalf("issuer_id = %q, want the sender %q", issuance.IssuerID, s.User.ID)
		}

		// One issuance, its open sale, its audit log - and nothing else. The
		// message carries no flight date and no first name, so neither a flight
		// leg nor a passenger can be built from it.
		assertIssuanceRowCount(t, tx, "sales", issuance.ID, 1)
		assertIssuanceRowCount(t, tx, "audit_logs", issuance.ID, 1)
		assertIssuanceRowCount(t, tx, "issuance_flights", issuance.ID, 0)
		assertIssuanceRowCount(t, tx, "issuance_passengers", issuance.ID, 0)

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "🚀" {
			t.Fatalf("reactions = %v, want a single 🚀", reactions)
		}

		// A success is a reaction and nothing else. Every message the bot sends
		// lands in a working group, so a confirmation line restating what the
		// reaction already said is noise - only refusals earn text. Counting the
		// sends is the only way to catch a reply creeping back in, since the
		// reaction itself also goes out as a send.
		if sends, _ := s.Client.calls(); sends != 1 {
			t.Fatalf("the bot sent %d message(s) on success, want 1 - the reaction alone", sends)
		}
		if got := s.Notify.Load(); got != 1 {
			t.Fatalf("notifyIssuanceCreated called %d time(s), want 1", got)
		}
	})
}

// The same sender addressed by LID instead of a phone number must land on the
// same user - whatsmeow puts the phone-bearing JID in SenderAlt in that case.
func TestHandleEmissao_ResolvesSenderAddressedByLID(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		evt := s.event()
		evt.Info.AddressingMode = types.AddressingModeLID
		evt.Info.SenderAlt = evt.Info.Sender
		evt.Info.Sender = types.NewJID("123456789012345", types.HiddenUserServer)

		s.Bot.handleEmissao(context.Background(), evt, s.text(), time.Now())

		var issuance data.Issuance
		if err := tx.Where("program_locator = ?", "PL"+s.Suffix).First(&issuance).Error; err != nil {
			t.Fatalf("expected the issuance to have been created: %v", err)
		}
		if issuance.IssuerID != s.User.ID {
			t.Fatalf("issuer_id = %q, want the sender %q", issuance.IssuerID, s.User.ID)
		}
	})
}

func TestHandleEmissao_IgnoresChatsThatWereNotApproved(t *testing.T) {
	statuses := []struct {
		name   string
		status string
		active bool
	}{
		{"pending", "pending", false},
		{"denied", "denied", false},
		{"approved but stopped", "approved", false},
	}

	for _, tt := range statuses {
		t.Run(tt.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				s := newEmissaoScenario(t, tx, data.Airline{})

				if err := tx.Model(&data.ListenedChat{}).
					Where("chat_jid = ?", s.ChatJID).
					Updates(map[string]any{"status": tt.status, "active": tt.active}).Error; err != nil {
					t.Fatalf("failed to change the chat status: %v", err)
				}

				s.Bot.handleEmissao(context.Background(), s.event(), s.text(), time.Now())

				assertNoIssuance(t, tx, "PL"+s.Suffix)
				if len(s.Client.reactions()) != 0 {
					t.Fatal("a chat the bot may not act on must get no visible response at all")
				}
			})
		})
	}
}

func TestHandleEmissao_IgnoresUnknownSenders(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		evt := s.event()
		evt.Info.Sender = types.NewJID("5511999998888", types.DefaultUserServer)

		s.Bot.handleEmissao(context.Background(), evt, s.text(), time.Now())

		assertNoIssuance(t, tx, "PL"+s.Suffix)
		if len(s.Client.reactions()) != 0 {
			t.Fatal("an unknown sender must get no visible response at all")
		}
	})
}

// A message that fails halfway must leave nothing behind - not even the account
// its nickname would have created.
func TestHandleEmissao_RollsBackAutoCreatedRecords(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.handleEmissao(context.Background(), s.event(), s.text("CODIGOQUENAOEXISTE"), time.Now())

		assertNoIssuance(t, tx, "PL"+s.Suffix)

		var nicknames int64
		if err := tx.Model(&data.AccountNickname{}).
			Where("nickname = ?", "Heraldo"+s.Suffix).Count(&nicknames).Error; err != nil {
			t.Fatalf("failed to count account_nicknames: %v", err)
		}
		if nicknames != 0 {
			t.Fatal("the account conjured from the nickname survived a rolled-back message")
		}

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌", reactions)
		}
	})
}

func TestHandleEmissao_RejectsAMessageThatDoesNotAddUp(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		wrong := strings.Replace(s.text(), "= 700,72", "= 999,99", 1)
		s.Bot.handleEmissao(context.Background(), s.event(), wrong, time.Now())

		assertNoIssuance(t, tx, "PL"+s.Suffix)
		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌", reactions)
		}
	})
}

// Two operators issuing the same PNR is a mistake whichever way it arrives, and
// the bot has two independent guards for it. This is the one that catches the
// second message *after* the first has landed - within a chat that is every
// repeat, since the worker runs one job at a time and the second one therefore
// always sees the first already committed. The partial unique index on
// program_locator is what actually refuses it.
func TestHandleEmissao_ARepeatedLocatorIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		s.Bot.handleEmissao(context.Background(), s.eventWithID("first-id"), s.text(), time.Now())
		if got := s.Notify.Load(); got != 1 {
			t.Fatalf("the first EMISSAO should have been registered, notify=%d", got)
		}
		s.Client.reset()
		s.Notify.Store(0)

		s.Bot.handleEmissao(context.Background(), s.eventWithID("second-id"), s.text(), time.Now())

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌ on the repeat", reactions)
		}
		if text := s.Client.lastText(); !strings.Contains(text, "Já existe uma emissão ativa") ||
			!strings.Contains(text, locator) {
			t.Fatalf("the refusal %q should name the locator that is already taken", text)
		}
		if got := s.Notify.Load(); got != 0 {
			t.Fatalf("notifyIssuanceCreated called %d time(s) for the repeat, want 0", got)
		}

		var rows int64
		if err := tx.Model(&data.Issuance{}).Where("program_locator = ?", locator).Count(&rows).Error; err != nil {
			t.Fatalf("failed to count issuances: %v", err)
		}
		if rows != 1 {
			t.Fatalf("issuances for %s: got %d, want 1", locator, rows)
		}
	})
}

// The other guard, for the case the index cannot catch: the same PNR running in
// two chats at the same moment, where neither transaction has committed yet and
// so neither sees the other. Chats run in parallel by design, so this is the only
// thing standing between them.
//
// The holder stands in for the job in the other chat. Its identity is all the
// lock cares about - what is being asserted is that a second job cannot take a
// locator someone else is executing, and that the message is refused rather than
// left to wait and become a duplicate a few seconds later.
func TestExecuteJob_ALocatorHeldByAnotherChatIsRefused(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})
		locator := "PL" + s.Suffix

		holder := &job{}
		if !s.Bot.jobs().acquire(holder, locator) {
			t.Fatal("the first job should have taken the locator")
		}

		second := s.Bot.admit(queuedMessage{
			evt:       s.messageEvent(s.text()),
			startedAt: time.Now(),
		})
		<-second.acked
		s.Client.reset()

		s.Bot.executeJob(second)

		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌ while the other chat holds the locator", reactions)
		}
		if text := s.Client.lastText(); !strings.Contains(text, "já está sendo processada") ||
			!strings.Contains(text, locator) {
			t.Fatalf("the refusal %q should say the locator is already being processed", text)
		}
		assertNoIssuance(t, tx, locator)
		if got := s.Notify.Load(); got != 0 {
			t.Fatalf("notifyIssuanceCreated called %d time(s), want 0 - nothing was written", got)
		}
	})
}

func assertNoIssuance(t *testing.T, tx *gorm.DB, locator string) {
	t.Helper()

	var count int64
	if err := tx.Model(&data.Issuance{}).Where("program_locator = ?", locator).Count(&count).Error; err != nil {
		t.Fatalf("failed to count issuances: %v", err)
	}
	if count != 0 {
		t.Fatalf("found %d issuance(s) for %s, want none", count, locator)
	}
}

func assertIssuanceRowCount(t *testing.T, tx *gorm.DB, table, issuanceID string, want int64) {
	t.Helper()

	var got int64
	if err := tx.Table(table).Where("issuance_id = ?", issuanceID).Count(&got).Error; err != nil {
		t.Fatalf("failed to count %s: %v", table, err)
	}
	if got != want {
		t.Fatalf("%s count = %d, want %d", table, got, want)
	}
}

// The whole pipeline on a real EMISSAO, reaction by reaction: ✅ the moment the
// gate lets the message through, 🕐 while it waits its turn, 🚀 once the
// transaction committed. Driven inline rather than through the dispatcher
// because these tests share a single rolled-back transaction, which two
// goroutines cannot use at once.
func TestProcessIncomingMessage_Emissao_ReactsCheckClockRocket(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.processIncomingMessage(s.messageEvent(s.text()), false, time.Now())

		want := []string{"✅", "🕐", "🚀"}
		if got := s.Client.reactions(); !reflect.DeepEqual(got, want) {
			t.Fatalf("reactions: got %v, want %v", got, want)
		}
		if text := s.Client.lastText(); text != "" {
			t.Fatalf("text: got %q, want none - a success is the reaction and nothing else", text)
		}
		if got := s.Notify.Load(); got != 1 {
			t.Fatalf("notifyIssuanceCreated called %d time(s), want 1", got)
		}
	})
}

// A refusal ends the same indicator on ❌, and only then adds the line saying
// which of the numbers the bot could not read.
func TestProcessIncomingMessage_Emissao_ReactsCheckClockCross(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		lines := strings.Split(s.text(), "\n")
		lines[2] = "31k * 16,5 + 189,22 tx = 999,99" // checksum that cannot hold

		s.Bot.processIncomingMessage(s.messageEvent(strings.Join(lines, "\n")), false, time.Now())

		want := []string{"✅", "🕐", "❌"}
		if got := s.Client.reactions(); !reflect.DeepEqual(got, want) {
			t.Fatalf("reactions: got %v, want %v", got, want)
		}
		if text := s.Client.lastText(); !strings.Contains(text, "não fecham") {
			t.Fatalf("text: got %q, want it to say the arithmetic does not add up", text)
		}
		if got := s.Notify.Load(); got != 0 {
			t.Fatalf("notifyIssuanceCreated called %d time(s), want 0", got)
		}
	})
}

// The gate stays silent, and that includes the acknowledgement. Someone typing
// in a group the bot merely happens to be in must not learn that it reads.
func TestProcessIncomingMessage_Emissao_UnapprovedChatGetsNoReactionAtAll(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		evt := s.messageEvent(s.text())
		evt.Info.Chat = types.NewJID("999000111", types.GroupServer)

		s.Bot.processIncomingMessage(evt, false, time.Now())

		if got := s.Client.reactions(); len(got) != 0 {
			t.Fatalf("reactions: got %v, want none - the gate is silent, ✅ included", got)
		}
	})
}
