package sale

import (
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/murandido/m-80-backend/internal/data"
)

// LinkPartnerClient links the sale of an issuance to its issuer's client when the issuer is a
// partner. A partner issues on their own behalf, so the sale is always theirs to pay - typing
// that client on every row was busywork the operator had to remember.
//
// It never overwrites: a sale that already has a client keeps it, so a choice made by hand
// (or an earlier partner's) is not undone by a later change of issuer. The client is looked up
// by name in the issuance's agency - a client belongs to one agency - and created there when
// it does not exist yet.
//
// Returns the ids of the sales whose balance moved: the linked sale, plus any the client's
// credit was spread onto. Empty when nothing was linked.
func LinkPartnerClient(tx *gorm.DB, issuanceID string) ([]string, error) {
	var issuance data.Issuance
	if err := tx.Select("id", "issuer_id", "agency_id").
		First(&issuance, "id = ?", issuanceID).Error; err != nil {
		return nil, err
	}

	var issuer data.User
	err := tx.Select("id", "name", "is_partner", "partner_client_name").
		First(&issuer, "id = ?", issuance.IssuerID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !issuer.IsPartner {
		return nil, nil
	}

	var linked data.Sale
	err = tx.Select("id", "client_id").First(&linked, "issuance_id = ?", issuanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if linked.ClientID != nil {
		return nil, nil
	}

	client, err := partnerClient(tx, issuer, issuance.AgencyID)
	if err != nil {
		return nil, err
	}
	if err := tx.Model(&data.Sale{}).Where("id = ?", linked.ID).
		Update("client_id", client.ID).Error; err != nil {
		return nil, err
	}

	// The client may hold credit from earlier lump sums; replaying them is what makes it pay
	// this sale too, exactly as linking the client by hand does.
	changed, err := RebalanceClient(tx, client.ID)
	if err != nil {
		return nil, err
	}
	return append([]string{linked.ID}, changed...), nil
}

// PartnerClientName is the client a partner's sales go to: the name set on the user, or the
// user's own name when none was, stored the way every client name is - trimmed, upper case.
func PartnerClientName(user data.User) string {
	name := user.Name
	if user.PartnerClientName != nil && strings.TrimSpace(*user.PartnerClientName) != "" {
		name = *user.PartnerClientName
	}
	return strings.ToUpper(strings.TrimSpace(name))
}

func partnerClient(tx *gorm.DB, issuer data.User, agencyID string) (data.Client, error) {
	name := PartnerClientName(issuer)
	var client data.Client
	err := tx.Where("agency_id = ? AND name = ?", agencyID, name).First(&client).Error
	if err == nil {
		return client, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return data.Client{}, err
	}
	client = data.Client{Name: name, AgencyID: &agencyID}
	if err := tx.Create(&client).Error; err != nil {
		return data.Client{}, err
	}
	return client, nil
}
