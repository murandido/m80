package bot

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"gorm.io/gorm"
)

// emissaoFixture is the FK chain an EMISSAO needs before any nickname is looked
// up: a loyalty program pointing at an airline, and a real airport for line 2.
type emissaoFixture struct {
	Airline data.Airline
	Program data.LoyaltyProgram
	Airport data.Airport
	Suffix  string
}

func seedEmissaoFixture(t *testing.T, tx *gorm.DB, airline data.Airline) emissaoFixture {
	t.Helper()
	suffix := strings.ToUpper(uuid.New().String()[:6])

	if airline.Name == "" {
		airline.Name = "Test Air"
	}
	airline.Code = "A" + suffix
	seed(t, tx, &airline)

	program := data.LoyaltyProgram{Name: "Test Program", Code: "P" + suffix, DefaultAirlineID: airline.ID}
	seed(t, tx, &program)

	// airports is populated by data.SeedAirports at API startup, which never
	// runs in a test binary - so CNF is seeded here when it isn't already
	// present, rather than assumed.
	var airport data.Airport
	if err := tx.Where(data.Airport{IATACode: "CNF"}).
		Attrs(data.Airport{Name: "Confins", Country: "BR"}).
		FirstOrCreate(&airport).Error; err != nil {
		t.Fatalf("failed to seed the CNF airport: %v", err)
	}

	return emissaoFixture{Airline: airline, Program: program, Airport: airport, Suffix: suffix}
}

func seed(t *testing.T, tx *gorm.DB, model any) {
	t.Helper()
	if err := tx.Create(model).Error; err != nil {
		t.Fatalf("failed to seed %T: %v", model, err)
	}
}

// message builds a parsed message for the fixture's program, so tests only spell
// out the part they are about.
func (f emissaoFixture) message(accountNickname string) *emissaoMessage {
	return &emissaoMessage{
		ProgramCode:        f.Program.Code,
		ProgramLocator:     "PL" + f.Suffix,
		OriginIATACode:     "CNF",
		Points:             31000,
		ValueOfTheThousand: 1650,
		Taxes:              18922,
		Total:              70072,
		AccountNickname:    accountNickname,
	}
}

func TestResolveEmissao_ExistingNicknameReusesAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		account := data.Account{LoyaltyProgramID: f.Program.ID, Complete: true}
		seed(t, tx, &account)
		nickname := data.AccountNickname{
			AccountID:        account.ID,
			LoyaltyProgramID: f.Program.ID,
			Nickname:         "Heraldo",
		}
		seed(t, tx, &nickname)

		before := countRows(t, tx, "accounts", "loyalty_program_id = ?", f.Program.ID)

		// Typed in a different case than it was registered - the same account.
		input, err := resolveEmissao(tx, f.message("heraldo"))
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}

		if input.AccountID != account.ID {
			t.Fatalf("AccountID = %q, want the existing account %q", input.AccountID, account.ID)
		}
		if input.AccountNicknameID == nil || *input.AccountNicknameID != nickname.ID {
			t.Fatalf("AccountNicknameID = %v, want %q", input.AccountNicknameID, nickname.ID)
		}
		if after := countRows(t, tx, "accounts", "loyalty_program_id = ?", f.Program.ID); after != before {
			t.Fatalf("accounts went from %d to %d, expected no new row", before, after)
		}
	})
}

