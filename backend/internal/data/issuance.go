package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Issuance struct {
	ID                  string          `gorm:"primaryKey;type:uuid" json:"id"`
	LoyaltyProgramID    string          `gorm:"column:loyalty_program_id;type:uuid;not null" json:"loyalty_program_id"`
	LoyaltyProgram      *LoyaltyProgram `gorm:"foreignKey:LoyaltyProgramID;references:ID" json:"loyalty_program,omitempty"`
	AirlineID           string          `gorm:"column:airline_id;type:uuid;not null" json:"airline_id"`
	Airline             *Airline        `gorm:"foreignKey:AirlineID;references:ID" json:"airline,omitempty"`
	ProgramLocator      string          `gorm:"column:program_locator;not null" json:"program_locator"`
	AirlineLocator      *string         `gorm:"column:airline_locator" json:"airline_locator"`
	LatamPurchaseNumber *string         `gorm:"column:latam_purchase_number" json:"latam_purchase_number"`
	MainPassengerName   *string         `gorm:"column:main_passenger_name" json:"main_passenger_name"`
	// PassengerLastName is the surname typed for the airline's lookup. Separate from
	// MainPassengerName, which the sync overwrites with the full name - see
	// BookingLastName.
	PassengerLastName   *string  `gorm:"column:passenger_last_name" json:"passenger_last_name"`
	TripType            *string  `gorm:"column:trip_type;type:trip_type" json:"trip_type"`
	OriginIATACode      *string  `gorm:"column:origin_iata_code" json:"origin_iata_code"`
	OriginAirport       *Airport `gorm:"foreignKey:OriginIATACode;references:IATACode" json:"origin_airport,omitempty"`
	DestinationIATACode *string  `gorm:"column:destination_iata_code" json:"destination_iata_code"`
	DestinationAirport  *Airport `gorm:"foreignKey:DestinationIATACode;references:IATACode" json:"destination_airport,omitempty"`
	Points              int      `gorm:"not null" json:"points"`
	ValueOfTheThousand  int      `gorm:"column:value_of_the_thousand;not null" json:"value_of_the_thousand"`
	Taxes               *int     `json:"taxes"`
	// CostOfTheThousand is the buy side of the same trade ValueOfTheThousand is the
	// sell side of: what the agency pays the account's administrator per thousand
	// miles, in centavos. Nil means the cost has not been agreed yet, which is not
	// the same as zero - the supplier payment summary refuses to generate on a nil
	// rather than billing the supplier short.
	//
	// It is deliberately absent from IssuanceResponse and UpdateIssuanceRequest: what
	// the agency pays a supplier is admin-only, and the issuances grid is an operator
	// screen. It is read and written exclusively through /sales/suppliers, which is
	// behind RequireAdmin.
	CostOfTheThousand *int `gorm:"column:cost_of_the_thousand" json:"cost_of_the_thousand"`
	// TaxesIncludedInCost says whether the taxes are part of what is paid for this
	// issuance. Per issuance because it is per deal: some administrators are reimbursed
	// the taxes they paid the airline and some are not, and one account can have both.
	// Admin-only for the same reason CostOfTheThousand is.
	TaxesIncludedInCost bool `gorm:"column:taxes_included_in_cost;not null;default:false" json:"taxes_included_in_cost"`
	// Reviewed records that somebody already checked this line while reconciling what the
	// agency owes the supplier. It is a control marking and not a payment: the sell side
	// settles through sales/sale_payments, the supplier side has no ledger, and the two are
	// meant to be able to disagree in both directions. Admin-only, like the two above.
	Reviewed bool `gorm:"column:reviewed;not null;default:false" json:"reviewed"`
	// RedemptionFeeSavedCents is the airline's redemption fee this issuance did NOT pay
	// because the account it was made on is exempt - Azul's Diamante Unique. It is what
	// prices the decision of issuing there rather than on an ordinary account.
	//
	// Nil on every ordinary account, and nil whenever the rule could not be applied (no
	// synced itinerary, or a program with no fee configured). Deliberately not zero: zero
	// would claim the rule ran and found nothing to save.
	//
	// What an ordinary account actually paid the airline is not recorded anywhere: it is
	// settled outside this system and has no reader here. This column itself is a saving and
	// does not enter sale.PointsValueCents; the fee reaches what the client owes through Taxes,
	// where issuance.RecalculateRedemptionFee folds it once on an exempt, synced issuance.
	RedemptionFeeSavedCents *int `gorm:"column:redemption_fee_saved_cents" json:"redemption_fee_saved_cents"`
	// RedemptionFeeManual is set the moment an operator types the amount by hand, and is
	// what stops the next Traveos sync from overwriting the correction. Clearing the cell
	// turns it off and hands the issuance back to the rule.
	RedemptionFeeManual bool     `gorm:"column:redemption_fee_manual;not null;default:false" json:"redemption_fee_manual"`
	AccountID           string   `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account             *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`
	// AccountNicknameID records which of the account's aliases the operator picked.
	// It is a FK rather than a copied string so renaming an alias propagates to past
	// issuances instead of leaving them showing a stale name.
	AccountNicknameID *string          `gorm:"column:account_nickname_id;type:uuid" json:"account_nickname_id"`
	AccountNickname   *AccountNickname `gorm:"foreignKey:AccountNicknameID;references:ID" json:"account_nickname,omitempty"`
	IssuerID          string           `gorm:"column:issuer_id;type:uuid;not null" json:"issuer_id"`
	Issuer            *User            `gorm:"foreignKey:IssuerID;references:ID" json:"issuer,omitempty"`
	AgencyID          string           `gorm:"column:agency_id;type:uuid;not null" json:"agency_id"`
	Agency            *Agency          `gorm:"foreignKey:AgencyID;references:ID" json:"agency,omitempty"`
	Complete          bool             `gorm:"not null;default:false" json:"complete"`
	// TraveosSyncedAt is when the itinerary was last pulled from Traveos. It is
	// what separates "complete because the airline confirmed it" from "complete
	// because someone ticked it on the grid" - only the former has passengers and
	// flights that came from the source, and only the former can render a ticket.
	TraveosSyncedAt *time.Time `gorm:"column:traveos_synced_at" json:"traveos_synced_at"`
	// TraveosSyncingAt is set while a pull is in flight and cleared when it ends,
	// failure included. It is on the row rather than in the browser that started
	// the pull so every panel shows the same issuance as busy - and so leaving the
	// screen and coming back does not lose that it still is.
	TraveosSyncingAt *time.Time `gorm:"column:traveos_syncing_at" json:"traveos_syncing_at"`
	// TraveosAttemptedAt is when the automatic lookup last ran, whether or not it
	// found anything. TraveosSyncedAt only records success, and "we already looked
	// and it is not there" is the other half - without it the panel would ask again
	// on every page load, which for an imported day is hundreds of calls a refresh.
	TraveosAttemptedAt *time.Time `gorm:"column:traveos_attempted_at" json:"traveos_attempted_at"`
	// BookingSource names the provider whose answer produced the itinerary on this
	// row - "traveos", "azul", and so on. Nil on every row written before the
	// providers existed, which is not the same as "unknown provider".
	//
	// It is what the migration off Traveos is judged on: an airline whose direct
	// integration is quietly falling back to the vendor on every lookup looks
	// exactly like one that is working, until this column is counted.
	BookingSource *string `gorm:"column:booking_source" json:"booking_source"`
	// Imported marks an issuance that came from the pre-panel spreadsheet
	// (internal/legacyimport). The booking was made outside the system and long
	// before the row existed, so Traveos may be searched for it but must never be
	// asked to issue it - see fetchOrImport. The importer goes away after the
	// migration; this column does not, because the rows it describes stay.
	Imported   bool                `gorm:"not null;default:false" json:"imported"`
	Flights    []IssuanceFlight    `gorm:"foreignKey:IssuanceID;references:ID" json:"flights,omitempty"`
	Passengers []IssuancePassenger `gorm:"foreignKey:IssuanceID;references:ID" json:"passengers,omitempty"`
	// Adjustments are the alterations made to the ticket after it was issued. Points and
	// Taxes above stay what the booking cost when it was made.
	Adjustments        []IssuanceAdjustment `gorm:"foreignKey:IssuanceID;references:ID" json:"adjustments,omitempty"`
	AdditionalServices datatypes.JSON       `gorm:"column:additional_services;type:jsonb" json:"additional_services"`
	Observation        *string              `gorm:"type:text" json:"observation"`
	Active             bool                 `gorm:"not null;default:true" json:"active"`
	CancelledAt        *time.Time           `gorm:"column:cancelled_at" json:"cancelled_at"`
	CancelledByID      *string              `gorm:"column:cancelled_by_id;type:uuid" json:"cancelled_by_id"`
	CancelledBy        *User                `gorm:"foreignKey:CancelledByID;references:ID" json:"cancelled_by,omitempty"`
	RefundedPoints     *int                 `gorm:"column:refunded_points" json:"refunded_points"`
	RefundedTaxes      *int                 `gorm:"column:refunded_taxes" json:"refunded_taxes"`
	CancellationFee    *int                 `gorm:"column:cancellation_fee" json:"cancellation_fee"`
	// CancellationOnly marks a row that exists only to carry an estorno. The
	// spreadsheet importer writes one when a negative row names a booking the panel
	// never had: there is nothing to cancel, so it invents an issuance to hang the
	// refund off (internal/legacyimport.commitCancellation). Its points were never
	// miles this agency issued, so the supplier balance counts the refund and not the
	// booking - which is what makes the day read minus the refund, like the sheet.
	//
	// Stored rather than worked out, because it cannot be worked out: an issuance
	// genuinely made and cancelled on the same tab is identical in every column, and
	// that one has to keep netting to zero.
	CancellationOnly bool           `gorm:"not null;default:false" json:"cancellation_only"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

func (i *Issuance) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = uuid.New().String()
	}
	return nil
}

// BookingLastName is the surname the airline is asked with: the one typed for the
// lookup when there is one, else the main passenger's name - which the bot fills with
// just the surname, and the sync with the full name (booking.SurnameCandidates copes
// with both).
func (i Issuance) BookingLastName() *string {
	if i.PassengerLastName != nil && strings.TrimSpace(*i.PassengerLastName) != "" {
		return i.PassengerLastName
	}
	return i.MainPassengerName
}
