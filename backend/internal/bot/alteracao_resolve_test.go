package bot

import (
	"strings"
	"testing"

	"github.com/murandido/m-80-backend/internal/data"
	"gorm.io/gorm"
)

func settle(t *testing.T, tx *gorm.DB, lines ...string) ([]alteracaoLine, error) {
	t.Helper()

	msg, err := parseAlteracao(strings.Join(append([]string{"ALTERACAO - ABC123"}, lines...), "\n"))
	if err != nil {
		t.Fatalf("parseAlteracao returned %v", err)
	}
	return settleAlteracaoLines(tx, msg.Corrections)
}

// The three shapes the parser cannot separate, each settled by the table that
// knows the answer.
func TestSettleAlteracaoLines_ResolvesTheAmbiguousShapes(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		seedAirport(t, tx, "CNF", "Confins")
		seed(t, tx, &data.ObservationPreset{Code: "XYZ", Text: "Três letras, mas um código"})
		seed(t, tx, &data.ObservationPreset{Code: "ACORDO", Text: "Acordo comercial"})

		cases := []struct {
			line string
			want alteracaoLineKind
			why  string
		}{
			{"CNF", lineRoute, "a registered airport"},
			{"XYZ", lineObservation, "three letters, but a registered preset code"},
			{"ACORDO", lineObservation, "a registered preset code"},
			{"Bia", lineOwners, "neither an airport nor a preset, so an account alias"},
			{"Murilo", lineOwners, "no preset by that name, so an account alias"},
		}

		for _, tt := range cases {
			settled, err := settle(t, tx, tt.line)
			if err != nil {
				t.Fatalf("settling %q returned %v", tt.line, err)
			}
			if settled[0].Kind != tt.want {
				t.Errorf("%q settled as %v, want %v (%s)", tt.line, settled[0].Kind, tt.want, tt.why)
			}
		}
	})
}

// The collision only becomes visible after settling: the parser called "Bia" a
// route line, and only the airports table turns it into the second account line.
func TestSettleAlteracaoLines_RejectsADuplicateThatOnlySettlingReveals(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		_, err := settle(t, tx, "Bia", "Murilo - Wilmara")
		if err == nil {
			t.Fatal("two account lines were accepted")
		}
		if !strings.Contains(err.Error(), "duas linhas de conta") {
			t.Fatalf("error %q does not name the duplicated kind", err)
		}
	})
}

// Which extra is the surname and which the purchase number is not in the text -
// it is in the airline's flags, exactly as on an EMISSAO's line 2.
func TestResolveAlteracao_AssignsRouteExtrasByAirlineFlags(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{RequiresLastName: true, RequiresPurchaseNumber: true})
		issuance := data.Issuance{AirlineID: f.Airline.ID, LoyaltyProgramID: f.Program.ID}

		msg, err := parseAlteracao("ALTERACAO - ABC123\nCNF - SILVA - LA9574243YWPJ")
		if err != nil {
			t.Fatalf("parseAlteracao returned %v", err)
		}

		fields, err := resolveAlteracao(tx, msg, issuance)
		if err != nil {
			t.Fatalf("resolveAlteracao returned %v", err)
		}

		if !fields.HasRoute {
			t.Fatal("the route line was not recognised")
		}
		if fields.MainPassengerName == nil || *fields.MainPassengerName != "SILVA" {
			t.Fatalf("MainPassengerName = %v, want SILVA", fields.MainPassengerName)
		}
		if fields.LatamPurchaseNumber == nil || *fields.LatamPurchaseNumber != "LA9574243YWPJ" {
			t.Fatalf("LatamPurchaseNumber = %v, want LA9574243YWPJ", fields.LatamPurchaseNumber)
		}
	})
}

// A field the airline demands on an EMISSAO is not demanded again here: a
// correction names only what is changing, so leaving the surname out asks to keep
// it, not to erase it. The nil is what mergeCorrection reads as "untouched".
func TestResolveAlteracao_LeavesOutAnExtraTheMessageDidNotMention(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{Name: "LATAM", RequiresLastName: true})
		issuance := data.Issuance{AirlineID: f.Airline.ID, LoyaltyProgramID: f.Program.ID}

		msg, err := parseAlteracao("ALTERACAO - ABC123\nCNF")
		if err != nil {
			t.Fatalf("parseAlteracao returned %v", err)
		}

		fields, err := resolveAlteracao(tx, msg, issuance)
		if err != nil {
			t.Fatalf("resolveAlteracao refused a route line that only moves the origin: %v", err)
		}
		if !fields.HasRoute || fields.OriginIATACode == nil || *fields.OriginIATACode != "CNF" {
			t.Fatalf("OriginIATACode = %v, want CNF", fields.OriginIATACode)
		}
		if fields.MainPassengerName != nil {
			t.Fatalf("MainPassengerName = %q, want nil so the stored surname survives", *fields.MainPassengerName)
		}
	})
}

// A field the airline does not use is still an error: nobody typed it by accident
// twice, and silently dropping it would lose a correction someone meant.
func TestResolveAlteracao_RefusesARouteWithAFieldTooMany(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{Name: "GOL", RequiresLastName: true})
		issuance := data.Issuance{AirlineID: f.Airline.ID, LoyaltyProgramID: f.Program.ID}

		msg, err := parseAlteracao("ALTERACAO - ABC123\nCNF - SILVA - LA9574243YWPJ")
		if err != nil {
			t.Fatalf("parseAlteracao returned %v", err)
		}

		if _, err := resolveAlteracao(tx, msg, issuance); err == nil {
			t.Fatal("a route line with a field the airline does not use was accepted")
		} else if !strings.Contains(err.Error(), "não usa") {
			t.Fatalf("error %q does not say the field is unused", err)
		}
	})
}

// An unknown observation code is refused rather than stored raw: a mistyped code
// vanishing into the observation field is worse than a message sent again.
func TestResolveAlteracao_RefusesAnUnknownObservationCodeItWasToldAbout(t *testing.T) {
	runInTx(t, func(tx *gorm.DB) {
		f := seedEmissaoFixture(t, tx, data.Airline{})
		issuance := data.Issuance{AirlineID: f.Airline.ID, LoyaltyProgramID: f.Program.ID}
		seed(t, tx, &data.ObservationPreset{Code: "ACORDO" + f.Suffix, Text: "Acordo comercial"})

		// Settling sends an unknown bare word to the account line, so the only way
		// to reach the observation branch with a bad code is a line that settles
		// as an observation and then fails to resolve. resolveObservation is what
		// guards that, and it is shared with the EMISSAO - assert it directly.
		if _, err := resolveObservation(tx, "NAOEXISTE"); err == nil {
			t.Fatal("an unknown observation code was accepted")
		}

		msg, err := parseAlteracao("ALTERACAO - ABC123\nACORDO" + f.Suffix)
		if err != nil {
			t.Fatalf("parseAlteracao returned %v", err)
		}
		fields, err := resolveAlteracao(tx, msg, issuance)
		if err != nil {
			t.Fatalf("resolveAlteracao returned %v for a registered code", err)
		}
		if fields.Observation == nil || *fields.Observation != "Acordo comercial" {
			t.Fatalf("Observation = %v, want the preset text", fields.Observation)
		}
	})
}
