package bot

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

func TestEventHandler_OfflineSyncPreview_SetsSyncingFlag(t *testing.T) {
	b := newRegistrarChatBot(nil, &fakeClientBase{})

	if b.offlineSyncing.Load() {
		t.Fatal("offlineSyncing should start false")
	}

	b.EventHandler(&events.OfflineSyncPreview{Total: 3, Messages: 3})

	if !b.offlineSyncing.Load() {
		t.Fatal("offlineSyncing should be true after a preview with Total > 0")
	}
}

func TestEventHandler_OfflineSyncPreview_ZeroTotalDoesNotSetFlag(t *testing.T) {
	b := newRegistrarChatBot(nil, &fakeClientBase{})

	b.EventHandler(&events.OfflineSyncPreview{Total: 0})

	if b.offlineSyncing.Load() {
		t.Fatal("offlineSyncing should stay false when there's nothing to sync")
	}
}

func TestEventHandler_OfflineSyncCompleted_ClearsSyncingFlag(t *testing.T) {
	b := newRegistrarChatBot(nil, &fakeClientBase{})
	b.offlineSyncing.Store(true)

	b.EventHandler(&events.OfflineSyncCompleted{Count: 3})

	if b.offlineSyncing.Load() {
		t.Fatal("offlineSyncing should be false after OfflineSyncCompleted")
	}
}

func newPingEvent() *events.Message {
	return &events.Message{
		Info: types.MessageInfo{
			MessageSource: types.MessageSource{
				Chat:   types.NewJID("11999999999", types.DefaultUserServer),
				Sender: types.NewJID("11999999999", types.DefaultUserServer),
			},
			ID: types.MessageID("test-ping-id"),
		},
		Message: &waE2E.Message{Conversation: proto.String("!ping")},
	}
}

func TestProcessIncomingMessage_DuringBacklog_MarksReadButDoesNotRouteCommand(t *testing.T) {
	client := &fakeClientBase{}
	b := &Bot{
		Logger: waLog.Noop,
		Status: NewStatusStore(func() {}),
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}

	b.processIncomingMessage(newPingEvent(), true)

	sendCalls, markReadCalls := client.calls()
	if markReadCalls != 1 {
		t.Fatalf("markReadCalls: got %d, want 1 - a backlog message must still be marked read", markReadCalls)
	}
	if sendCalls != 0 {
		t.Fatalf("sendCalls: got %d, want 0 - a backlog message must never be routed as a command", sendCalls)
	}
}

func TestProcessIncomingMessage_NotBacklog_RoutesCommandNormally(t *testing.T) {
	client := &fakeClientBase{}
	b := &Bot{
		Logger: waLog.Noop,
		Status: NewStatusStore(func() {}),
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}

	b.processIncomingMessage(newPingEvent(), false)

	sendCalls, markReadCalls := client.calls()
	if markReadCalls != 1 {
		t.Fatalf("markReadCalls: got %d, want 1", markReadCalls)
	}
	if sendCalls == 0 {
		t.Fatal("sendCalls: got 0, want at least 1 - !ping should have replied")
	}
}