func TestResolveEmissao_UnknownNicknameCreatesIncompleteAccount(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		input, err := resolveEmissao(tx, f.message("Heraldo"))
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}

		var account data.Account
		if err := tx.First(&account, "id = ?", input.AccountID).Error; err != nil {
			t.Fatalf("expected the account to have been created: %v", err)
		}
		if account.Complete {
			t.Fatal("an account conjured from a message must be marked incomplete")
		}
		if account.LoyaltyProgramID != f.Program.ID {
			t.Fatalf("account belongs to program %q, want %q", account.LoyaltyProgramID, f.Program.ID)
		}

		// Stored as typed, even though lookups ignore case.
		var nickname data.AccountNickname
		if err := tx.First(&nickname, "id = ?", *input.AccountNicknameID).Error; err != nil {
			t.Fatalf("expected the nickname to have been created: %v", err)
		}
		if nickname.Nickname != "Heraldo" {
			t.Fatalf("nickname = %q, want it stored exactly as typed", nickname.Nickname)
		}

		// The alias names a person too, and without one the panel's account card
		// falls back to "Conta sem titular" - the account exists but reads as
		// nameless, which is what an operator reported.
		if account.HolderID == nil {
			t.Fatal("the account was created with no holder, so the panel has no name to show for it")
		}
		var holder data.Person
		if err := tx.First(&holder, "id = ?", *account.HolderID).Error; err != nil {
			t.Fatalf("expected the holder to have been created: %v", err)
		}
		if holder.Name != "Heraldo" {
			t.Fatalf("holder name = %q, want Heraldo", holder.Name)
		}
		if holder.Complete {
			t.Fatal("a person conjured from a message must be marked incomplete")
		}

		var personNicknames int64
		if err := tx.Model(&data.PersonNickname{}).Where("person_id = ?", holder.ID).Count(&personNicknames).Error; err != nil {
			t.Fatalf("failed to count person_nicknames: %v", err)
		}
		if personNicknames != 1 {
			t.Fatalf("person_nicknames = %d, want 1", personNicknames)
		}

		// Left null on purpose: see attachAdministrator. A defaulted administrator
		// would swallow the next different name on line 4 as one of its aliases.
		if account.AdministratorID != nil {
			t.Fatalf("administrator_id = %v, want nil on a freshly conjured account", account.AdministratorID)
		}
	})
}

func TestResolveEmissao_AdminNicknameCreatesIncompletePersonAndLinks(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		msg := f.message("Heraldo")
		msg.AdminNickname = "Fulano" + f.Suffix

		input, err := resolveEmissao(tx, msg)
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}

		var person data.Person
		if err := tx.Where("name = ?", msg.AdminNickname).First(&person).Error; err != nil {
			t.Fatalf("expected the person to have been created: %v", err)
		}
		if person.Complete {
			t.Fatal("a person conjured from a message must be marked incomplete")
		}

		var account data.Account
		if err := tx.First(&account, "id = ?", input.AccountID).Error; err != nil {
			t.Fatalf("failed to reload the account: %v", err)
		}
		if account.AdministratorID == nil || *account.AdministratorID != person.ID {
			t.Fatalf("administrator_id = %v, want %q", account.AdministratorID, person.ID)
		}
	})
}

// seedAccountWithAdministrator seeds an account that already has an
// administrator, reachable by the nickname "Heraldo" - the shape the two tests
// below are about.
func seedAccountWithAdministrator(t *testing.T, tx *gorm.DB, f emissaoFixture, adminName string) (data.Account, data.Person) {
	t.Helper()

	admin := data.Person{Name: adminName}
	seed(t, tx, &admin)
	seed(t, tx, &data.PersonNickname{PersonID: admin.ID, Nickname: adminName})

	account := data.Account{LoyaltyProgramID: f.Program.ID, AdministratorID: &admin.ID}
	seed(t, tx, &account)
	seed(t, tx, &data.AccountNickname{
		AccountID:        account.ID,
		LoyaltyProgramID: f.Program.ID,
		Nickname:         "Heraldo",
	})

	return account, admin
}

// An account's administrator is a settled fact, so a different word for it in a
// message is another way of naming that same person. This used to be ignored in
// silence, which is how a message naming the wrong administrator looked like it
// had worked.
func TestResolveEmissao_NewAdminNameBecomesAnAliasOfTheCurrentOne(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})
		account, admin := seedAccountWithAdministrator(t, tx, f, "Wilmara"+f.Suffix)

		peopleBefore := countRows(t, tx, "people", "name = ?", "Fulano"+f.Suffix)

		msg := f.message("Heraldo")
		msg.AdminNickname = "Fulano" + f.Suffix

		if _, err := resolveEmissao(tx, msg); err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}

		// The new name points at the person who was already the administrator.
		var alias data.PersonNickname
		if err := tx.Where("nickname = ?", msg.AdminNickname).First(&alias).Error; err != nil {
			t.Fatalf("expected %q to have been registered as an alias: %v", msg.AdminNickname, err)
		}
		if alias.PersonID != admin.ID {
			t.Fatalf("the alias points at %q, want the current administrator %q", alias.PersonID, admin.ID)
		}

		// And no second person was invented for it.
		if after := countRows(t, tx, "people", "name = ?", "Fulano"+f.Suffix); after != peopleBefore {
			t.Fatalf("people named %q went from %d to %d - a duplicate person was created",
				msg.AdminNickname, peopleBefore, after)
		}

		var reloaded data.Account
		if err := tx.First(&reloaded, "id = ?", account.ID).Error; err != nil {
			t.Fatalf("failed to reload the account: %v", err)
		}
		if *reloaded.AdministratorID != admin.ID {
			t.Fatal("the account's administrator was replaced instead of gaining an alias")
		}
	})
}

