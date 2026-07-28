package bot

import (
	"strings"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

// alteracaoFields is what a correction message asks to change, already resolved
// against the database. Every field is a pointer and nil means "the message said
// nothing about this" - not "clear it". The one exception is the route block:
// see HasRoute.
type alteracaoFields struct {
	// Full marks a correction that restates the entire message instead of only
	// the lines that changed. Only an edit of the original EMISSAO can: it
	// replaces the whole text, so a field missing from it was deleted on purpose
	// and gets cleared. In a partial correction the same nil only means the
	// message stayed silent about it.
	Full bool

	// The three header fields only ever arrive from an edit - an ALTERACAO's
	// line 1 is the locator, and locates rather than corrects.
	LoyaltyProgramID *string
	AirlineID        *string
	AirlineLocator   *string

	Points             *int
	ValueOfTheThousand *int
	Taxes              *int

	// HasRoute records that the message carried a route line at all, which the
	// three fields below cannot express on their own: only the origin is always
	// present, and a nil surname is a legitimate outcome of parsing one.
	//
	// What that nil means then follows Full, like every other field here. In a
	// restatement it clears the surname; in an ALTERACAO the line names only what
	// is changing, so "GRU" after "GRU - SILVA" moves the origin and keeps SILVA.
	HasRoute            bool
	OriginIATACode      *string
	MainPassengerName   *string
	LatamPurchaseNumber *string

	AccountID         *string
	AccountNicknameID *string
	AgencyID          *string

	Observation *string
}

// resolveAlteracao turns the classified lines into resolved field values,
// looking every code and alias up in the database.
//
// Like resolveEmissao it must run inside a transaction, and for the same
// reason: it is not read-only. An account alias nobody registered yet creates
// the account, the person behind it and the alias itself, and none of that may
// outlive a message that fails further down.
func resolveAlteracao(tx *gorm.DB, msg *alteracaoMessage, issuance data.Issuance) (alteracaoFields, error) {
	var fields alteracaoFields

	settled, err := settleAlteracaoLines(tx, msg.Corrections)
	if err != nil {
		return fields, err
	}

	for _, line := range settled {
		if err := resolveAlteracaoLine(tx, line, issuance, &fields); err != nil {
			return fields, err
		}
	}
	return fields, nil
}

// settleAlteracaoLines gives the two provisional kinds their final answer and
// rejects a message that says the same thing twice.
//
// The duplicate check has to happen here rather than in the parser: a line the
// parser called a route may turn out to be an account line, and only then does
// it collide with the account line that was already unambiguous.
func settleAlteracaoLines(tx *gorm.DB, corrections []alteracaoLine) ([]alteracaoLine, error) {
	settled := make([]alteracaoLine, 0, len(corrections))
	seen := make(map[alteracaoLineKind]string, len(corrections))

	for _, line := range corrections {
		kind, err := settleAlteracaoLineKind(tx, line)
		if err != nil {
			return nil, err
		}

		if previous, duplicated := seen[kind]; duplicated {
			return nil, operatorErrorf(nil,
				"A mensagem traz duas linhas de %s: %q e %q", kind, previous, line.Text)
		}
		seen[kind] = line.Text

		settled = append(settled, alteracaoLine{Text: line.Text, Kind: kind})
	}
	return settled, nil
}

// settleAlteracaoLineKind resolves the shapes the parser could not. A
// three-letter word is an airport if the airports table says so, an observation
// code if observation_presets does, and an account alias otherwise - which is
// also the fallback for a bare word that matches no preset.
func settleAlteracaoLineKind(tx *gorm.DB, line alteracaoLine) (alteracaoLineKind, error) {
	fields := splitTrimmed(line.Text, "-")

	switch line.Kind {
	case lineRoute:
		found, err := airportExists(tx, fields[0])
		if err != nil || found {
			return lineRoute, err
		}

		// Not an airport. A three-letter word standing alone could still be a
		// preset code before it is an account alias.
		if len(fields) == 1 && !strings.ContainsAny(line.Text, "()") {
			return settleBareWord(tx, fields[0])
		}
		return lineOwners, nil

	case lineObservation:
		return settleBareWord(tx, fields[0])

	default:
		return line.Kind, nil
	}
}

func settleBareWord(tx *gorm.DB, word string) (alteracaoLineKind, error) {
	found, err := observationPresetExists(tx, word)
	if err != nil {
		return lineOwners, err
	}
	if found {
		return lineObservation, nil
	}
	return lineOwners, nil
}

func airportExists(tx *gorm.DB, code string) (bool, error) {
	var count int64
	err := tx.Model(&data.Airport{}).Where("LOWER(iata_code) = LOWER(?)", code).Count(&count).Error
	return count > 0, err
}

func observationPresetExists(tx *gorm.DB, code string) (bool, error) {
	var count int64
	err := tx.Model(&data.ObservationPreset{}).Where("LOWER(code) = LOWER(?)", code).Count(&count).Error
	return count > 0, err
}

// resolveAlteracaoLine reads one settled line into fields. Each kind is parsed
// by the very function that parses the equivalent EMISSAO line, so a number, an
// airport code or an alias means exactly the same thing in both messages - and
// a change to the format only has one place to be made.
func resolveAlteracaoLine(tx *gorm.DB, line alteracaoLine, issuance data.Issuance, fields *alteracaoFields) error {
	var draft emissaoMessage

	switch line.Kind {
	case lineAmounts:
		if err := parseEmissaoAmounts(line.Text, &draft); err != nil {
			return err
		}
		// The "= total" checksum guards a correction exactly as it guards an
		// issuance: a separator typed one place off is still a valid number, and
		// the sum is the only thing that catches it.
		if err := validateTotal(&draft); err != nil {
			return err
		}
		fields.Points = &draft.Points
		fields.ValueOfTheThousand = &draft.ValueOfTheThousand
		fields.Taxes = &draft.Taxes
		return nil

	case lineRoute:
		if err := parseEmissaoRoute(line.Text, &draft); err != nil {
			return err
		}

		// Which of the extras is the surname and which is the purchase number
		// depends on the airline's flags - the same airline the issuance already
		// belongs to, since a correction cannot change it.
		var airline data.Airline
		if err := tx.First(&airline, "id = ?", issuance.AirlineID).Error; err != nil {
			return err
		}
		lastName, purchaseNumber, err := assignCorrectionExtras(airline, draft.Line2Extras)
		if err != nil {
			return err
		}

		// No ValidateAirportCodes here: settleAlteracaoLineKind only calls a line
		// a route once the airports table has confirmed the code.
		fields.HasRoute = true
		fields.OriginIATACode = &draft.OriginIATACode
		fields.MainPassengerName = lastName
		fields.LatamPurchaseNumber = purchaseNumber
		return nil

	case lineOwners:
		if err := parseEmissaoOwners(line.Text, &draft); err != nil {
			return err
		}

		account, nicknameID, err := resolveAccountByNickname(tx, issuance.LoyaltyProgramID, draft.AccountNickname)
		if err != nil {
			return err
		}
		if draft.AdminNickname != "" {
			if err := attachAdministrator(tx, &account, draft.AccountNickname, draft.AdminNickname); err != nil {
				return err
			}
		}
		if draft.AgencyNickname != "" {
			agencyID, err := resolveAgencyByNickname(tx, draft.AgencyNickname)
			if err != nil {
				return err
			}
			fields.AgencyID = &agencyID
		}

		fields.AccountID = &account.ID
		fields.AccountNicknameID = nicknameID
		return nil

	default:
		observation, err := resolveObservation(tx, line.Text)
		if err != nil {
			return err
		}
		fields.Observation = observation
		return nil
	}
}
