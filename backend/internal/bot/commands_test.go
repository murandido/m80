package bot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The DB-backed tests below need a live database, same as the API handler
// tests (internal/api/v1/handlers/issuances_cancel_test.go) - this file
// mirrors that exact skip-if-unreachable convention so `go test ./...` stays
// green on a machine with no Postgres up.

var (
	testDBOnce sync.Once
	testDB     *gorm.DB
	testDBErr  error

	phoneCounter int64
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	testDBOnce.Do(func() {
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)

		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Silent),
		})
		if err != nil {
			testDBErr = err
			return
		}
		sqlDB, err := db.DB()
		if err != nil {
			testDBErr = err
			return
		}
		if err := sqlDB.Ping(); err != nil {
			testDBErr = err
			return
		}

		data.Connect()
		testDB = data.DB
	})

	if testDBErr != nil {
		t.Skipf("skipping registrar-chat tests: no test database available: %v", testDBErr)
	}
	return testDB
}

// runInTx runs fn inside a transaction that's always rolled back, so seeded
// rows never leak between tests.
func runInTx(t *testing.T, fn func(tx *gorm.DB)) {
	t.Helper()
	db := setupTestDB(t)

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("failed to begin test transaction: %v", tx.Error)
	}
	t.Cleanup(func() { tx.Rollback() })

	fn(tx)
}

// createUser seeds a user with a unique, valid Brazilian phone (User.BeforeCreate
// validates DDD + 9 digits), built from an atomic counter so seeds never collide.
func createUser(t *testing.T, tx *gorm.DB, phone string) data.User {
	t.Helper()
	if phone == "" {
		phone = fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
	}
	user := data.User{Name: "Tester", Phone: phone, Password: "irrelevant"}
	if err := tx.Create(&user).Error; err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	return user
}

// fakeClientBase is a minimal bot.ClientBase double. It records every
// reaction emoji and every reply text sent (both go out through SendMessage,
// since ReactMessage routes through BuildReaction + SendMessage) so tests can
// assert on them without caring about the exact whatsmeow protobuf plumbing.
type fakeClientBase struct {
	mu            sync.Mutex
	sentReactions []string
	sentTexts     []string
	sendCalls     int
	markReadCalls int

	// trace is every visible action in the order it happened, which is what the
	// queue tests assert on: whether two jobs of one chat interleaved, and which
	// message each reply was quoting. The other recorders above are per-kind and
	// cannot answer either question.
	trace []string

	// groupInfo/groupInfoErr configure GetGroupInfo's canned response, for
	// tests exercising the group-name resolution path.
	groupInfo    *types.GroupInfo
	groupInfoErr error

	// openedEdit/openEditErr configure what DecryptSecretEncryptedMessage hands
	// back, so the edit tests exercise the unwrap rather than real crypto.
	openedEdit  *waE2E.Message
	openEditErr error
}

func (f *fakeClientBase) SendMessage(_ context.Context, _ types.JID, message *waE2E.Message, _ ...whatsmeow.SendRequestExtra) (whatsmeow.SendResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sendCalls++
	if message.ReactionMessage != nil {
		f.sentReactions = append(f.sentReactions, message.ReactionMessage.GetText())
		// The target id rides along in the key so the trace can tell whose
		// progress indicator moved - three messages in flight at once produce
		// three interleaved ✅/🕐 pairs, and only the id separates them.
		f.trace = append(f.trace, "react:"+message.ReactionMessage.GetText()+":"+message.ReactionMessage.GetKey().GetID())
	}
	if text := message.GetExtendedTextMessage().GetText(); text != "" {
		f.sentTexts = append(f.sentTexts, text)
		// The quoted id is what makes replies to identical commands ("!ping")
		// tellable apart, which is the only way to assert queue ordering.
		f.trace = append(f.trace, "reply:"+message.GetExtendedTextMessage().GetContextInfo().GetStanzaID())
	}
	return whatsmeow.SendResponse{}, nil
}

func (f *fakeClientBase) SendChatPresence(_ context.Context, _ types.JID, _ types.ChatPresence, _ types.ChatPresenceMedia) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.trace = append(f.trace, "presence")
	return nil
}

