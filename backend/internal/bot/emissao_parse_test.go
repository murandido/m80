package bot

import (
	"reflect"
	"strings"
	"testing"
)

// The three messages below are real ones, copied out of the group the bot will
// listen to. They are the reason the format is what it is.
const (
	smilesMessage = "EMISSAO - SMILES - IWBEQP\nCNF - MACHADO\n31k * 16,5 + 189,22 tx = 700,72\nHeraldo"
	latamMessage  = "EMISSAO - LATAM - CXHMKF\nGIG - FELIX - LA9574243YWPJ\n89,912k * 30 + 340,36 tx = 3037,72\nPedro"
	azulMessage   = "EMISSAO - AZUL - ZMH7MZ\nBEL\n360k * 15 + 373,05 tx = 5773,05\nIsaias"
)

func TestParseEmissao_RealMessages(t *testing.T) {
	tests := []struct {
		name string
		text string
		want emissaoMessage
	}{
		{
			name: "smiles",
			text: smilesMessage,
			want: emissaoMessage{
				ProgramCode:        "SMILES",
				ProgramLocator:     "IWBEQP",
				OriginIATACode:     "CNF",
				Line2Extras:        []string{"MACHADO"},
				Points:             31000,
				ValueOfTheThousand: 1650,
				Taxes:              18922,
				Total:              70072,
				AccountNickname:    "Heraldo",
			},
		},
		{
			name: "latam with purchase number",
			text: latamMessage,
			want: emissaoMessage{
				ProgramCode:        "LATAM",
				ProgramLocator:     "CXHMKF",
				OriginIATACode:     "GIG",
				Line2Extras:        []string{"FELIX", "LA9574243YWPJ"},
				Points:             89912,
				ValueOfTheThousand: 3000,
				Taxes:              34036,
				Total:              303772,
				AccountNickname:    "Pedro",
			},
		},
		{
			name: "azul with origin only",
			text: azulMessage,
			want: emissaoMessage{
				ProgramCode:        "AZUL",
				ProgramLocator:     "ZMH7MZ",
				OriginIATACode:     "BEL",
				Points:             360000,
				ValueOfTheThousand: 1500,
				Taxes:              37305,
				Total:              577305,
				AccountNickname:    "Isaias",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseEmissao(tt.text)
			if err != nil {
				t.Fatalf("parseEmissao returned %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Fatalf("parsed\n  %+v\nwant\n  %+v", *got, tt.want)
			}
		})
	}
}

func TestParseEmissao_OptionalFields(t *testing.T) {
	text := "EMISSAO - SMILES/AA - IWBEQP/AA987654\nCNF - MACHADO\n31k * 16,5 + 189,22 tx = 700,72\nHeraldo - Fulano (AgenciaX)\nACORDO"

	got, err := parseEmissao(text)
	if err != nil {
		t.Fatalf("parseEmissao returned %v", err)
	}

	if got.AirlineCode != "AA" {
		t.Fatalf("AirlineCode = %q, want AA", got.AirlineCode)
	}
	if got.AirlineLocator != "AA987654" {
		t.Fatalf("AirlineLocator = %q, want AA987654", got.AirlineLocator)
	}
	if got.AdminNickname != "Fulano" {
		t.Fatalf("AdminNickname = %q, want Fulano", got.AdminNickname)
	}
	if got.AgencyNickname != "AgenciaX" {
		t.Fatalf("AgencyNickname = %q, want AgenciaX", got.AgencyNickname)
	}
	if got.ObservationCode != "ACORDO" {
		t.Fatalf("ObservationCode = %q, want ACORDO", got.ObservationCode)
	}
	if got.AccountNickname != "Heraldo" {
		t.Fatalf("AccountNickname = %q, want Heraldo", got.AccountNickname)
	}
}

// Comma and period are typed interchangeably in the group, so the same issuance
// written either way has to land on the very same numbers.
func TestParseEmissao_CommaAndPeriodAreEquivalent(t *testing.T) {
	tests := []struct {
		name         string
		withComma    string
		withPeriod   string
		wantPoints   int
		wantThousand int
		wantTaxes    int
	}{
		{
			name:         "cents",
			withComma:    "31k * 16,5 + 189,22 tx = 700,72",
			withPeriod:   "31k * 16.5 + 189.22 tx = 700.72",
			wantPoints:   31000,
			wantThousand: 1650,
			wantTaxes:    18922,
		},
		{
			name:         "fractional k",
			withComma:    "89,912k * 30 + 340,36 tx = 3037,72",
			withPeriod:   "89.912k * 30 + 340.36 tx = 3037.72",
			wantPoints:   89912,
			wantThousand: 3000,
			wantTaxes:    34036,
		},
		{
			name:         "single decimal k",
			withComma:    "90,8k * 10 + 0,04 tx = 908,04",
			withPeriod:   "90.8k * 10 + 0.04 tx = 908.04",
			wantPoints:   90800,
			wantThousand: 1000,
			wantTaxes:    4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comma := parseAmountsLine(t, tt.withComma)
			period := parseAmountsLine(t, tt.withPeriod)

			if !reflect.DeepEqual(comma, period) {
				t.Fatalf("comma parsed as %+v but period parsed as %+v", comma, period)
			}
			if comma.Points != tt.wantPoints {
				t.Fatalf("Points = %d, want %d", comma.Points, tt.wantPoints)
			}
			if comma.ValueOfTheThousand != tt.wantThousand {
				t.Fatalf("ValueOfTheThousand = %d, want %d", comma.ValueOfTheThousand, tt.wantThousand)
			}
			if comma.Taxes != tt.wantTaxes {
				t.Fatalf("Taxes = %d, want %d", comma.Taxes, tt.wantTaxes)
			}
		})
	}
}