// The alias only works while the name is free. Once it belongs to someone else
// the two readings genuinely conflict - person_nicknames.nickname is unique
// globally - so the message is refused rather than guessed at.
func TestResolveEmissao_AdminNameOwnedBySomeoneElseIsRejected(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})
		_, admin := seedAccountWithAdministrator(t, tx, f, "Wilmara"+f.Suffix)

		// A third party who already answers to the name the message uses.
		stranger := data.Person{Name: "Fulano" + f.Suffix}
		seed(t, tx, &stranger)
		seed(t, tx, &data.PersonNickname{PersonID: stranger.ID, Nickname: "Fulano" + f.Suffix})

		msg := f.message("Heraldo")
		msg.AdminNickname = "Fulano" + f.Suffix

		_, err := resolveEmissao(tx, msg)
		if err == nil {
			t.Fatal("expected the conflict to be refused")
		}
		// The operator has to be able to see both sides to know which one is wrong.
		if !strings.Contains(err.Error(), admin.Name) || !strings.Contains(err.Error(), msg.AdminNickname) {
			t.Fatalf("err = %q, want it to name both the current administrator and the rejected name", err)
		}

		var operator operatorError
		if !errors.As(err, &operator) {
			t.Fatal("the conflict must be an operatorError, or the bot answers with a generic apology")
		}
	})
}

func TestResolveEmissao_AgencyNicknameCreatesNonDefaultAgency(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		msg := f.message("Heraldo")
		msg.AgencyNickname = "Agencia" + f.Suffix

		input, err := resolveEmissao(tx, msg)
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}
		if input.AgencyID == nil {
			t.Fatal("expected the agency to have been resolved")
		}

		var agency data.Agency
		if err := tx.First(&agency, "id = ?", *input.AgencyID).Error; err != nil {
			t.Fatalf("expected the agency to have been created: %v", err)
		}
		if agency.IsDefault {
			t.Fatal("an agency conjured from a message must never become the default")
		}
	})
}

// No agency on line 4 leaves the field nil, so the issuance service falls back
// to whichever agency is flagged is_default.
func TestResolveEmissao_NoAgencyLeavesTheDefaultToTheService(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		input, err := resolveEmissao(tx, f.message("Heraldo"))
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}
		if input.AgencyID != nil {
			t.Fatalf("AgencyID = %v, want nil", input.AgencyID)
		}
	})
}