func (f *fakeClientBase) MarkRead(_ context.Context, _ []types.MessageID, _ time.Time, _, _ types.JID, _ ...types.ReceiptType) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.markReadCalls++
	f.trace = append(f.trace, "read")
	return nil
}

// traceMatching returns the recorded actions whose prefix is one of want, in
// order - so a test can look at just the presence/reply rhythm of the worker
// without the read receipts the intake goroutines interleave into it.
func (f *fakeClientBase) traceMatching(want ...string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var kept []string
	for _, entry := range f.trace {
		for _, prefix := range want {
			if strings.HasPrefix(entry, prefix) {
				kept = append(kept, entry)
				break
			}
		}
	}
	return kept
}

func (f *fakeClientBase) calls() (sendCalls, markReadCalls int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sendCalls, f.markReadCalls
}

func (f *fakeClientBase) BuildReaction(_, _ types.JID, id types.MessageID, reaction string) *waE2E.Message {
	return &waE2E.Message{ReactionMessage: &waE2E.ReactionMessage{
		Text: proto.String(reaction),
		Key:  &waCommon.MessageKey{ID: proto.String(string(id))},
	}}
}

func (f *fakeClientBase) DecryptSecretEncryptedMessage(_ context.Context, _ *events.Message) (*waE2E.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.openEditErr != nil {
		return nil, f.openEditErr
	}
	return f.openedEdit, nil
}

// primeOpenedEdit sets what DecryptSecretEncryptedMessage will reveal: not the
// text itself, but the second envelope WhatsApp seals inside the first - a
// MESSAGE_EDIT protocol message naming the message being replaced.
func (f *fakeClientBase) primeOpenedEdit(originalID, text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.openedEdit = &waE2E.Message{
		ProtocolMessage: &waE2E.ProtocolMessage{
			Type: waE2E.ProtocolMessage_MESSAGE_EDIT.Enum(),
			Key:  &waCommon.MessageKey{ID: proto.String(originalID)},
			EditedMessage: &waE2E.Message{
				ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text)},
			},
		},
	}
}

func (f *fakeClientBase) GetGroupInfo(_ context.Context, _ types.JID) (*types.GroupInfo, error) {
	if f.groupInfoErr != nil {
		return nil, f.groupInfoErr
	}
	return f.groupInfo, nil
}

func (f *fakeClientBase) reactions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sentReactions...)
}

// lastText returns the most recent reply the bot sent, or "" if it only reacted.
func (f *fakeClientBase) lastText() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.sentTexts) == 0 {
		return ""
	}
	return f.sentTexts[len(f.sentTexts)-1]
}

func newRegistrarChatEvent(chatJID, senderPhone string) *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:           types.NewJID(chatJID, types.GroupServer),
				Sender:         types.NewJID(senderPhone, types.DefaultUserServer),
				AddressingMode: types.AddressingModePN,
			},
			ID: types.MessageID("test-msg-id"),
		},
	}
}

// newRegistrarChatDMEvent builds a DM-shaped event (chat == a direct contact,
// not a group), with PushName set - the display name resolution path for
// non-group chats reads straight off it.
func newRegistrarChatDMEvent(chatJID, senderPhone, pushName string) *events.Message {
	evt := newRegistrarChatEvent(chatJID, senderPhone)
	evt.Info.Chat = types.NewJID(chatJID, types.DefaultUserServer)
	evt.Info.PushName = pushName
	return evt
}

func newRegistrarChatBot(tx *gorm.DB, client *fakeClientBase) *Bot {
	return newRegistrarChatBotWithNotify(tx, client, func() {})
}

func newRegistrarChatBotWithNotify(tx *gorm.DB, client *fakeClientBase, notifyChatRequested func()) *Bot {
	return &Bot{
		Logger:              waLog.Noop,
		Status:              NewStatusStore(func() {}),
		DB:                  tx,
		notifyChatRequested: notifyChatRequested,
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}
}

func countListenedChats(t *testing.T, tx *gorm.DB, chatJID string) int64 {
	t.Helper()
	var count int64
	if err := tx.Model(&data.ListenedChat{}).Where("chat_jid = ?", chatJID).Count(&count).Error; err != nil {
		t.Fatalf("failed to count listened_chats: %v", err)
	}
	return count
}

