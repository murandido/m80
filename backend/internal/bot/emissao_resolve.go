package bot

import (
	"errors"
	"fmt"
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	issuancesvc "github.com/murandido/m-80-backend/internal/issuance"
	"gorm.io/gorm"
)

// operatorError is an error whose message was written for the person who sent
// the WhatsApp message - in Portuguese, naming what they typed - and is
// therefore safe to send straight back to the group. Anything that is not one
// (a dropped connection, a constraint nobody anticipated) is logged and answered
// with a generic apology instead, so a Postgres error never lands in a chat.
type operatorError struct {
	message string
	cause   error
}

func (e operatorError) Error() string { return e.message }

// Unwrap keeps errors.Is working through the friendly message, so callers can
// still recognise the typed error underneath.
func (e operatorError) Unwrap() error { return e.cause }

func operatorErrorf(cause error, format string, args ...any) error {
	return operatorError{message: fmt.Sprintf(format, args...), cause: cause}
}

// resolveEmissao turns a parsed message into the input the issuance service
// expects, looking every code and nickname up in the database.
//
// It must run inside a transaction, because resolving is not read-only: a
// nickname nobody registered yet creates the record it names. That is
// deliberate. The alternative - refusing the message - would mean an operator
// cannot issue a ticket until someone opens the panel and fills in a full
// account, and the whole point of typing it into WhatsApp is that it works from
// the airport. What gets created is marked incomplete, so it surfaces in the
// panel as something still to be filled in rather than passing for real data.
func resolveEmissao(tx *gorm.DB, msg *emissaoMessage) (issuancesvc.CreateInput, error) {
	var input issuancesvc.CreateInput

	var program data.LoyaltyProgram
	if err := tx.Where("LOWER(code) = LOWER(?)", msg.ProgramCode).First(&program).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return input, operatorErrorf(err, "Não encontrei nenhum programa com o código %q", msg.ProgramCode)
		}
		return input, err
	}

	airline, err := resolveEmissaoAirline(tx, msg, program)
	if err != nil {
		return input, err
	}

	lastName, purchaseNumber, err := assignLine2Extras(airline, msg.Line2Extras)
	if err != nil {
		return input, err
	}

	if err := issuancesvc.ValidateAirportCodes(tx, &msg.OriginIATACode); err != nil {
		if errors.Is(err, issuancesvc.ErrAirportNotFound) {
			return input, operatorErrorf(err, "Não encontrei o aeroporto %q", msg.OriginIATACode)
		}
		return input, err
	}

	account, nicknameID, err := resolveAccountByNickname(tx, program.ID, msg.AccountNickname)
	if err != nil {
		return input, err
	}

	if msg.AdminNickname != "" {
		if err := attachAdministrator(tx, &account, msg.AccountNickname, msg.AdminNickname); err != nil {
			return input, err
		}
	}

	var agencyID *string
	if msg.AgencyNickname != "" {
		id, err := resolveAgencyByNickname(tx, msg.AgencyNickname)
		if err != nil {
			return input, err
		}
		agencyID = &id
	}

	observation, err := resolveObservation(tx, msg.ObservationCode)
	if err != nil {
		return input, err
	}

	return issuancesvc.CreateInput{
		LoyaltyProgramID:    program.ID,
		AirlineID:           &airline.ID,
		ProgramLocator:      msg.ProgramLocator,
		AirlineLocator:      optionalString(msg.AirlineLocator),
		LatamPurchaseNumber: purchaseNumber,
		MainPassengerName:   lastName,
		OriginIATACode:      &msg.OriginIATACode,
		Points:              msg.Points,
		ValueOfTheThousand:  msg.ValueOfTheThousand,
		Taxes:               &msg.Taxes,
		AccountID:           account.ID,
		AccountNicknameID:   nicknameID,
		AgencyID:            agencyID,
		Observation:         observation,
		// The message carries no flight date, destination or passenger details,
		// so the issuance is born incomplete exactly like a draft row autosaved
		// from the grid - the panel is where the rest gets filled in.
		Complete: false,
	}, nil
}

