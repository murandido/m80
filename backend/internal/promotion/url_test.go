package promotion

import "testing"

// The whole point of the canonical key: a campaign shared three different ways is one
// campaign. Without this the enrolment history never matches and everybody is enrolled
// again on the second paste.
func TestCanonicalURLKeyIgnoresHowTheLinkWasShared(t *testing.T) {
	same := []string{
		"https://www.voeazul.com.br/br/pt/ofertas/inter",
		"http://voeazul.com.br/br/pt/ofertas/inter",
		"https://VoeAzul.com.br/br/pt/ofertas/inter/",
		"https://www.voeazul.com.br/br/pt/ofertas/inter?utm_source=whatsapp&utm_campaign=ago26",
		"www.voeazul.com.br/br/pt/ofertas/inter#regulamento",
		"voeazul.com.br/br/pt/ofertas/inter",
	}
	want := "voeazul.com.br/br/pt/ofertas/inter"
	for _, raw := range same {
		got, err := CanonicalURLKey(raw)
		if err != nil {
			t.Fatalf("CanonicalURLKey(%q): %v", raw, err)
		}
		if got != want {
			t.Errorf("CanonicalURLKey(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestCanonicalURLKeySeparatesDifferentCampaigns(t *testing.T) {
	first, err := CanonicalURLKey("https://www.voeazul.com.br/br/pt/ofertas/inter")
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalURLKey("https://www.voeazul.com.br/br/pt/ofertas/livelo")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("two campaigns collapsed into one key: %q", first)
	}
}

func TestCanonicalURLKeyRejectsWhatIsNotALink(t *testing.T) {
	for _, raw := range []string{"", "   ", "bom dia"} {
		if got, err := CanonicalURLKey(raw); err == nil {
			t.Errorf("CanonicalURLKey(%q) = %q, want an error", raw, got)
		}
	}
}

func TestDomainOfDropsWWW(t *testing.T) {
	tests := map[string]string{
		"https://www.smiles.com.br/portal/campanhas/x": "smiles.com.br",
		"https://promo.voeazul.com.br/oferta":          "promo.voeazul.com.br",
		"VOEAZUL.COM.BR/br":                            "voeazul.com.br",
	}
	for raw, want := range tests {
		got, err := DomainOf(raw)
		if err != nil {
			t.Fatalf("DomainOf(%q): %v", raw, err)
		}
		if got != want {
			t.Errorf("DomainOf(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestFindLinkPicksTheLinkOutOfAMessage(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		want  string
		found bool
	}{
		{
			"link alone",
			"https://www.voeazul.com.br/br/pt/ofertas/inter",
			"https://www.voeazul.com.br/br/pt/ofertas/inter",
			true,
		},
		{
			"link in a sentence, with trailing punctuation",
			"pessoal, saiu essa: https://www.voeazul.com.br/br/pt/ofertas/inter.",
			"https://www.voeazul.com.br/br/pt/ofertas/inter",
			true,
		},
		{
			// A bare domain in conversation is somebody talking about the site, not
			// asking for an enrolment run. Requiring a scheme is what keeps chatter
			// from spending real enrolments.
			"bare domain is not a link",
			"o site da voeazul.com.br tá fora do ar",
			"",
			false,
		},
		{"no link", "bom dia", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, found := FindLink(tc.text)
			if found != tc.found || got != tc.want {
				t.Fatalf("FindLink(%q) = (%q, %v), want (%q, %v)", tc.text, got, found, tc.want, tc.found)
			}
		})
	}
}