func TestCmdRegistrarChat_KnownUser_CreatesPendingRowAndReactsOK(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		user := createUser(t, tx, phone)
		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)

		chatJID := "1111111111"
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("expected a listened_chats row to be created: %v", err)
		}
		if chat.RequestedByID != user.ID {
			t.Fatalf("requested_by_id: got %q, want %q", chat.RequestedByID, user.ID)
		}
		if chat.Status != "pending" {
			t.Fatalf("status: got %q, want %q", chat.Status, "pending")
		}
		if chat.Active {
			t.Fatal("active: got true, want false")
		}

		if got := client.reactions(); len(got) != 1 || got[0] != "🚀" {
			t.Fatalf("reactions: got %v, want [✅]", got)
		}
	})
}

func TestCmdRegistrarChat_UnknownUser_SilentlyIgnored(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)

		chatJID := "2222222222"
		evt := newRegistrarChatEvent(chatJID, "5511900000000")

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		if count := countListenedChats(t, tx, evt.Info.Chat.String()); count != 0 {
			t.Fatalf("listened_chats rows: got %d, want 0", count)
		}
		if got := client.reactions(); len(got) != 0 {
			t.Fatalf("reactions: got %v, want none - unknown senders are ignored silently", got)
		}
	})
}

func TestCmdRegistrarChat_AlreadyRequested_NoDuplicateSilentlyIgnored(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		user := createUser(t, tx, phone)
		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)

		chatJID := "3333333333"
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())
		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		if count := countListenedChats(t, tx, evt.Info.Chat.String()); count != 1 {
			t.Fatalf("listened_chats rows: got %d, want 1", count)
		}
		// The duplicate writes nothing but still answers 🚀. It used to answer
		// nothing at all; that stopped being possible once intake acknowledges
		// the message with ✅ and 🕐, which would otherwise never resolve.
		if got := client.reactions(); len(got) != 2 || got[0] != "🚀" || got[1] != "🚀" {
			t.Fatalf("reactions: got %v, want [🚀 🚀] - a duplicate is already-true, not a failure", got)
		}
		if text := client.lastText(); text != "" {
			t.Fatalf("text: got %q, want none - 🚀 says everything a duplicate needs", text)
		}
		_ = user
	})
}

// seedListenedChat creates a listened_chats row directly (bypassing the
// command), for tests that need to start from a specific pre-existing state
// (stopped, denied, actively monitored) rather than one !registrar-chat can
// produce on its own.
func seedListenedChat(t *testing.T, tx *gorm.DB, chatJID string, requestedBy data.User, status string, active bool) data.ListenedChat {
	t.Helper()
	chat := data.ListenedChat{
		ChatJID:       chatJID,
		RequestedByID: requestedBy.ID,
		Status:        status,
		Active:        active,
	}
	if err := tx.Create(&chat).Error; err != nil {
		t.Fatalf("failed to seed listened_chats row: %v", err)
	}
	return chat
}

func TestCmdRegistrarChat_StoppedChat_ReopensAsNewPendingRequest(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		originalPhone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		originalRequester := createUser(t, tx, originalPhone)
		newPhone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		newRequester := createUser(t, tx, newPhone)

		chatJID := "5555555555"
		seeded := seedListenedChat(t, tx, chatJID+"@g.us", originalRequester, "approved", false)

		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)
		evt := newRegistrarChatEvent(chatJID, "55"+newPhone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		if count := countListenedChats(t, tx, evt.Info.Chat.String()); count != 1 {
			t.Fatalf("listened_chats rows: got %d, want 1 (reopened, not duplicated)", count)
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", seeded.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.RequestedByID != newRequester.ID {
			t.Fatalf("requested_by_id: got %q, want %q (new requester)", reloaded.RequestedByID, newRequester.ID)
		}
		if reloaded.Status != "pending" {
			t.Fatalf("status: got %q, want %q", reloaded.Status, "pending")
		}
		if reloaded.Active {
			t.Fatal("active: got true, want false")
		}
		if reloaded.ApprovedByID != nil {
			t.Fatalf("approved_by_id: got %v, want nil", reloaded.ApprovedByID)
		}
		if got := client.reactions(); len(got) != 1 || got[0] != "🚀" {
			t.Fatalf("reactions: got %v, want [✅]", got)
		}
	})
}

