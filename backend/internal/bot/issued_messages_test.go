package bot

import (
	"fmt"
	"testing"
)

func TestIssuedMessages_RemembersWhatAMessageCreated(t *testing.T) {
	var issued issuedMessages
	issued.remember("MSG1", issuedMessage{IssuanceID: "issuance-1", ProgramLocator: "IWBEQP"})

	entry, found := issued.lookup("MSG1")
	if !found {
		t.Fatal("the message was not remembered")
	}
	if entry.IssuanceID != "issuance-1" || entry.ProgramLocator != "IWBEQP" {
		t.Fatalf("remembered %+v, want the issuance and locator that were stored", entry)
	}

	if _, found := issued.lookup("NEVER-SENT"); found {
		t.Fatal("a message that was never remembered came back as known")
	}
}

// An empty id is what a test double or a malformed event hands over; storing it
// would make every other such message look like the same one.
func TestIssuedMessages_IgnoresAnEmptyID(t *testing.T) {
	var issued issuedMessages
	issued.remember("", issuedMessage{IssuanceID: "issuance-1"})

	if _, found := issued.lookup(""); found {
		t.Fatal("an empty message id was stored")
	}
}

func TestIssuedMessages_EvictsTheOldestPastCapacity(t *testing.T) {
	var issued issuedMessages

	for i := range issuedMessagesCapacity + 10 {
		issued.remember(fmt.Sprintf("MSG%d", i), issuedMessage{IssuanceID: fmt.Sprintf("issuance-%d", i)})
	}

	if _, found := issued.lookup("MSG0"); found {
		t.Fatal("the oldest entry survived past the capacity, so the map grows without bound")
	}
	newest := fmt.Sprintf("MSG%d", issuedMessagesCapacity+9)
	if _, found := issued.lookup(newest); !found {
		t.Fatalf("the newest entry %s was evicted instead of the oldest", newest)
	}
	if len(issued.by) != issuedMessagesCapacity {
		t.Fatalf("holding %d entries, want the capacity %d", len(issued.by), issuedMessagesCapacity)
	}
}

// Re-remembering the same id must overwrite rather than queue a second eviction
// slot, or the map evicts live entries long before it is actually full.
func TestIssuedMessages_RememberingTwiceDoesNotGrow(t *testing.T) {
	var issued issuedMessages

	issued.remember("MSG1", issuedMessage{IssuanceID: "issuance-1"})
	issued.remember("MSG1", issuedMessage{IssuanceID: "issuance-2"})

	if len(issued.order) != 1 {
		t.Fatalf("eviction order holds %d entries for one message id", len(issued.order))
	}
	entry, _ := issued.lookup("MSG1")
	if entry.IssuanceID != "issuance-2" {
		t.Fatalf("IssuanceID = %q, want the later value", entry.IssuanceID)
	}
}
