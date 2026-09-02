package account

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
)

func intPtr(v int) *int       { return &v }
func strPtr(v string) *string { return &v }

// capped is what decides whether a counter is shown at all, and every false here
// is a deliberate product rule rather than missing data handling: an injunction
// suspends the cap, an unidentified account gives no basis to claim one, and a
// program with no researched limit has nothing to count against.
func TestCapped(t *testing.T) {
	t.Parallel()

	full := data.LoyaltyProgram{
		CPFLimit:       intPtr(25),
		CPFLimitWindow: strPtr(data.CPFLimitWindowCalendarYear),
		CPFLimitMode:   strPtr(data.CPFLimitModeDistinctPeople),
	}

	cases := []struct {
		name    string
		account data.Account
		program data.LoyaltyProgram
		want    bool
	}{
		{"standard account on a fully researched program", data.Account{Category: data.AccountCategoryStandard}, full, true},
		{"an injunction suspends the cap", data.Account{Category: data.AccountCategoryInjunction}, full, false},
		{"an unidentified account has no basis for one", data.Account{Category: data.AccountCategoryUnidentified}, full, false},
		{
			name:    "no researched limit, nothing to count against",
			account: data.Account{Category: data.AccountCategoryStandard},
			program: data.LoyaltyProgram{CPFLimitWindow: full.CPFLimitWindow, CPFLimitMode: full.CPFLimitMode},
		},
		{
			name:    "a limit with no window is not a rule yet",
			account: data.Account{Category: data.AccountCategoryStandard},
			program: data.LoyaltyProgram{CPFLimit: full.CPFLimit, CPFLimitMode: full.CPFLimitMode},
		},
		{
			name:    "a limit with no mode is not a rule yet",
			account: data.Account{Category: data.AccountCategoryStandard},
			program: data.LoyaltyProgram{CPFLimit: full.CPFLimit, CPFLimitWindow: full.CPFLimitWindow},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := capped(tc.account, tc.program); got != tc.want {
				t.Fatalf("capped() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWindowStart(t *testing.T) {
	t.Parallel()

	// Deliberately not UTC: the programs reset on the operator's calendar, and a
	// late-evening instant in São Paulo is already the next day in UTC.
	saoPaulo, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatalf("loading the location: %v", err)
	}
	now := time.Date(2026, time.September, 2, 22, 30, 0, 0, saoPaulo)

	t.Run("a rolling window is twelve months back and never resets", func(t *testing.T) {
		t.Parallel()
		since, resetsAt := windowStart(data.CPFLimitWindowRolling12Months, now)

		want := time.Date(2025, time.September, 2, 22, 30, 0, 0, saoPaulo)
		if !since.Equal(want) {
			t.Fatalf("since = %v, want %v", since, want)
		}
		if resetsAt != nil {
			t.Fatalf("resetsAt = %v, want nil - a rolling window slides, it does not reset", resetsAt)
		}
	})

	t.Run("a calendar year opens on 1 January in the operator's zone", func(t *testing.T) {
		t.Parallel()
		since, resetsAt := windowStart(data.CPFLimitWindowCalendarYear, now)

		wantSince := time.Date(2026, time.January, 1, 0, 0, 0, 0, saoPaulo)
		if !since.Equal(wantSince) {
			t.Fatalf("since = %v, want %v", since, wantSince)
		}
		wantReset := time.Date(2027, time.January, 1, 0, 0, 0, 0, saoPaulo)
		if resetsAt == nil {
			t.Fatalf("resetsAt = nil, want %v", wantReset)
		} else if !resetsAt.Equal(wantReset) {
			t.Fatalf("resetsAt = %v, want %v", *resetsAt, wantReset)
		}
	})

	// The fallback matters more than it looks: a window value this code does not
	// recognise must not read as "no window", which would show every account as
	// having consumed nothing.
	t.Run("an unrecognised window falls back to the calendar year, never to unlimited", func(t *testing.T) {
		t.Parallel()
		since, resetsAt := windowStart("something_nobody_wrote_yet", now)

		if since.Year() != 2026 || since.Month() != time.January || since.Day() != 1 {
			t.Fatalf("since = %v, want 1 January 2026", since)
		}
		if resetsAt == nil {
			t.Fatal("resetsAt = nil, want the calendar-year reset")
		}
	})
}

// One query reads all four counts; pick is what selects the reading a given
// program's rule actually asks for. Crossing the wires here reports the wrong
// consumption against the cap, which is a number an operator acts on.
func TestAccountCountsPick(t *testing.T) {
	t.Parallel()

	counts := accountCounts{
		DistinctYear:    1,
		IssuancesYear:   2,
		DistinctRolling: 3,
		IssuancesRolls:  4,
	}

	cases := []struct {
		window string
		mode   string
		want   int
	}{
		{data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople, 1},
		{data.CPFLimitWindowCalendarYear, data.CPFLimitModePerIssuance, 2},
		{data.CPFLimitWindowRolling12Months, data.CPFLimitModeDistinctPeople, 3},
		{data.CPFLimitWindowRolling12Months, data.CPFLimitModePerIssuance, 4},
		// Same fallback as windowStart, and for the same reason.
		{"unknown", data.CPFLimitModeDistinctPeople, 1},
	}

	for _, tc := range cases {
		t.Run(tc.window+"/"+tc.mode, func(t *testing.T) {
			t.Parallel()
			if got := counts.pick(tc.window, tc.mode); got != tc.want {
				t.Fatalf("pick(%q, %q) = %d, want %d", tc.window, tc.mode, got, tc.want)
			}
		})
	}
}
