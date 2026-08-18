package handlers

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// The beneficiary ledger is the thing that decides whether an operator is told "this account
// is one issuance away from being blocked". Everything here is about the two questions the
// programs actually ask - "how many different people" (Smiles, Azul) and "how many issuances"
// (LATAM) - and about not double-counting one person who arrives twice under two spellings.
//
// Real CPFs are needed: NormalizeCPF validates the check digits, so a made-up number is
// silently ignored and the test would pass by matching on the name instead.
const (
	cpfAlice = "529.982.247-25"
	cpfBruno = "111.444.777-35"

	// Any valid 32-byte key: the fingerprint only has to be stable within a test run.
	testCPFIndexKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
)

type cpfWorld struct {
	program data.LoyaltyProgram
	account data.Account
	airline data.Airline
	agency  data.Agency
	user    data.User
}

// seedCPFWorld builds an account on a program with a cap, in the mode and window the caller
// is testing.
func seedCPFWorld(t *testing.T, tx *gorm.DB, limit int, window, mode string) cpfWorld {
	t.Helper()
	suffix := uuid.New().String()[:8]

	airline := data.Airline{Name: "CPF Air", Code: "C" + suffix}
	create(t, tx, &airline)

	program := data.LoyaltyProgram{
		Name:             "CPF Program",
		Code:             "P" + suffix,
		DefaultAirlineID: airline.ID,
		CPFLimit:         &limit,
		CPFLimitWindow:   &window,
		CPFLimitMode:     &mode,
	}
	create(t, tx, &program)

	acc := data.Account{LoyaltyProgramID: program.ID, Category: data.AccountCategoryStandard}
	create(t, tx, &acc)

	agency := data.Agency{Name: "CPF Agency " + suffix}
	create(t, tx, &agency)

	return cpfWorld{
		program: program,
		account: acc,
		airline: airline,
		agency:  agency,
		user:    createUser(t, tx, "CPF Tester"),
	}
}

// issue creates an issuance on the world's account, backdated to at.
func (w cpfWorld) issue(t *testing.T, tx *gorm.DB, at time.Time) data.Issuance {
	t.Helper()
	issuance := data.Issuance{
		LoyaltyProgramID:   w.program.ID,
		AirlineID:          w.airline.ID,
		ProgramLocator:     "PL" + uuid.New().String()[:8],
		Points:             1000,
		ValueOfTheThousand: 50,
		AccountID:          w.account.ID,
		IssuerID:           w.user.ID,
		AgencyID:           w.agency.ID,
		CreatedAt:          at,
	}
	create(t, tx, &issuance)
	return issuance
}

func (w cpfWorld) usage(t *testing.T, tx *gorm.DB) *account.Usage {
	t.Helper()
	usage, err := account.UsageFor(tx, w.account, w.program)
	if err != nil {
		t.Fatalf("UsageFor returned an error: %v", err)
	}
	return usage
}

func register(t *testing.T, tx *gorm.DB, accountID, issuanceID, name string, cpf *string, at time.Time) {
	t.Helper()
	if err := account.Register(tx, accountID, issuanceID, name, cpf, at); err != nil {
		t.Fatalf("Register(%q) returned an error: %v", name, err)
	}
}

func TestSameCPFDoesNotConsumeASecondSlot(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		first := world.issue(t, tx, now)
		register(t, tx, world.account.ID, first.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		// Same person, a different spelling of the name, and one of the two masked. Only
		// the document decides.
		second := world.issue(t, tx, now)
		register(t, tx, world.account.ID, second.ID, "ALICE F FERREIRA", strPtr("52998224725"), now)

		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("used = %d, want 1: the same CPF opened a second slot", used)
		}
	})
}

func TestWithoutACPFTheNameCarriesTheMatch(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		first := world.issue(t, tx, now)
		register(t, tx, world.account.ID, first.ID, "João da Silva", nil, now)

		// Accents and spacing are not a different person - which is the whole point of the
		// fallback key, since imported history has neither documents nor consistent typing.
		second := world.issue(t, tx, now)
		register(t, tx, world.account.ID, second.ID, "JOAO  DA SILVA", nil, now)

		third := world.issue(t, tx, now)
		register(t, tx, world.account.ID, third.ID, "Maria Souza", nil, now)

		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("used = %d, want 2 (João and Maria)", used)
		}
	})
}

