package data

import (
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// The kinds stored in azul_statement_lines.kind. The first three mirror internal/statement.Kind
// exactly (kept as literals rather than an import so the data layer stays free of the statement
// package); the last two are Azul-specific.
const (
	AzulLineRedemption = "redemption"
	AzulLineRefund     = "refund"
	AzulLineAccrual    = "accrual"
	// AzulLinePurchase is a point acquisition (type ER "Sale") - buying or transferring points in
	// from a bank or partner, and campaign/promotion credits, which are the same thing. It is a
	// point entry like an accrual and reconciles the same way (see azulLineKind), kept as its own
	// kind so the statement page can label it "Compra" instead of lumping it with flight accrual.
	AzulLinePurchase = "purchase"
	// AzulLineExpiration is a point-expiration movement (type PE). It is stored and shown in
	// reconciliation but deliberately kept OUT of the FIFO costing for now - see the sync.
	AzulLineExpiration = "expiration"
	// AzulLineOther is any movement whose type code the mapping does not recognise. Stored so
	// nothing is silently dropped, and so an unmapped type shows up in reconciliation.
	AzulLineOther = "other"
)

// AzulKindFromTypeCode maps an Azul statement type code onto the stored kind. It is the single
// source of truth for that mapping, used both when the sync stores a line and when the statement
// page derives a line's kind for display. context is the movement's partner name and type
// description joined (to tell a plain purchase from a club/card accrual); comment is the airline's
// free-text note (to tell a promotional entry from an ordinary one).
//
// The codes are the ones measured on live accounts:
//   - AN  Direct redemption          -> redemption
//   - AI  Airline Issuance           -> accrual (points earned on a flight)
//   - RF  Redemption Refund          -> refund (the estorno of a cancelled redemption)
//   - PE  Point expiration           -> expiration (kept out of the FIFO for now)
//   - ER  Sale                       -> accrual for a club/card partner (Clube TudoAzul earns as a
//     membership); else purchase - buying points, a partner transfer and a campaign credit are all
//     the same "Compra"
//   - PC  Points Corrections         -> purchase when the comment names a promotion (a correction
//     of one, the same as buying points back in); else other
//   - OI  Aceite de Campanha Promo.  -> other (a zero-point enrolment with no comment), and
//     anything unrecognised
//
// purchase reconciles as a batch credit exactly like accrual (see azulLineKind); it is a separate
// kind only so the page can label it. "other" is counted in the net so nothing is silently
// dropped, but is not reconciled.
func AzulKindFromTypeCode(code, context, comment string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "AN":
		return AzulLineRedemption
	case "AI":
		return AzulLineAccrual
	case "RF":
		return AzulLineRefund
	case "PE":
		return AzulLineExpiration
	case "ER":
		if mentionsClubOrCard(context) {
			return AzulLineAccrual
		}
		return AzulLinePurchase
	case "PC":
		if mentionsPromotion(comment) {
			return AzulLinePurchase
		}
		return AzulLineOther
	default:
		return AzulLineOther
	}
}

// mentionsPromotion reports whether the airline's comment names a promotion or campaign, matched
// case-insensitively on the accented and unaccented spellings the field uses. It reads the
// comment, not the type description, on purpose: the OI enrolment's own description is "Aceite de
// Campanha Promocional" but its comment is empty, so it stays "other" as the operator asked. It
// is what promotes a points correction that fixes a promotion into a "Compra".
func mentionsPromotion(comment string) bool {
	lowered := strings.ToLower(comment)
	for _, needle := range []string{"promoção", "promocao", "campanha"} {
		if strings.Contains(lowered, needle) {
			return true
		}
	}
	return false
}

// mentionsClubOrCard reports whether the movement's partner/description names an Azul club or a
// co-branded card, which earns points as a membership (an accrual) rather than a one-off
// purchase. Matched case-insensitively on the accented and unaccented spellings the field uses.
func mentionsClubOrCard(context string) bool {
	lowered := strings.ToLower(context)
	for _, needle := range []string{"clube", "cartão", "cartao", "card"} {
		if strings.Contains(lowered, needle) {
			return true
		}
	}
	return false
}

