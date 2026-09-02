package account

import (
	"errors"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
)

// passengersOf decides who an issuance counted against the account's cap, and the
// order of its two sources is the rule: the synced Traveos rows when they exist,
// and only otherwise the single name the grid or the bot recorded. Reading the
// fallback first would count one passenger for a booking that flew four.
func TestPassengersOf(t *testing.T) {
	t.Parallel()

	mainPassenger := "MARIA SOUZA"
	document := "71460238001"

	t.Run("the synced passenger rows win when they exist", func(t *testing.T) {
		t.Parallel()
		got := passengersOf(data.Issuance{
			MainPassengerName: &mainPassenger,
			Passengers: []data.IssuancePassenger{
				{FirstName: "JOAO", LastName: "SILVA", Document: &document},
				{FirstName: "ANA", LastName: "LIMA"},
			},
		})

		if len(got) != 2 {
			t.Fatalf("got %d passengers, want 2 - the synced rows, not the single grid name", len(got))
		}
		if got[0].Name != "JOAO SILVA" || got[0].CPF == nil || *got[0].CPF != document {
			t.Fatalf("first passenger = %+v, want JOAO SILVA carrying the document", got[0])
		}
		if got[1].Name != "ANA LIMA" || got[1].CPF != nil {
			t.Fatalf("second passenger = %+v, want ANA LIMA with no document", got[1])
		}
	})

	t.Run("falls back to the single recorded name", func(t *testing.T) {
		t.Parallel()
		got := passengersOf(data.Issuance{MainPassengerName: &mainPassenger})

		if len(got) != 1 || got[0].Name != mainPassenger {
			t.Fatalf("got %+v, want the one recorded name", got)
		}
	})

	t.Run("a blank name is nobody, not an empty passenger", func(t *testing.T) {
		t.Parallel()
		blank := "   "
		if got := passengersOf(data.Issuance{MainPassengerName: &blank}); got != nil {
			t.Fatalf("got %+v, want nil - a blank name must not open a beneficiary", got)
		}
		if got := passengersOf(data.Issuance{}); got != nil {
			t.Fatalf("got %+v, want nil", got)
		}
	})
}

// The asymmetry this pins is deliberate: a document a human typed is refused,
// while Register degrades to matching by name, because refusing an issuance over
// a document the airline sent is worse than counting it by name.
func TestRejectBadCPF(t *testing.T) {
	t.Parallel()

	valid := "714.602.380-01"
	invalid := "111.111.111-11"
	empty := ""

	if err := rejectBadCPF(nil); err != nil {
		t.Fatalf("rejectBadCPF(nil) = %v, want nil - no document is not a bad document", err)
	}
	if err := rejectBadCPF(&empty); err != nil {
		t.Fatalf("rejectBadCPF(\"\") = %v, want nil", err)
	}
	if err := rejectBadCPF(&valid); err != nil {
		t.Fatalf("rejectBadCPF(valid) = %v, want nil", err)
	}
	if err := rejectBadCPF(&invalid); !errors.Is(err, ErrInvalidCPF) {
		t.Fatalf("rejectBadCPF(invalid) = %v, want ErrInvalidCPF", err)
	}
}