func TestACPFArrivingLaterCompletesTheBeneficiary(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		// The issuance is born with a name only; the Traveos sync brings the document.
		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Bruno Antunes", nil, now)
		register(t, tx, world.account.ID, issuance.ID, "Bruno Antunes", strPtr(cpfBruno), now)

		var beneficiaries []data.AccountBeneficiary
		if err := tx.Where("account_id = ?", world.account.ID).Find(&beneficiaries).Error; err != nil {
			t.Fatalf("failed to read the beneficiaries: %v", err)
		}
		if len(beneficiaries) != 1 {
			t.Fatalf("got %d beneficiaries, want 1: the document created a duplicate", len(beneficiaries))
		}
		if beneficiaries[0].CPFFingerprint == nil {
			t.Error("the beneficiary did not learn the CPF")
		}
		// The number itself is never stored - only a masked form reaches the screen.
		if got := beneficiaries[0].CPFMasked; got == nil || *got != "***.444.777-**" {
			t.Errorf("cpf_masked = %v, want ***.444.777-**", got)
		}
		if used := world.usage(t, tx).Used; used != 1 {
			t.Errorf("used = %d, want 1", used)
		}
	})
}

func TestRegisteringTheSameIssuanceTwiceChargesOneSlot(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	// Per-issuance mode, because in distinct-people mode this would pass for the wrong
	// reason. A Traveos resync deletes and reinserts the passengers, so it re-registers.
	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 24, data.CPFLimitWindowRolling12Months, data.CPFLimitModePerIssuance)
		now := time.Now()

		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("used = %d, want 1: a resync consumed a second slot", used)
		}
	})
}

func TestTheTwoCountingModesReadTheSameRowsDifferently(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		// One person, two issuances. Smiles reuses the slot; LATAM charges twice.
		for range 2 {
			issuance := world.issue(t, tx, now)
			register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)
		}

		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("distinct_people used = %d, want 1", used)
		}

		perIssuance := data.CPFLimitModePerIssuance
		world.program.CPFLimitMode = &perIssuance
		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("per_issuance used = %d, want 2", used)
		}
	})
}

func TestTheHolderDoesNotConsumeASlot(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		holder := data.Person{Name: "Carlos Holder"}
		create(t, tx, &holder)
		if err := tx.Model(&data.Account{}).Where("id = ?", world.account.ID).
			Update("holder_id", holder.ID).Error; err != nil {
			t.Fatalf("failed to attach the holder: %v", err)
		}

		own := world.issue(t, tx, now)
		register(t, tx, world.account.ID, own.ID, "Carlos Holder", nil, now)
		guest := world.issue(t, tx, now)
		register(t, tx, world.account.ID, guest.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		// None of the three programs counts the holder, so an administrator flying on
		// their own account every week must not eat its slots.
		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("used = %d, want 1: the holder was counted", used)
		}
	})
}

func TestCancellingAnIssuanceDoesNotGiveTheSlotBack(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		// The programs charge the slot when the ticket is issued and do not refund it, so
		// both a cancel and a delete have to leave the count alone.
		cancelledAt := time.Now()
		if err := tx.Model(&data.Issuance{}).Where("id = ?", issuance.ID).
			Update("cancelled_at", cancelledAt).Error; err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}
		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("after cancelling, used = %d, want 1", used)
		}

		if err := tx.Unscoped().Delete(&data.Issuance{}, "id = ?", issuance.ID).Error; err != nil {
			t.Fatalf("failed to delete: %v", err)
		}
		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("after deleting, used = %d, want 1", used)
		}
	})
}

