package data

import (
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The kinds stored in smiles_statement_lines.kind. They are the same vocabulary as
// azul_statement_lines (kept as literals rather than an import so the data layer stays free
// of the statement package), minus expiration: the Smiles statement has no expiry movement
// in it, so a constant for one would be a promise the source does not keep.
const (
	SmilesLineRedemption = "redemption"
	SmilesLineRefund     = "refund"
	SmilesLineAccrual    = "accrual"
	SmilesLinePurchase   = "purchase"
	SmilesLineOther      = "other"
)

// SmilesKindFromType maps a Smiles movement onto the stored kind. It is the single source of
// truth for that mapping, used both when the sync stores a line and when the statement page
// derives a line's kind for display - so a rule added here takes effect on rows that are
// already stored, without a re-sync.
//
// The inputs are the airline's own words, measured on a live account (see
// backend/docs/smiles-api.md):
//
//   - typeCode "Redemption"               -> redemption. Miles leaving for a booking.
//   - typeCode "Accrual" + subType
//     "Cancellation"                      -> refund. Miles coming back from a cancelled
//     redemption; it is the ONLY way this API spells an estorno, and it arrives as a
//     credit rather than as a negative redemption.
//   - typeCode "Accrual", club or card    -> accrual. "Clube Smiles", "Bônus Clube Smiles
//     Plano 5000", "Cartão Smiles Bradesco Visa Gold" - earned as a membership.
//   - typeCode "Accrual", a flight        -> accrual. Miles earned flying.
//   - typeCode "Accrual", anything else   -> purchase. "Pontos Livelo", "Créditos Inter
//     Loop", "Compra de Milhas PaGol", "Esfera Fidelidade" - buying miles and transferring
//     them in are the same "Compra" to the panel.
//   - anything unrecognised               -> other. Counted in the net so nothing is
//     silently dropped, but not reconciled.
//
// purchase reconciles as a batch credit exactly like accrual (see statement.LoadSmiles); the
// split exists only so the page can label it.
func SmilesKindFromType(typeCode, subType, productName, partnerName string) string {
	switch strings.ToLower(strings.TrimSpace(typeCode)) {
	case "redemption":
		return SmilesLineRedemption
	case "accrual":
		if strings.EqualFold(strings.TrimSpace(subType), "cancellation") {
			return SmilesLineRefund
		}
		context := productName + " " + partnerName
		// mentionsClubOrCard is shared with the Azul mapping on purpose: "is this a club or
		// a co-branded card" is the same question in both programmes, and the words the two
		// airlines use for it are the same words.
		if mentionsClubOrCard(context) || mentionsFlight(productName) {
			return SmilesLineAccrual
		}
		return SmilesLinePurchase
	default:
		return SmilesLineOther
	}
}

// mentionsFlight reports whether the movement is a flight - the airline writes the product
// name of a booking as "VOO". Miles earned flying are an accrual, not a purchase.
func mentionsFlight(productName string) bool {
	return strings.EqualFold(strings.TrimSpace(productName), "voo")
}

// SmilesStatementLine is one synced movement of an account's Smiles statement. See migration
// 63. Unlike internal/statement's parsers, which read a file and drop it, these rows persist:
// the API is polled on a ticker and the panel reads the last stored window between polls.
type SmilesStatementLine struct {
	ID        string   `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID string   `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`

	// OccurredOn is the day the movement happened - the grain the window is keyed and
	// replaced on. OccurredAt stays nil: the payload has a clock, but in a timezone the
	// contract never states, and a timestamp nobody can place is worse than none.
	OccurredOn time.Time  `gorm:"column:occurred_on;type:date;not null" json:"occurred_on"`
	OccurredAt *time.Time `gorm:"column:occurred_at" json:"occurred_at"`

	// Kind is the movement in the panel's terms (see SmilesLine* above). TypeCode, SubType
	// and TypeDescription are the airline's own, kept verbatim.
	Kind            string  `gorm:"column:kind;not null" json:"kind"`
	TypeCode        *string `gorm:"column:type_code" json:"type_code"`
	SubType         *string `gorm:"column:sub_type" json:"sub_type"`
	TypeDescription *string `gorm:"column:type_description" json:"type_description"`

	// Points is signed (negative when miles left the account). The airline sends everything
	// unsigned; the sign is applied by internal/smiles on the way in.
	Points int `gorm:"column:points;not null" json:"points"`

	Locator     *string `gorm:"column:locator" json:"locator"`
	Origin      *string `gorm:"column:origin" json:"origin"`
	Destination *string `gorm:"column:destination" json:"destination"`
	PartnerName *string `gorm:"column:partner_name" json:"partner_name"`
	Description *string `gorm:"column:description" json:"description"`
	Channel     *string `gorm:"column:channel" json:"channel"`

	// DayBalance is the balance the account held at the end of OccurredOn - DERIVED by the sync,
	// not reported by the airline (see migration 63). It is information for the page, and must
	// never be used to validate a window: it is built from the movements, so it always agrees
	// with them.
	DayBalance *int `gorm:"column:day_balance" json:"day_balance"`

	// ExternalID is the airline's own identifier for the movement. It is what a manual link
	// is keyed on, and the reason those links survive a window being replaced.
	ExternalID *string `gorm:"column:external_id" json:"external_id"`

	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (SmilesStatementLine) TableName() string {
	return "smiles_statement_lines"
}

func (l *SmilesStatementLine) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// LineKey is what a manual link (to an issuance or to a batch) is stored against. The
// airline's own identifier is preferred because it survives the window being replaced; the
// composite fallback is for a movement that arrives without one, and is the same shape the
// Azul page uses.
func (l SmilesStatementLine) LineKey() string {
	if l.ExternalID != nil && strings.TrimSpace(*l.ExternalID) != "" {
		return strings.TrimSpace(*l.ExternalID)
	}
	return "sig:" + l.OccurredOn.Format("2006-01-02") + ":" + l.Kind + ":" + strconv.Itoa(l.Points)
}

// ReplaceSmilesStatementWindow swaps the whole window from windowStart forward for one
// account: it deletes every line at or after windowStart and inserts the given lines, in one
// transaction.
//
// The window is replaced rather than merged even though these rows DO carry a stable
// identifier, and that is a deliberate choice rather than an oversight. Replacing is what
// makes a correction on the airline's side propagate: a movement they reclassify, re-date or
// withdraw simply stops being in the window, whereas an upsert would leave our copy of it
// behind forever with nothing to notice. The identifier earns its keep elsewhere - it is what
// a manual link is keyed on, so the links survive the replacement.
//
// Passing no lines is valid and just clears the window (an account whose recent statement is
// genuinely empty).
func ReplaceSmilesStatementWindow(db *gorm.DB, accountID string, windowStart time.Time, lines []SmilesStatementLine) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ? AND occurred_on >= ?", accountID, windowStart).
			Delete(&SmilesStatementLine{}).Error; err != nil {
			return err
		}
		if len(lines) == 0 {
			return nil
		}
		// Force the account id and a fresh row id so a caller cannot insert a line under the
		// wrong account or collide with a deleted row's id.
		for i := range lines {
			lines[i].AccountID = accountID
			lines[i].ID = ""
		}
		return tx.Create(&lines).Error
	})
}