// resolveEmissaoAirline picks the airline named on line 1, or the loyalty
// program's default when line 1 named none.
func resolveEmissaoAirline(tx *gorm.DB, msg *emissaoMessage, program data.LoyaltyProgram) (data.Airline, error) {
	var airline data.Airline

	if msg.AirlineCode == "" {
		err := tx.First(&airline, "id = ?", program.DefaultAirlineID).Error
		return airline, err
	}

	err := tx.Where("LOWER(code) = LOWER(?)", msg.AirlineCode).First(&airline).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return airline, operatorErrorf(err, "Não encontrei nenhuma cia aérea com o código %q", msg.AirlineCode)
	}
	return airline, err
}

// assignLine2Extras decides what the fields after the origin airport mean. The
// parser cannot: the message says "GIG - FELIX - LA9574243YWPJ" with no hint of
// which is which, and the answer lives in the airline's requires_last_name and
// requires_purchase_number flags. They appear in that fixed order, so once the
// flags are known the assignment is positional.
func assignLine2Extras(airline data.Airline, extras []string) (lastName, purchaseNumber *string, err error) {
	remaining := extras
	take := func() *string {
		if len(remaining) == 0 {
			return nil
		}
		value := remaining[0]
		remaining = remaining[1:]
		return &value
	}

	if airline.RequiresLastName {
		if lastName = take(); lastName == nil {
			return nil, nil, operatorErrorf(issuancesvc.ErrLastNameRequired, "A %s exige o sobrenome do passageiro na linha 2", airline.Name)
		}
	}
	if airline.RequiresPurchaseNumber {
		if purchaseNumber = take(); purchaseNumber == nil {
			return nil, nil, operatorErrorf(issuancesvc.ErrPurchaseNumberRequired, "A %s exige o número da compra na linha 2", airline.Name)
		}
	}

	// An airline that doesn't demand a last name still records one when the
	// operator volunteered it - it costs nothing and saves typing it again in
	// the panel.
	if lastName == nil && len(remaining) > 0 {
		lastName = take()
	}

	if len(remaining) > 0 {
		return nil, nil, operatorErrorf(nil, "A linha 2 tem %d campo(s) que a %s não usa: %s",
			len(remaining), airline.Name, strings.Join(remaining, ", "))
	}
	return lastName, purchaseNumber, nil
}

// resolveAccountByNickname finds the account an alias points at, creating an
// incomplete one when the alias is new. Matching ignores case so "heraldo" and
// "Heraldo" are the same account; the alias is stored exactly as typed.
func resolveAccountByNickname(tx *gorm.DB, programID, nickname string) (data.Account, *string, error) {
	var existing data.AccountNickname
	err := tx.Where("loyalty_program_id = ? AND LOWER(nickname) = LOWER(?)", programID, nickname).
		Order("created_at").
		First(&existing).Error

	switch {
	case err == nil:
		var account data.Account
		if err := tx.First(&account, "id = ?", existing.AccountID).Error; err != nil {
			return data.Account{}, nil, err
		}
		return account, &existing.ID, nil

	case errors.Is(err, gorm.ErrRecordNotFound):
		// The alias names whose account this is, so it also names a person. Creating
		// that person and linking them as the holder is what makes the account
		// recognisable in the panel: an account has no name of its own, and the
		// cards there identify one by its holder.
		holderID, err := resolvePersonByNickname(tx, nickname)
		if err != nil {
			return data.Account{}, nil, err
		}

		// administrator_id stays null on purpose. Defaulting it to the holder would
		// make the first different name on line 4 become an alias of the holder
		// forever - see attachAdministrator, which only ever aliases onto an
		// administrator somebody actually chose.
		account := data.Account{LoyaltyProgramID: programID, HolderID: &holderID, Complete: false}
		if err := tx.Create(&account).Error; err != nil {
			return data.Account{}, nil, err
		}

		created := data.AccountNickname{
			AccountID:        account.ID,
			LoyaltyProgramID: programID,
			Nickname:         nickname,
		}
		if err := tx.Create(&created).Error; err != nil {
			return data.Account{}, nil, err
		}
		return account, &created.ID, nil

	default:
		return data.Account{}, nil, err
	}
}