func TestCmdRegistrarChat_DeniedChat_ReopensAsNewPendingRequest(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		requester := createUser(t, tx, phone)

		chatJID := "6666666666"
		seeded := seedListenedChat(t, tx, chatJID+"@g.us", requester, "denied", false)

		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		if count := countListenedChats(t, tx, evt.Info.Chat.String()); count != 1 {
			t.Fatalf("listened_chats rows: got %d, want 1 (reopened, not duplicated)", count)
		}

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", seeded.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "pending" {
			t.Fatalf("status: got %q, want %q", reloaded.Status, "pending")
		}
		if got := client.reactions(); len(got) != 1 || got[0] != "🚀" {
			t.Fatalf("reactions: got %v, want [✅]", got)
		}
	})
}

// An already-monitored chat is left exactly as it is. It answers 🚀 rather than
// nothing, for the same reason a duplicate pending request does: intake already
// promised an answer with ✅ and 🕐, and "the chat is already being monitored"
// is the state the sender asked for.
func TestCmdRegistrarChat_ActiveChat_ChangesNothingAndSaysItIsDone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		requester := createUser(t, tx, phone)

		chatJID := "7777777777"
		seeded := seedListenedChat(t, tx, chatJID+"@g.us", requester, "approved", true)

		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", seeded.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "approved" || !reloaded.Active {
			t.Fatalf("chat should be untouched: got status=%q active=%v", reloaded.Status, reloaded.Active)
		}
		if got := client.reactions(); len(got) != 1 || got[0] != "🚀" {
			t.Fatalf("reactions: got %v, want [🚀] - already monitored is already true", got)
		}
	})
}

// A permanently deactivated chat (DeactivateListenedChat) never reopens on
// its own - unlike a soft-stopped one (StopListenedChat), which still does
// (see TestCmdRegistrarChat_StoppedChat_ReopensAsNewPendingRequest above,
// unchanged). The refusal is spoken rather than silent: 🚀 would claim the
// chat is now pending approval when nothing was written at all.
func TestCmdRegistrarChat_DeactivatedChat_StaysDeactivatedAndSaysSo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		originalPhone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		originalRequester := createUser(t, tx, originalPhone)
		newPhone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		newRequester := createUser(t, tx, newPhone)

		chatJID := "8181818181"
		seeded := seedListenedChat(t, tx, chatJID+"@g.us", originalRequester, "deactivated", false)

		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)
		evt := newRegistrarChatEvent(chatJID, "55"+newPhone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var reloaded data.ListenedChat
		if err := tx.First(&reloaded, "id = ?", seeded.ID).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		if reloaded.Status != "deactivated" || reloaded.Active {
			t.Fatalf("chat should stay deactivated: got status=%q active=%v", reloaded.Status, reloaded.Active)
		}
		if reloaded.RequestedByID != originalRequester.ID {
			t.Fatalf("requested_by_id should be untouched: got %q, want %q", reloaded.RequestedByID, originalRequester.ID)
		}
		if got := client.reactions(); len(got) != 1 || got[0] != "❌" {
			t.Fatalf("reactions: got %v, want [❌] - a deactivated chat never reopens on its own", got)
		}
		if text := client.lastText(); !strings.Contains(text, "painel") {
			t.Fatalf("text: got %q, want it to point at the panel as the only way back", text)
		}
		_ = newRequester
	})
}

func TestCmdRegistrarChat_LIDAddressing_ResolvesViaSenderAlt(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		user := createUser(t, tx, phone)
		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)

		chatJID := "4444444444"
		evt := &events.Message{
			Info: types.MessageInfo{
				MessageSource: types.MessageSource{
					Chat:           types.NewJID(chatJID, types.GroupServer),
					Sender:         types.NewJID("999888777666555", types.HiddenUserServer),
					SenderAlt:      types.NewJID("55"+phone, types.DefaultUserServer),
					AddressingMode: types.AddressingModeLID,
				},
				ID: types.MessageID("test-msg-id-lid"),
			},
		}

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("expected a listened_chats row to be created: %v", err)
		}
		if chat.RequestedByID != user.ID {
			t.Fatalf("requested_by_id: got %q, want %q", chat.RequestedByID, user.ID)
		}
	})
}