// AzulStatementLineOrder is how stored lines are read back: by day, then by where each sat within
// its day. Rows synced before migration 67 have no position; Postgres sorts those NULLs last and
// they tie-break on id, the old (arbitrary) order, until a sync rewrites them.
const AzulStatementLineOrder = "occurred_on ASC, position_in_day ASC, id ASC"

// AzulStatementLine is one synced movement of an account's Azul statement. See migration 55.
// Unlike internal/statement (which reads a file and drops it), these rows persist: the API is
// polled on a ticker and the panel reads the last stored window between polls.
type AzulStatementLine struct {
	ID        string   `gorm:"primaryKey;type:uuid" json:"id"`
	AccountID string   `gorm:"column:account_id;type:uuid;not null" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID;references:ID" json:"account,omitempty"`

	// OccurredOn is the day the movement happened - the grain the window is keyed and replaced
	// on. OccurredAt is left nil until a source with a real clock exists (the legacy endpoint
	// gives day precision only).
	OccurredOn time.Time  `gorm:"column:occurred_on;type:date;not null" json:"occurred_on"`
	OccurredAt *time.Time `gorm:"column:occurred_at" json:"occurred_at"`

	// Kind is the movement in the panel's terms (see AzulLine* above). TypeCode/TypeDescription
	// are the airline's own, kept verbatim.
	Kind            string  `gorm:"column:kind;not null" json:"kind"`
	TypeCode        *string `gorm:"column:type_code" json:"type_code"`
	TypeDescription *string `gorm:"column:type_description" json:"type_description"`

	// Points is signed (negative when points left the account).
	Points int `gorm:"column:points;not null" json:"points"`

	Locator     *string `gorm:"column:locator" json:"locator"`
	Origin      *string `gorm:"column:origin" json:"origin"`
	Destination *string `gorm:"column:destination" json:"destination"`
	PartnerName *string `gorm:"column:partner_name" json:"partner_name"`
	Description *string `gorm:"column:description" json:"description"`
	// Comment is the airline's free-text note, kept verbatim so the page can tell a promotional
	// entry from an ordinary one. Nil on rows synced before migration 60.
	Comment *string `gorm:"column:comment" json:"comment"`

	// DayBalance is the balance the airline reported for OccurredOn, kept verbatim. It is only
	// consistent within the response it came in (see azul.Transaction.DayBalance), so the page
	// derives its balances instead (azulsync.ClosingBalances); the latest day's value is the
	// absolute one the sync validates against points/resume before storing the window.
	DayBalance *int `gorm:"column:day_balance" json:"day_balance"`

	// PositionInDay is where the movement sat within its day, 0 being the day's oldest. The
	// airline gives no clock, but returns each day newest first, so that position is the only
	// real order within a day. Nil on rows synced before migration 67.
	PositionInDay *int `gorm:"column:position_in_day" json:"position_in_day"`

	CreatedAt time.Time `gorm:"column:created_at" json:"created_at"`
}

func (AzulStatementLine) TableName() string {
	return "azul_statement_lines"
}

func (l *AzulStatementLine) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = uuid.New().String()
	}
	return nil
}

// ReplaceAzulStatementWindow swaps the whole window from windowStart forward for one account:
// it deletes every line at or after windowStart and inserts the given lines, in one
// transaction. This is the storage model the airline's data forces - transactionId is always
// 0, so there is no key to merge on, and identical rows on one day are legitimately distinct.
// Replacing the window wholesale is what makes that safe: the fresh statement is the truth for
// its own period, and anything older than windowStart is left untouched.
//
// Passing no lines is valid and just clears the window (an account whose recent statement is
// genuinely empty).
func ReplaceAzulStatementWindow(db *gorm.DB, accountID string, windowStart time.Time, lines []AzulStatementLine) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("account_id = ? AND occurred_on >= ?", accountID, windowStart).
			Delete(&AzulStatementLine{}).Error; err != nil {
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
