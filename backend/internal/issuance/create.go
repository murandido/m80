// Package issuance holds the business rules for creating an issuance, shared by
// the two processes that can create one: the REST API serving the panel
// (cmd/api) and the WhatsApp bot (cmd/bot). It is deliberately free of any HTTP
// or WhatsApp concept - callers translate its typed errors into whichever medium
// they answer in.
package issuance

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/murandido/m-80-backend/internal/account"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// auditActionCreated mirrors the action recorded by the panel's own audit log
// entries, so an issuance born on WhatsApp is indistinguishable from one born on
// the grid when the history is read back.
const auditActionCreated = "created"

// postgresUniqueViolation is the Postgres error code for a unique constraint
// violation - here, always the partial unique index on active program locators.
const postgresUniqueViolation = "23505"

// CreateInput carries everything needed to create an issuance. It mirrors
// schemas.CreateIssuanceRequest but in already-parsed form: FlightDate is a real
// time.Time rather than a "YYYY-MM-DD" string, since parsing a wire format is
// the caller's job, not this package's.
type CreateInput struct {
	LoyaltyProgramID    string
	ProgramLocator      string
	AirlineLocator      *string
	LatamPurchaseNumber *string
	MainPassengerName   *string
	TripType            *string
	OriginIATACode      *string
	DestinationIATACode *string
	Points              int
	ValueOfTheThousand  int
	Taxes               *int
	AccountID           string
	AccountNicknameID   *string
	Observation         *string
	AdditionalServices  datatypes.JSON

	// AirlineID nil falls back to the loyalty program's default_airline_id.
	AirlineID *string
	// AgencyID nil falls back to the agency flagged is_default.
	AgencyID *string
	// IssuerID nil falls back to actorID - i.e. whoever performed the action is
	// also credited as the issuer unless someone else is named explicitly.
	IssuerID *string
	// FlightDate, when set, creates the primary outbound leg's departure date.
	FlightDate *time.Time

	// Complete marks the issuance as finished on creation. The panel always
	// leaves it false (rows are completed later, from the grid); the bot may set
	// it once every required field arrived in the same message.
	Complete bool

	// CreatedAt backdates the issuance. Only the spreadsheet importer
	// (internal/legacyimport) sets it, and only ever to the day of the sheet the
	// row came from: created_at is what the grid and every date filter read as
	// "the day of the issuance", so an August row stamped with now() would show up
	// as today's and vanish from its own day. Nil everywhere else, which leaves the
	// timestamp to the database.
	CreatedAt *time.Time
	// Imported records that this issuance came from that spreadsheet. It is what
	// keeps Traveos from being asked to *issue* the booking - see findOrEmit - and
	// unlike the importer itself, it stays after the migration is over.
	Imported bool
}

// Create persists an issuance together with the two rows that must always
// accompany it - an open sale and a "created" audit log - and, optionally, the
// primary outbound flight leg.
//
// db may be a plain *gorm.DB or a transaction already in progress: GORM turns
// the nested Transaction call into a SAVEPOINT, so a caller that needs to create
// supporting rows atomically alongside the issuance (the bot auto-creating
// accounts from nicknames) can hand its own transaction straight in.
func Create(db *gorm.DB, in CreateInput, actorID string) (data.Issuance, error) {
	issuance := data.Issuance{
		LoyaltyProgramID:    in.LoyaltyProgramID,
		ProgramLocator:      in.ProgramLocator,
		AirlineLocator:      in.AirlineLocator,
		LatamPurchaseNumber: in.LatamPurchaseNumber,
		MainPassengerName:   in.MainPassengerName,
		TripType:            in.TripType,
		OriginIATACode:      in.OriginIATACode,
		DestinationIATACode: in.DestinationIATACode,
		Points:              in.Points,
		ValueOfTheThousand:  in.ValueOfTheThousand,
		Taxes:               in.Taxes,
		AccountID:           in.AccountID,
		AccountNicknameID:   in.AccountNicknameID,
		Observation:         in.Observation,
		AdditionalServices:  in.AdditionalServices,
		IssuerID:            actorID,
		Complete:            in.Complete,
		Imported:            in.Imported,
	}

	// GORM only stamps CreatedAt when it is still the zero value, so setting it here
	// is all backdating takes.
	if in.CreatedAt != nil {
		issuance.CreatedAt = *in.CreatedAt
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := ValidateAccountNickname(tx, in.AccountID, in.AccountNicknameID); err != nil {
			return err
		}

		airline, err := resolveAirline(tx, in)
		if err != nil {
			return err
		}
		issuance.AirlineID = airline.ID

		// Only when the issuance is born already complete - an incomplete one is
		// expected to have gaps. See ValidateAirlineRequirements.
		if in.Complete {
			if err := ValidateAirlineRequirements(airline, in.MainPassengerName, in.LatamPurchaseNumber); err != nil {
				return err
			}
		}

		if err := ValidateAirportCodes(tx, in.OriginIATACode, in.DestinationIATACode); err != nil {
			return err
		}

		if issuance.AgencyID, err = resolveAgencyID(tx, in.AgencyID); err != nil {
			return err
		}
		if in.IssuerID != nil {
			var issuer data.User
			if err := tx.First(&issuer, "id = ?", *in.IssuerID).Error; err != nil {
				return ErrIssuerNotFound
			}
			issuance.IssuerID = *in.IssuerID
		}

		if err := tx.Create(&issuance).Error; err != nil {
			return err
		}

		// The loyalty programs cap how many people an account may issue for, so every
		// issuance consumes something the operator has to be able to see. At this point
		// all we have is the name - the document, when there is one, arrives with the
		// Traveos sync and is attached to the same beneficiary then.
		if in.MainPassengerName != nil {
			if err := account.Register(
				tx, issuance.AccountID, issuance.ID, *in.MainPassengerName, nil, issuance.CreatedAt,
			); err != nil {
				return err
			}
		}

		// Every issuance is born owing money, so its sale is created in the same
		// transaction rather than when the issuance is marked complete: the Vendas tab
		// is the only place that tracks who still has to be charged, and an issuance
		// with no sale row would simply never surface there.
		if err := tx.Create(&data.Sale{IssuanceID: issuance.ID}).Error; err != nil {
			return err
		}

		if in.FlightDate != nil {
			if err := UpsertPrimaryOutboundFlight(tx, issuance.ID, *in.FlightDate); err != nil {
				return err
			}
		}

		return tx.Create(&data.AuditLog{
			IssuanceID:  issuance.ID,
			ChangedByID: actorID,
			Action:      auditActionCreated,
		}).Error
	})

	if err != nil {
		return data.Issuance{}, TranslateWriteError(err)
	}

	return issuance, nil
}

