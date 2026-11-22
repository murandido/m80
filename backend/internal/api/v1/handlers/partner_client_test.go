package handlers

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/murandido/m-80-backend/internal/sale"
)

// seedPartner creates a partner user. The name carries a random suffix: the client is looked
// up by name in the agency, and parallel tests must not find each other's.
func seedPartner(t *testing.T, tx *gorm.DB, clientName *string) data.User {
	t.Helper()
	partner := createUser(t, tx, "Parceiro "+uuid.New().String()[:8])
	partner.IsPartner = true
	partner.PartnerClientName = clientName
	if err := tx.Save(&partner).Error; err != nil {
		t.Fatalf("failed to mark the partner: %v", err)
	}
	return partner
}

// seedUnlinkedSale is an issuance issued by issuerID with its sale, no client yet.
func seedUnlinkedSale(t *testing.T, tx *gorm.DB, issuerID string) (data.Issuance, data.Sale) {
	t.Helper()
	issuance, _ := seedIssuance(t, tx)
	if err := tx.Model(&issuance).UpdateColumn("issuer_id", issuerID).Error; err != nil {
		t.Fatalf("failed to set the issuer: %v", err)
	}
	issuance.IssuerID = issuerID
	sold := data.Sale{IssuanceID: issuance.ID}
	create(t, tx, &sold)
	return issuance, sold
}

func clientOfSale(t *testing.T, tx *gorm.DB, saleID string) *data.Client {
	t.Helper()
	var linked data.Sale
	if err := tx.First(&linked, "id = ?", saleID).Error; err != nil {
		t.Fatalf("failed to read the sale: %v", err)
	}
	if linked.ClientID == nil {
		return nil
	}
	var client data.Client
	if err := tx.First(&client, "id = ?", *linked.ClientID).Error; err != nil {
		t.Fatalf("failed to read the client: %v", err)
	}
	return &client
}

// A partner's sale goes to their client: created in the issuance's agency, upper case, the
// first time, and reused after - one client per partner per agency, never one per sale.
func TestLinkPartnerClient_CreatesThenReusesTheClient(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		name := "hudson " + uuid.New().String()[:8]
		partner := seedPartner(t, tx, &name)

		first, firstSale := seedUnlinkedSale(t, tx, partner.ID)
		if _, err := sale.LinkPartnerClient(tx, first.ID); err != nil {
			t.Fatalf("LinkPartnerClient: %v", err)
		}
		client := clientOfSale(t, tx, firstSale.ID)
		if client == nil {
			t.Fatal("the partner's sale was left without a client")
		}
		if want := sale.PartnerClientName(partner); client.Name != want {
			t.Errorf("client name = %q, want %q", client.Name, want)
		}
		if client.AgencyID == nil || *client.AgencyID != first.AgencyID {
			t.Errorf("client agency = %v, want the issuance's %s", client.AgencyID, first.AgencyID)
		}

		// A second issuance in the same agency finds the client instead of creating another.
		second, secondSale := seedUnlinkedSale(t, tx, partner.ID)
		if err := tx.Model(&second).UpdateColumn("agency_id", first.AgencyID).Error; err != nil {
			t.Fatalf("failed to move the issuance: %v", err)
		}
		if _, err := sale.LinkPartnerClient(tx, second.ID); err != nil {
			t.Fatalf("LinkPartnerClient: %v", err)
		}
		if again := clientOfSale(t, tx, secondSale.ID); again == nil || again.ID != client.ID {
			t.Errorf("second sale client = %v, want the same client %s", again, client.ID)
		}
	})
}

// Team issuers are left alone, and a client already on the sale is never replaced.
func TestLinkPartnerClient_LeavesTeamAndLinkedSalesAlone(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		team := createUser(t, tx, "Equipe "+uuid.New().String()[:8])
		teamIssuance, teamSale := seedUnlinkedSale(t, tx, team.ID)
		if _, err := sale.LinkPartnerClient(tx, teamIssuance.ID); err != nil {
			t.Fatalf("LinkPartnerClient: %v", err)
		}
		if client := clientOfSale(t, tx, teamSale.ID); client != nil {
			t.Errorf("a team member's sale got client %q", client.Name)
		}

		partner := seedPartner(t, tx, nil)
		chosen := seedClient(t, tx)
		linked := seedClientSale(t, tx, chosen.ID, 1_000, teamIssuance.CreatedAt)
		if err := tx.Exec(`UPDATE issuances SET issuer_id = ?
			WHERE id = (SELECT issuance_id FROM sales WHERE id = ?)`, partner.ID, linked.ID).Error; err != nil {
			t.Fatalf("failed to set the issuer: %v", err)
		}
		if _, err := sale.LinkPartnerClient(tx, linked.IssuanceID); err != nil {
			t.Fatalf("LinkPartnerClient: %v", err)
		}
		if client := clientOfSale(t, tx, linked.ID); client == nil || client.ID != chosen.ID {
			t.Errorf("the client chosen by hand was replaced by %v", client)
		}
	})
}

// Handing an issuance to a partner through the grid links the sale to them, the same as if
// the partner had created it.
func TestUpdateIssuance_IssuerChangeToPartnerLinksTheSale(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		team := createUser(t, tx, "Equipe "+uuid.New().String()[:8])
		issuance, sold := seedUnlinkedSale(t, tx, team.ID)
		partner := seedPartner(t, tx, nil)

		rec := callPatchIssuance(tx, issuance.ID, team.ID, `{"issuer_id":"`+partner.ID+`"}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("PATCH status %d: %s", rec.Code, rec.Body.String())
		}
		client := clientOfSale(t, tx, sold.ID)
		if client == nil || client.Name != sale.PartnerClientName(partner) {
			t.Errorf("sale client after the issuer change = %v, want the partner's", client)
		}
	})
}
