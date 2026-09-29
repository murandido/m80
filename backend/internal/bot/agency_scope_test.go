package bot

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/access"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func seedNicknamedAgency(t *testing.T, tx *gorm.DB, nickname string) data.Agency {
	t.Helper()
	agency := data.Agency{Name: nickname + " " + uuid.New().String()[:8]}
	seed(t, tx, &agency)
	seed(t, tx, &data.AgencyNickname{AgencyID: agency.ID, Nickname: nickname})
	return agency
}

// The bot acts for a panel user, so an EMISSAO naming an agency outside their grants has to
// be refused - otherwise WhatsApp is the way around every grant the panel enforces.
func TestResolveEmissao_AgencyStaysInsideTheSendersScope(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})
		suffix := strings.ToLower(f.Suffix)
		mine := seedNicknamedAgency(t, tx, "mine"+suffix)
		seedNicknamedAgency(t, tx, "theirs"+suffix)
		scope := access.AgencyScope{IDs: []string{mine.ID}}

		msg := f.message("Heraldo")
		msg.AgencyNickname = "MINE" + f.Suffix
		input, err := resolveEmissao(tx, msg, scope)
		if err != nil || input.AgencyID == nil || *input.AgencyID != mine.ID {
			t.Fatalf("own agency: want %s, got %v (err %v)", mine.ID, input.AgencyID, err)
		}

		msg = f.message("Heraldo")
		msg.AgencyNickname = "theirs" + suffix
		if _, err := resolveEmissao(tx, msg, scope); err == nil || !strings.Contains(err.Error(), "não tem acesso") {
			t.Errorf("foreign agency: want a refusal, got %v", err)
		}

		before := countRows(t, tx, "agencies", "1 = ?", 1)
		msg = f.message("Heraldo")
		msg.AgencyNickname = "brandnew" + suffix
		if _, err := resolveEmissao(tx, msg, scope); err == nil {
			t.Error("unknown agency: a restricted sender must not conjure one")
		}
		if after := countRows(t, tx, "agencies", "1 = ?", 1); after != before {
			t.Errorf("unknown agency: %d agencies were created", after-before)
		}

		// A dev still conjures it, as before.
		if _, err := resolveEmissao(tx, msg, access.FullScope()); err != nil {
			t.Errorf("dev with an unknown agency: %v", err)
		}
	})
}

func TestDefaultAgencyFor(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		mine := seedNicknamedAgency(t, tx, "only"+strings.ToLower(uuid.New().String()[:6]))

		got, err := defaultAgencyFor(tx, access.AgencyScope{IDs: []string{mine.ID}})
		if err != nil || got == nil || *got != mine.ID {
			t.Errorf("restricted: want the sender's own agency %s, got %v (err %v)", mine.ID, got, err)
		}
		if _, err := defaultAgencyFor(tx, access.AgencyScope{}); err == nil || !strings.Contains(err.Error(), "nenhuma agência") {
			t.Errorf("no grants: want a refusal, got %v", err)
		}
	})
}