// resolveAirline returns the airline the issuance belongs to - the one named
// explicitly, or the loyalty program's default. The full row is returned rather
// than just the id because the completion check needs its requires_* flags.
func resolveAirline(tx *gorm.DB, in CreateInput) (data.Airline, error) {
	var airline data.Airline

	if in.AirlineID != nil {
		if err := tx.First(&airline, "id = ?", *in.AirlineID).Error; err != nil {
			return data.Airline{}, err
		}
		return airline, nil
	}

	var loyaltyProgram data.LoyaltyProgram
	if err := tx.First(&loyaltyProgram, "id = ?", in.LoyaltyProgramID).Error; err != nil {
		return data.Airline{}, ErrLoyaltyProgramNotFound
	}
	if err := tx.First(&airline, "id = ?", loyaltyProgram.DefaultAirlineID).Error; err != nil {
		return data.Airline{}, err
	}
	return airline, nil
}

func resolveAgencyID(tx *gorm.DB, agencyID *string) (string, error) {
	if agencyID != nil {
		return *agencyID, nil
	}

	var defaultAgency data.Agency
	if err := tx.Where("is_default = ?", true).First(&defaultAgency).Error; err != nil {
		return "", ErrNoDefaultAgency
	}
	return defaultAgency.ID, nil
}

// TranslateWriteError maps the one database error the callers care about - the
// partial unique index on active program locators - onto a sentinel they can
// match. Anything else is returned untouched so a genuine infrastructure failure
// keeps its original message.
func TranslateWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation {
		return ErrProgramLocatorTaken
	}
	return err
}

// UpsertPrimaryOutboundFlight creates or updates the leg_order=1 outbound leg with the
// only field this simplified UI collects - the departure date. flight_number/arrival_at/etc.
// are left nil rather than filled with placeholder data. Shared by create and update so
// a date typed before the issuance exists is persisted the same way as one edited after.
func UpsertPrimaryOutboundFlight(tx *gorm.DB, issuanceID string, departureAt time.Time) error {
	var flight data.IssuanceFlight
	err := tx.Where("issuance_id = ? AND direction = ? AND leg_order = ?", issuanceID, "outbound", 1).
		First(&flight).Error

	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		flight = data.IssuanceFlight{
			IssuanceID:  issuanceID,
			Direction:   "outbound",
			LegOrder:    1,
			DepartureAt: departureAt,
		}
		return tx.Create(&flight).Error
	case err != nil:
		return err
	default:
		flight.DepartureAt = departureAt
		return tx.Save(&flight).Error
	}
}

// ValidateAccountNickname checks that the chosen alias actually belongs to the chosen
// account. Both ids arrive from the client independently, so without this a typo (or a
// stale grid row) could persist an issuance labelled with another account's alias.
// A nil nicknameID is valid: accounts with no registered alias are still issuable.
func ValidateAccountNickname(db *gorm.DB, accountID string, nicknameID *string) error {
	if nicknameID == nil {
		return nil
	}

	var nickname data.AccountNickname
	if err := db.First(&nickname, "id = ?", *nicknameID).Error; err != nil {
		return ErrAccountNicknameNotFound
	}
	if nickname.AccountID != accountID {
		return ErrAccountNicknameMismatch
	}
	return nil
}