func TestParseEmissao_Rejects(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		wantMessage string
	}{
		{
			name:        "wrong checksum",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 16,5 + 189,22 tx = 999,99\nHeraldo",
			wantMessage: "não fecham",
		},
		{
			// A separator typed one place off is the case the checksum exists
			// for: 1,65 parses perfectly well on its own, and only disagreeing
			// with the total gives it away.
			name:        "price off by a decimal place",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 1,65 + 189,22 tx = 700,72\nHeraldo",
			wantMessage: "não fecham",
		},
		{
			// The other half of the defence: a thousands group written where a
			// decimal belongs carries more precision than cents can hold, so it
			// is refused before the checksum is even reached.
			name:        "thousands separator on a price",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 1.500 + 0 tx = 46500,00\nHeraldo",
			wantMessage: "O valor do milheiro",
		},
		{
			name:        "too few lines",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 16,5 + 189,22 tx = 700,72",
			wantMessage: "pelo menos 4 linhas",
		},
		{
			name:        "malformed line 1",
			text:        "EMISSAO - SMILES\nCNF\n31k * 16,5 + 189,22 tx = 700,72\nHeraldo",
			wantMessage: "A linha 1 precisa ser",
		},
		{
			name:        "line 3 has no total",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 16,5 + 189,22 tx\nHeraldo",
			wantMessage: "A linha 3 precisa ser",
		},
		{
			name:        "line 3 has no multiplication",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k + 189,22 tx = 700,72\nHeraldo",
			wantMessage: "A linha 3 precisa ser",
		},
		{
			name:        "origin is not an airport code",
			text:        "EMISSAO - SMILES - IWBEQP\nCONFINS\n31k * 16,5 + 189,22 tx = 700,72\nHeraldo",
			wantMessage: "não é um código de aeroporto",
		},
		{
			name:        "fractional points",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n89,9125k * 30 + 340,36 tx = 3037,72\nHeraldo",
			wantMessage: "A quantidade de pontos",
		},
		{
			name:        "three decimals on the price",
			text:        "EMISSAO - SMILES - IWBEQP\nCNF\n31k * 16,555 + 189,22 tx = 700,72\nHeraldo",
			wantMessage: "O valor do milheiro",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseEmissao(tt.text)
			if err == nil {
				t.Fatal("expected parseEmissao to fail")
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Fatalf("err = %q, want it to mention %q", err, tt.wantMessage)
			}
		})
	}
}

func TestIsEmissaoMessage(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{smilesMessage, true},
		{"EMISSÃO - SMILES - IWBEQP\nCNF", true},
		{"emissao - smiles - iwbeqp\ncnf", true},
		{"  EMISSAO - SMILES - IWBEQP", true},
		{"bom dia pessoal", false},
		{"!ping", false},
		{"", false},
		{"vou fazer a EMISSAO agora", false},
	}

	for _, tt := range tests {
		if got := isEmissaoMessage(tt.text); got != tt.want {
			t.Fatalf("isEmissaoMessage(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

// parseAmountsLine parses a full message built around the given line 3, so the
// separator subtests exercise the real entry point rather than an internal.
func parseAmountsLine(t *testing.T, amounts string) emissaoMessage {
	t.Helper()

	msg, err := parseEmissao("EMISSAO - SMILES - IWBEQP\nCNF\n" + amounts + "\nHeraldo")
	if err != nil {
		t.Fatalf("parseEmissao(%q) returned %v", amounts, err)
	}
	return *msg
}