func TestResolveEmissao_AirlineRequirements(t *testing.T) {
	tests := []struct {
		name    string
		airline data.Airline
		extras  []string
		wantErr error
		assert  func(t *testing.T, in issuancesvc.CreateInput)
	}{
		{
			name:    "last name required but missing",
			airline: data.Airline{RequiresLastName: true},
			wantErr: issuancesvc.ErrLastNameRequired,
		},
		{
			name:    "last name required and present",
			airline: data.Airline{RequiresLastName: true},
			extras:  []string{"MACHADO"},
			assert: func(t *testing.T, in issuancesvc.CreateInput) {
				if in.MainPassengerName == nil || *in.MainPassengerName != "MACHADO" {
					t.Fatalf("MainPassengerName = %v, want MACHADO", in.MainPassengerName)
				}
			},
		},
		{
			name:    "purchase number required but missing",
			airline: data.Airline{RequiresLastName: true, RequiresPurchaseNumber: true},
			extras:  []string{"FELIX"},
			wantErr: issuancesvc.ErrPurchaseNumberRequired,
		},
		{
			name:    "both required and present",
			airline: data.Airline{RequiresLastName: true, RequiresPurchaseNumber: true},
			extras:  []string{"FELIX", "LA9574243YWPJ"},
			assert: func(t *testing.T, in issuancesvc.CreateInput) {
				if in.MainPassengerName == nil || *in.MainPassengerName != "FELIX" {
					t.Fatalf("MainPassengerName = %v, want FELIX", in.MainPassengerName)
				}
				if in.LatamPurchaseNumber == nil || *in.LatamPurchaseNumber != "LA9574243YWPJ" {
					t.Fatalf("LatamPurchaseNumber = %v, want LA9574243YWPJ", in.LatamPurchaseNumber)
				}
			},
		},
		{
			name:    "volunteered last name is kept",
			airline: data.Airline{},
			extras:  []string{"MACHADO"},
			assert: func(t *testing.T, in issuancesvc.CreateInput) {
				if in.MainPassengerName == nil || *in.MainPassengerName != "MACHADO" {
					t.Fatalf("MainPassengerName = %v, want MACHADO", in.MainPassengerName)
				}
			},
		},
		{
			name:    "more fields than the airline uses",
			airline: data.Airline{},
			extras:  []string{"MACHADO", "LA9574243YWPJ"},
			wantErr: nil, // asserted below as a plain error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runInTx(t, func(tx *gorm.DB) {
				f := seedEmissaoFixture(t, tx, tt.airline)

				msg := f.message("Heraldo")
				msg.Line2Extras = tt.extras

				in, err := resolveEmissao(tx, msg)

				if tt.wantErr != nil {
					if !errors.Is(err, tt.wantErr) {
						t.Fatalf("err = %v, want %v", err, tt.wantErr)
					}
					return
				}
				if tt.assert == nil {
					if err == nil {
						t.Fatal("expected resolveEmissao to reject the extra fields")
					}
					return
				}
				if err != nil {
					t.Fatalf("resolveEmissao returned %v", err)
				}
				tt.assert(t, in)
			})
		})
	}
}

func TestResolveEmissao_ObservationPreset(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		preset := data.ObservationPreset{Code: "ACORDO" + f.Suffix, Text: "Emissão em acordo com o cliente"}
		seed(t, tx, &preset)

		msg := f.message("Heraldo")
		msg.ObservationCode = preset.Code

		input, err := resolveEmissao(tx, msg)
		if err != nil {
			t.Fatalf("resolveEmissao returned %v", err)
		}
		if input.Observation == nil || *input.Observation != preset.Text {
			t.Fatalf("Observation = %v, want the preset's text", input.Observation)
		}
	})
}

func TestResolveEmissao_UnknownObservationPresetIsRejected(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		msg := f.message("Heraldo")
		msg.ObservationCode = "NOSUCHCODE"

		_, err := resolveEmissao(tx, msg)
		if err == nil || !strings.Contains(err.Error(), "não é um código de observação cadastrado") {
			t.Fatalf("err = %v, want it to name the unknown code", err)
		}
	})
}

func TestResolveEmissao_UnknownProgramIsRejected(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		msg := f.message("Heraldo")
		msg.ProgramCode = "NOSUCHPROGRAM"

		_, err := resolveEmissao(tx, msg)
		if err == nil || !strings.Contains(err.Error(), "Não encontrei nenhum programa") {
			t.Fatalf("err = %v, want it to name the unknown program", err)
		}
	})
}

func TestResolveEmissao_UnknownAirportIsRejected(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})

		msg := f.message("Heraldo")
		msg.OriginIATACode = "ZZZ"

		_, err := resolveEmissao(tx, msg)
		if !errors.Is(err, issuancesvc.ErrAirportNotFound) {
			t.Fatalf("err = %v, want ErrAirportNotFound", err)
		}
	})
}

func countRows(t *testing.T, tx *gorm.DB, table, where string, arg any) int64 {
	t.Helper()

	var count int64
	if err := tx.Table(table).Where(where, arg).Count(&count).Error; err != nil {
		t.Fatalf("failed to count %s: %v", table, err)
	}
	return count
}
