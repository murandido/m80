package bot

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
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

	bot := &Bot{
		Logger:                waLog.Noop,
		Status:                NewStatusStore(func() {}),
		DB:                    tx,
		notifyChatRequested:   func() {},
		notifyIssuanceCreated: func() { notify.Add(1) },
		notifyIssuanceUpdated: func() { updated.Add(1) },
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

func TestHandleEmissao_PersistsTheWholeIssuance(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		s := newEmissaoScenario(t, tx, data.Airline{})

		s.Bot.handleEmissao(s.event(), s.text(), time.Now())

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

		s.Bot.handleEmissao(evt, s.text(), time.Now())

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

				s.Bot.handleEmissao(s.event(), s.text(), time.Now())

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

		s.Bot.handleEmissao(evt, s.text(), time.Now())

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

		s.Bot.handleEmissao(s.event(), s.text("CODIGOQUENAOEXISTE"), time.Now())

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
		s.Bot.handleEmissao(s.event(), wrong, time.Now())

		assertNoIssuance(t, tx, "PL"+s.Suffix)
		if reactions := s.Client.reactions(); len(reactions) != 1 || reactions[0] != "❌" {
			t.Fatalf("reactions = %v, want a single ❌", reactions)
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