func TestTheWindowDecidesWhatStillCounts(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		// December of last year, and thirteen months ago: the first is inside a rolling
		// twelve months but outside this calendar year, the second is outside both.
		lastDecember := time.Date(now.Year()-1, time.December, 15, 12, 0, 0, 0, now.Location())
		thirteenMonthsAgo := now.AddDate(0, -13, 0)
		thisYear := time.Date(now.Year(), time.January, 1, 12, 0, 0, 0, now.Location()).AddDate(0, 0, 1)

		for _, seed := range []struct {
			name string
			at   time.Time
		}{
			{"Alice Ferreira", lastDecember},
			{"Bruno Antunes", thirteenMonthsAgo},
			{"Carla Dias", thisYear},
		} {
			issuance := world.issue(t, tx, seed.at)
			register(t, tx, world.account.ID, issuance.ID, seed.name, nil, seed.at)
		}

		// Nothing resets anything on 1 January: the query simply stops seeing last year.
		if used := world.usage(t, tx).Used; used != 1 {
			t.Fatalf("calendar_year used = %d, want 1 (only this year's issuance)", used)
		}
		usage := world.usage(t, tx)
		if usage.ResetsAt == nil || usage.ResetsAt.Year() != now.Year()+1 {
			t.Errorf("resets_at = %v, want 1 January %d", usage.ResetsAt, now.Year()+1)
		}

		rolling := data.CPFLimitWindowRolling12Months
		world.program.CPFLimitWindow = &rolling
		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("rolling_12_months used = %d, want 2 (December and this year)", used)
		}
		if world.usage(t, tx).ResetsAt != nil {
			t.Error("a rolling window has no reset date")
		}
	})
}

func TestOnlyStandardAccountsWithAResearchedLimitShowACounter(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()
		issuance := world.issue(t, tx, now)
		register(t, tx, world.account.ID, issuance.ID, "Alice Ferreira", strPtr(cpfAlice), now)

		if world.usage(t, tx) == nil {
			t.Fatal("a standard account on a capped program should have a usage")
		}

		// An injunction suspends the cap outright, and an unidentified account gives no
		// basis to claim one applies. Both render no counter - which is not a zero.
		for _, category := range []string{data.AccountCategoryInjunction, data.AccountCategoryUnidentified} {
			world.account.Category = category
			if usage := world.usage(t, tx); usage != nil {
				t.Errorf("category %q produced a usage of %d, want none", category, usage.Used)
			}
		}

		world.account.Category = data.AccountCategoryStandard
		world.program.CPFLimit = nil
		if usage := world.usage(t, tx); usage != nil {
			t.Errorf("a program with no researched limit produced a usage of %d, want none", usage.Used)
		}
	})
}

func TestReplacingTheUsesFollowsACorrectedPassenger(t *testing.T) {
	t.Setenv("CPF_INDEX_KEY", testCPFIndexKey)

	runInTx(t, func(tx *gorm.DB) {
		world := seedCPFWorld(t, tx, 25, data.CPFLimitWindowCalendarYear, data.CPFLimitModeDistinctPeople)
		now := time.Now()

		issuance := world.issue(t, tx, now)
		// The bot path records the surname the airline search used; the sync then learns
		// who the passengers actually were.
		register(t, tx, world.account.ID, issuance.ID, "MACHADO", nil, now)

		err := account.ReplaceIssuanceUses(tx, world.account.ID, issuance.ID, []account.Passenger{
			{Name: "Jose Machado", CPF: strPtr(cpfAlice)},
			{Name: "Ana Machado"},
		}, now)
		if err != nil {
			t.Fatalf("ReplaceIssuanceUses returned an error: %v", err)
		}

		// Two real passengers, and the surname-only placeholder gone rather than left
		// counting as a third person.
		if used := world.usage(t, tx).Used; used != 2 {
			t.Fatalf("used = %d, want 2", used)
		}
		var names []string
		if err := tx.Model(&data.AccountBeneficiary{}).
			Where("account_id = ?", world.account.ID).
			Order("name").Pluck("name", &names).Error; err != nil {
			t.Fatalf("failed to read the beneficiaries: %v", err)
		}
		if len(names) != 2 || names[0] != "Ana Machado" || names[1] != "Jose Machado" {
			t.Errorf("beneficiaries = %v, want [Ana Machado Jose Machado]", names)
		}
	})
}
