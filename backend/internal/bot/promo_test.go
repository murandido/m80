package bot

import "testing"

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
		{"promo command", "!promo https://www.voeazul.com.br/br/pt/ofertas/inter", kindPromo},
		{"promo command with no link", "!promo", kindPromo},
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

// The three registration points a new command needs. Missing the intake one means the
// message is never admitted; missing the route means its reaction hangs forever.
func TestPromoCommandIsAdmittedByIntake(t *testing.T) {
	if !isKnownCommand("!promo https://exemplo.com/oferta") {
		t.Fatal("!promo must be admitted by intake")
	}
	if commandName("!PROMO https://exemplo.com") != cmdNamePromo {
		t.Fatal("command name must be matched case-insensitively")
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
