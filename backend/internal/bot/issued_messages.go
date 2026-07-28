package bot

import "sync"

// issuedMessagesCapacity caps how many EMISSAO messages are remembered at once.
// The map exists to catch an edit of a message someone sent moments ago; a few
// hundred entries covers a working day in a busy group, and the fallback below
// covers everything older.
const issuedMessagesCapacity = 500

// issuedMessage is what a remembered EMISSAO produced.
type issuedMessage struct {
	IssuanceID     string
	ProgramLocator string
}

// issuedMessages maps a WhatsApp message id to the issuance that message
// created. It is what lets an edit be recognised as a correction of a specific
// issuance instead of an ordinary message that happens to carry a locator.
//
// Without it the rule "the PNR of a saved issuance can never change" cannot
// exist: an edit that rewrites the locator is indistinguishable from one that
// names a locator nobody ever issued, and the bot would answer "issuance not
// found" for what is really a forbidden change.
//
// It deliberately does not persist. An edit of a message old enough to have
// been evicted - or sent before the last restart - still works, it just falls
// back to looking the locator up in the database and loses the ability to tell
// those two cases apart. That is a worse error message, not a wrong write.
type issuedMessages struct {
	mu sync.Mutex
	by map[string]issuedMessage
	// order records insertion order so the oldest entry is the one evicted.
	// A slice rather than a proper LRU because entries are only ever added and
	// evicted, never touched again on read.
	order []string
}

func (m *issuedMessages) remember(messageID string, entry issuedMessage) {
	if messageID == "" {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.by == nil {
		m.by = make(map[string]issuedMessage, issuedMessagesCapacity)
	}
	if _, exists := m.by[messageID]; !exists {
		m.order = append(m.order, messageID)
	}
	m.by[messageID] = entry

	for len(m.order) > issuedMessagesCapacity {
		delete(m.by, m.order[0])
		m.order = m.order[1:]
	}
}

func (m *issuedMessages) lookup(messageID string) (issuedMessage, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, found := m.by[messageID]
	return entry, found
}
