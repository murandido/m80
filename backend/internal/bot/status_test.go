package bot

import "testing"

func TestStatusStore_GetSet(t *testing.T) {
	store := NewStatusStore(func() {})

	initial := store.Get()
	if initial.State != StateDisconnected {
		t.Fatalf("initial state: got %q, want %q", initial.State, StateDisconnected)
	}

	store.Set(Status{State: StateConnected, JID: "123@s.whatsapp.net"})

	got := store.Get()
	if got.State != StateConnected || got.JID != "123@s.whatsapp.net" {
		t.Fatalf("after Set: got %+v", got)
	}
	if got.UpdatedAt.IsZero() {
		t.Fatal("UpdatedAt was not set")
	}
}

func TestStatusStore_NotifiesOnSet(t *testing.T) {
	calls := 0
	store := NewStatusStore(func() { calls++ })

	store.Set(Status{State: StatePairing})
	store.Set(Status{State: StateConnected})

	if calls != 2 {
		t.Fatalf("notify calls: got %d, want 2", calls)
	}
}
