package bot

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	waLog "go.mau.fi/whatsmeow/util/log"
	"gorm.io/gorm"
)

// isPromoLinkMessage is the gate between "somebody pasted a promotion" and
// "somebody mentioned one". Getting it wrong in the permissive direction spends real
// enrolments on chatter, so the strictness below is the feature.
func TestIsPromoLinkMessageOnlyAcceptsABareLink(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{"bare link", "https://www.voeazul.com.br/br/pt/ofertas/inter", true},
		{"bare link with tracking", "https://www.voeazul.com.br/br/pt/ofertas/inter?utm_source=wa", true},
		{"link with surrounding words", "olha essa https://www.voeazul.com.br/br/pt/ofertas/inter", false},
		{"link with a trailing comment", "https://www.voeazul.com.br/br/pt/ofertas/inter vale a pena?", false},
		{"bare domain", "voeazul.com.br", false},
		{"conversation", "alguém viu a promoção da azul?", false},
		{"empty", "", false},
		{"whitespace around a link is fine", "  https://www.voeazul.com.br/ofertas  ", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isPromoLinkMessage(tc.text); got != tc.want {
				t.Fatalf("isPromoLinkMessage(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// An EMISSAO that happens to carry a link must stay an EMISSAO. classifyText checks
// the promotion case last precisely so a link can never take a message over.
func TestClassifyTextKeepsPromoLast(t *testing.T) {
	tests := []struct {
		name string
		text string
		want triggerKind
	}{
		{"bare link", "https://www.voeazul.com.br/br/pt/ofertas/inter", kindPromo},
		// The command that used to wrap the link is gone; typing it now is just an
		// unknown command, which the bot ignores without a reaction.
		{"the retired promo command", "!promo https://www.voeazul.com.br/br/pt/ofertas/inter", kindNone},
		{"other command", "!ping", kindCommand},
		{"unknown command", "!seila", kindNone},
		{"conversation", "bom dia", kindNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyText(tc.text); got != tc.want {
				t.Fatalf("classifyText(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// The clock means "waiting its turn" and the hourglass means "working". An enrolment
// never waits - it skips the chat queue - so intake must not put a clock on it, and
// holding the queue for the two minutes a ten-CPF run takes would park every EMISSAO
// typed meanwhile behind it.
func TestPromoJobSkipsTheChatQueue(t *testing.T) {
	j := &job{kind: kindPromo}
	if !j.skipsQueue() {
		t.Fatal("a promotion enrolment must not hold the chat queue")
	}
}

// Intake must not admit the retired command: admitting it would acknowledge with a ✅
// and then leave the reaction hanging, since nothing routes it any more.
func TestRetiredPromoCommandIsNotAdmitted(t *testing.T) {
	if isKnownCommand("!promo https://exemplo.com/oferta") {
		t.Fatal("!promo was retired and must not be admitted by intake")
	}
}

func TestPromoStatusLabelsAreReadableInPortuguese(t *testing.T) {
	// The group reads these, so they are checked rather than assumed: a raw "failure"
	// in a WhatsApp message is the kind of thing nobody notices until an operator asks
	// what it means.
	for _, status := range []string{"success", "failure", "unknown", "refused", "error"} {
		if label := promoStatusLabel(status); label == "" || label == status {
			t.Errorf("promoStatusLabel(%q) = %q, want a Portuguese label", status, label)
		}
	}
}

// The domain gate is what makes a bare link safe as the only trigger. It runs before
// any reaction, so a link for a site we do not drive has to come back false rather
// than reach the handler and be refused out loud - the group shares promotions all the
// time, and the bot answering every one of them is noise.
func TestPromoRecipeExistsOnlyForCoveredDomains(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		suffix := strings.ToUpper(uuid.New().String()[:6])

		airline := data.Airline{Name: "Test Air", Code: "A" + suffix}
		seed(t, tx, &airline)
		program := data.LoyaltyProgram{Name: "Test Program", Code: "P" + suffix, DefaultAirlineID: airline.ID}
		seed(t, tx, &program)
		recipe := data.PromoRecipe{
			Domain:           "voeazul.com.br",
			Name:             "Azul " + suffix,
			LoyaltyProgramID: program.ID,
			Active:           true,
		}
		seed(t, tx, &recipe)

		retired := data.PromoRecipe{
			Domain:           "smiles.com.br",
			Name:             "Smiles " + suffix,
			LoyaltyProgramID: program.ID,
			Active:           false,
		}
		seed(t, tx, &retired)

		bot := &Bot{DB: tx, Logger: waLog.Noop}

		tests := []struct {
			name string
			link string
			want bool
		}{
			{"the recipe's own domain", "https://voeazul.com.br/br/pt/ofertas/c6", true},
			{"a subdomain of it", "https://www.voeazul.com.br/br/pt/ofertas/c6", true},
			{"a domain no recipe covers", "https://www.latampass.latam.com/pt_br/promo", false},
			{"a domain that merely ends in the same words", "https://naovoeazul.com.br/ofertas", false},
			{"a domain whose only recipe is retired", "https://www.smiles.com.br/portal/campanhas/x", false},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := bot.promoRecipeExistsFor(context.Background(), tc.link); got != tc.want {
					t.Fatalf("promoRecipeExistsFor(%q) = %v, want %v", tc.link, got, tc.want)
				}
			})
		}
	})
}