func TestCmdRegistrarChat_GroupChat_FriendlyNameFromGetGroupInfo(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		createUser(t, tx, phone)
		client := &fakeClientBase{groupInfo: &types.GroupInfo{GroupName: types.GroupName{Name: "Grupo de Teste"}}}
		b := newRegistrarChatBot(tx, client)

		chatJID := "8888888888"
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("expected a listened_chats row to be created: %v", err)
		}
		if chat.FriendlyName == nil || *chat.FriendlyName != "Grupo de Teste" {
			t.Fatalf("friendly_name: got %v, want %q", chat.FriendlyName, "Grupo de Teste")
		}
	})
}

func TestCmdRegistrarChat_DMChat_FriendlyNameFromPushName(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		createUser(t, tx, phone)
		client := &fakeClientBase{}
		b := newRegistrarChatBot(tx, client)

		chatJID := "9999999999"
		evt := newRegistrarChatDMEvent(chatJID, "55"+phone, "Fulano de Tal")

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("expected a listened_chats row to be created: %v", err)
		}
		if chat.FriendlyName == nil || *chat.FriendlyName != "Fulano de Tal" {
			t.Fatalf("friendly_name: got %v, want %q", chat.FriendlyName, "Fulano de Tal")
		}
	})
}

func TestCmdRegistrarChat_GetGroupInfoFails_FriendlyNameStaysNilRowStillCreated(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		createUser(t, tx, phone)
		client := &fakeClientBase{groupInfoErr: errors.New("boom")}
		b := newRegistrarChatBot(tx, client)

		chatJID := "1010101010"
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		b.cmdRegistrarChat(context.Background(), evt, time.Now())

		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("expected a listened_chats row to be created despite the GetGroupInfo error: %v", err)
		}
		if chat.FriendlyName != nil {
			t.Fatalf("friendly_name: got %v, want nil", chat.FriendlyName)
		}
		if got := client.reactions(); len(got) != 1 || got[0] != "🚀" {
			t.Fatalf("reactions: got %v, want [✅] - a group-info failure must not block the command", got)
		}
	})
}

func TestCmdRegistrarChat_NotifiesOnlyOnCreateAndReopen(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		var notifyCalls int
		notify := func() { notifyCalls++ }

		phone := fmt.Sprintf("119%08d", atomic.AddInt64(&phoneCounter, 1)%100000000)
		createUser(t, tx, phone)
		client := &fakeClientBase{}
		b := newRegistrarChatBotWithNotify(tx, client, notify)

		chatJID := "1212121212"
		evt := newRegistrarChatEvent(chatJID, "55"+phone)

		// 1st call: new row created -> notifies.
		b.cmdRegistrarChat(context.Background(), evt, time.Now())
		if notifyCalls != 1 {
			t.Fatalf("notify calls after create: got %d, want 1", notifyCalls)
		}

		// 2nd call: still pending -> blocked, must NOT notify again.
		b.cmdRegistrarChat(context.Background(), evt, time.Now())
		if notifyCalls != 1 {
			t.Fatalf("notify calls after duplicate (pending): got %d, want 1", notifyCalls)
		}

		// Deny it directly, then request again: reopened -> notifies.
		var chat data.ListenedChat
		if err := tx.First(&chat, "chat_jid = ?", evt.Info.Chat.String()).Error; err != nil {
			t.Fatalf("reload chat: %v", err)
		}
		chat.Status = "denied"
		if err := tx.Save(&chat).Error; err != nil {
			t.Fatalf("failed to deny chat: %v", err)
		}

		b.cmdRegistrarChat(context.Background(), evt, time.Now())
		if notifyCalls != 2 {
			t.Fatalf("notify calls after reopen: got %d, want 2", notifyCalls)
		}

		// Unknown sender -> silently ignored, must NOT notify.
		unknownEvt := newRegistrarChatEvent("1313131313", "5511900000001")
		b.cmdRegistrarChat(context.Background(), unknownEvt, time.Now())
		if notifyCalls != 2 {
			t.Fatalf("notify calls after unknown sender: got %d, want 2", notifyCalls)
		}
	})
}