// attachAdministrator decides what an administrator name on line 4 means for an
// account that may already have one.
//
// The rule the operators asked for: an account's administrator is a settled fact,
// so a different word for it in a message is another way of naming that same
// person, not a new one. Writing "Fulano" where the account's administrator is
// already Wilmara registers "Fulano" as another alias of Wilmara.
//
// That only holds while the name is free. Once it belongs to somebody else the
// two readings genuinely conflict - person_nicknames.nickname is unique globally,
// so it cannot point at both - and the message is refused rather than guessed at.
// Refusing is also what the old code failed to do: it returned silently, which is
// how a message naming the wrong administrator looked like it had worked.
func attachAdministrator(tx *gorm.DB, account *data.Account, accountNickname, nickname string) error {
	personID, found, err := findPersonByNickname(tx, nickname)
	if err != nil {
		return err
	}

	switch {
	case found && account.AdministratorID == nil:
		return setAccountAdministrator(tx, account, personID)

	case found && *account.AdministratorID == personID:
		// Already the administrator, under this name or another of their aliases.
		return nil

	case found:
		var current data.Person
		if err := tx.First(&current, "id = ?", *account.AdministratorID).Error; err != nil {
			return err
		}
		return operatorErrorf(nil,
			"A conta %q já tem %s como administrador, e %q já é de outra pessoa",
			accountNickname, current.Name, nickname)

	case account.AdministratorID != nil:
		// Free name, account already has an administrator: another alias for them.
		return tx.Create(&data.PersonNickname{PersonID: *account.AdministratorID, Nickname: nickname}).Error

	default:
		// Free name, no administrator yet: a new person takes the role.
		created, err := resolvePersonByNickname(tx, nickname)
		if err != nil {
			return err
		}
		return setAccountAdministrator(tx, account, created)
	}
}

func setAccountAdministrator(tx *gorm.DB, account *data.Account, personID string) error {
	account.AdministratorID = &personID
	return tx.Model(account).Update("administrator_id", personID).Error
}

// findPersonByNickname looks an alias up without creating anything, so callers
// that need to branch on whether it exists can do so before committing to it.
func findPersonByNickname(tx *gorm.DB, nickname string) (string, bool, error) {
	var existing data.PersonNickname
	err := tx.Where("LOWER(nickname) = LOWER(?)", nickname).Order("created_at").First(&existing).Error

	switch {
	case err == nil:
		return existing.PersonID, true, nil
	case errors.Is(err, gorm.ErrRecordNotFound):
		return "", false, nil
	default:
		return "", false, err
	}
}

// resolvePersonByNickname returns the person an alias names, creating an
// incomplete one when the alias is new.
func resolvePersonByNickname(tx *gorm.DB, nickname string) (string, error) {
	personID, found, err := findPersonByNickname(tx, nickname)
	if err != nil {
		return "", err
	}
	if found {
		return personID, nil
	}

	// The alias is all the message gives us, so it doubles as the name until
	// someone completes the record in the panel.
	person := data.Person{Name: nickname, Complete: false}
	if err := tx.Create(&person).Error; err != nil {
		return "", err
	}
	if err := tx.Create(&data.PersonNickname{PersonID: person.ID, Nickname: nickname}).Error; err != nil {
		return "", err
	}
	return person.ID, nil
}

func resolveAgencyByNickname(tx *gorm.DB, nickname string) (string, error) {
	var existing data.AgencyNickname
	err := tx.Where("LOWER(nickname) = LOWER(?)", nickname).Order("created_at").First(&existing).Error

	switch {
	case err == nil:
		return existing.AgencyID, nil

	case errors.Is(err, gorm.ErrRecordNotFound):
		// is_default stays false: a brand new agency conjured from a message
		// must never become the fallback for every other issuance.
		agency := data.Agency{Name: nickname, IsDefault: false}
		if err := tx.Create(&agency).Error; err != nil {
			return "", err
		}
		if err := tx.Create(&data.AgencyNickname{AgencyID: agency.ID, Nickname: nickname}).Error; err != nil {
			return "", err
		}
		return agency.ID, nil

	default:
		return "", err
	}
}

// resolveObservation looks the fifth line up in observation_presets. An unknown
// code is an error rather than free text: a mistyped code disappearing silently
// is worse than a message the operator has to send again.
func resolveObservation(tx *gorm.DB, code string) (*string, error) {
	if code == "" {
		return nil, nil
	}

	var preset data.ObservationPreset
	err := tx.Where("LOWER(code) = LOWER(?)", code).First(&preset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, operatorErrorf(err, "%q não é um código de observação cadastrado", code)
	}
	if err != nil {
		return nil, err
	}
	return &preset.Text, nil
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
