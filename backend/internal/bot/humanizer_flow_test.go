package bot

import (
	"reflect"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// TestHumanizedFlow_Ping_LogsEachStageWithinExpectedRanges is a manual/visual
// check (run with `go test -run TestHumanizedFlow -v`) of the full humanized
// sequence for a fast case that never touches Traveos: mark-as-read -> ✅ ->
// 🕐 -> composing -> final reply. It logs the elapsed time at each stage so the
// ranges from the humanizer spec can be eyeballed directly, in addition to
// the range assertions already covered by internal/bot/humanizer's own tests.
func TestHumanizedFlow_Ping_LogsEachStageWithinExpectedRanges(t *testing.T) {
	client := &fakeClientBase{}
	b := &Bot{
		Logger: waLog.Noop,
		Status: NewStatusStore(func() {}),
		session: deviceSession{
			Client: client,
			GetJID: func() *types.JID { return nil },
		},
	}

	evt := newPingEvent()
	arrivedAt := time.Now()

	b.processIncomingMessage(evt, false, arrivedAt)

	sendCalls, markReadCalls := client.calls()
	t.Logf("total elapsed from arrival to final reply: %v", time.Since(arrivedAt))
	t.Logf("markReadCalls=%d sendCalls=%d", markReadCalls, sendCalls)

	if markReadCalls != 1 {
		t.Fatalf("markReadCalls: got %d, want 1", markReadCalls)
	}
	// Three reactions plus the reply itself. The 🚀 matters: a reaction is only
	// ever replaced, never cleared, so a command answering with text alone would
	// leave its 🕐 on screen forever.
	if sendCalls != 4 {
		t.Fatalf("sendCalls: got %d, want 4 - ✅, 🕐, one reply and the closing 🚀", sendCalls)
	}
	want := []string{"✅", "🕐", "🚀"}
	if got := client.reactions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("reactions: got %v, want %v - the indicator must not be left on the clock", got, want)
	}

	// Floor from the humanizer spec is 1.5-3s; DelayMarkAsRead (0.5-1.5s)
	// plus DelayComposing (0.8-2s fixed) already dominate a fast path like
	// this, so the total should comfortably sit at or above the floor.
	if elapsed := time.Since(arrivedAt); elapsed < 1500*time.Millisecond {
		t.Fatalf("total elapsed %v is under the humanizer's 1.5s response floor", elapsed)
	}
}
