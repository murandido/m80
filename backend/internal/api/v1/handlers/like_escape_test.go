package handlers

import (
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func TestLikePattern_EscapesWildcards(t *testing.T) {
	cases := map[string]string{
		"joao":    `%joao%`,
		"50%":     `%50\%%`,
		"a_b":     `%a\_b%`,
		`c:\temp`: `%c:\\temp%`,
		`\%_`:     `%\\\%\_%`,
	}
	for term, want := range cases {
		if got := likePattern(term); got != want {
			t.Errorf("likePattern(%q) = %q, want %q", term, got, want)
		}
	}
}

// Search terms used to reach LIKE as pattern syntax: "_" matched any character, so an
// operator searching a login with an underscore got every account back, and "50%"
// behaved like "50". Each term must match only the login that literally contains it.
func TestGetAccounts_SearchTreatsWildcardsLiterally(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		airline := data.Airline{Name: "CIA " + uuid.New().String()[:8], Code: uuid.New().String()[:6]}
		create(t, tx, &airline)
		program := data.LoyaltyProgram{
			Name:             "Programa " + uuid.New().String()[:8],
			Code:             uuid.New().String()[:6],
			DefaultAirlineID: airline.ID,
		}
		create(t, tx, &program)

		suffix := uuid.New().String()[:8]
		logins := []string{
			"a_b-" + suffix,
			"axb-" + suffix,
			"cupom50%-" + suffix,
			"cupom500-" + suffix,
			`c:\dir-` + suffix,
		}
		accountByLogin := map[string]string{}
		for _, login := range logins {
			value := login
			account := data.Account{HolderID: testHolder(t, tx), LoyaltyProgramID: program.ID, Login: &value}
			create(t, tx, &account)
			accountByLogin[login] = account.ID
		}

		for term, wantLogin := range map[string]string{
			"a_b":  "a_b-" + suffix,
			"_":    "a_b-" + suffix,
			"50%":  "cupom50%-" + suffix,
			"%":    "cupom50%-" + suffix,
			`c:\d`: `c:\dir-` + suffix,
		} {
			found := callGetAccounts(t, tx, "loyalty_program_id="+program.ID+"&q="+url.QueryEscape(term))
			if len(found) != 1 || found[0].ID != accountByLogin[wantLogin] {
				t.Errorf("searching %q returned %d accounts, want only %q", term, len(found), wantLogin)
			}
		}
	})
}
