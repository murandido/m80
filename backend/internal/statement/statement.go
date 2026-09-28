// Package statement reads the account statements the airlines let us export, and compares
// them with what the panel holds.
//
// It exists because there is no API for either programme. What there is: a CSV from Azul
// and a page of HTML from Smiles, both saved by hand. This package is the bridge until an
// API shows up, and is written to be deleted when one does - nothing outside it knows the
// shape of either file.
//
// Nothing here is stored. A statement is always re-exportable, and reconciling needs a diff
// now rather than a history, so the file is read, compared, acted on and dropped. That is
// also what makes the Azul CSV usable at all: it carries no transaction id and repeats
// identical lines legitimately (five -500 RLGHNM on one day are five real movements), so
// anything that tried to remember rows would have to choose between duplicating them and
// erasing four of the five.
package statement

import "time"

// Kind is what a movement does to the account, in the panel's terms rather than the
// airline's. Each programme spells its own operations differently - Azul writes "Resgate"
// and Smiles "Redemption" - and the reconciliation only cares about the three that matter.
type Kind string

const (
	// KindRedemption is points leaving the account for a booking. Reconciles against an
	// issuance by locator.
	KindRedemption Kind = "redemption"
	// KindRefund is points coming back on a booking that was cancelled. Reconciles
	// against the issuance's refunded points.
	KindRefund Kind = "refund"
	// KindAccrual is points arriving from anywhere else: a transfer partner, a bank, a
	// club, a flight. Reconciles against the point batches - and an accrual with no batch
	// is the finding that pays for this whole package, because those points are being
	// spent at whatever the neighbouring batch cost.
	KindAccrual Kind = "accrual"
	// KindExpiration is points the airline expired. It is counted in the statement's net
	// change (so a balance check still sees it) but deliberately falls through Reconcile's
	// switch: point expiration is kept OUT of the FIFO costing for now. It only ever comes
	// from the synced Azul statement; the CSV/HTML parsers never produced it.
	KindExpiration Kind = "expiration"
	// KindOther is any synced movement whose airline type code the mapping did not recognise.
	// Counted in the net so nothing is silently dropped, but not reconciled.
	KindOther Kind = "other"
)

// Movement is one line of a statement, normalised.
type Movement struct {
	// OccurredOn is the day the airline says it happened. Statements carry no clock we
	// can trust across programmes - Azul gives a date, Smiles a timestamp in its own
	// timezone - and the queue is ordered by day anyway.
	OccurredOn time.Time
	Kind       Kind
	// Locator is the booking reference, and is empty on an accrual. Azul repeats it once
	// per leg, so several movements can share one.
	Locator string
	// Points carries the airline's own sign: negative when they left the account. Keeping
	// the sign rather than an absolute value makes the sum of a statement its change in
	// balance, which is the one figure both files omit.
	Points int
	// Description is the airline's own wording, kept verbatim. It is what suggests an
	// origin for a batch ("Transferência de Pontos Livelo", "Pontuacao C6 Bank") and is
	// shown next to the movement so a human can tell two identical credits apart.
	Description string
}

// Statement is a parsed file.
type Statement struct {
	// AccountNumber is the programme's own number for the account, when the file says it.
	// Only Azul's does. It is advisory: the operator picks the account on the screen, and
	// this is what lets the screen warn that the file may be from a different one.
	AccountNumber string
	Movements     []Movement
}

// NetPoints is the change in balance the statement claims. Neither file states a balance,
// so this is the only way to check one.
func (s Statement) NetPoints() int {
	total := 0
	for _, movement := range s.Movements {
		total += movement.Points
	}
	return total
}
