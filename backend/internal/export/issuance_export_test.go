package export

import (
	"bytes"
	"os"
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/data"
	"github.com/xuri/excelize/v2"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }

func TestAccountLabel(t *testing.T) {
	tests := []struct {
		name  string
		given data.Issuance
		want  string
	}{
		{
			name:  "alias wins when present",
			given: data.Issuance{AccountNickname: &data.AccountNickname{Nickname: "Conta VIP"}},
			want:  "Conta VIP",
		},
		{
			name:  "holder name when no alias",
			given: data.Issuance{Account: &data.Account{Holder: &data.Person{Name: "Ana Souza"}}},
			want:  "Ana Souza",
		},
		{
			name:  "login when no alias and no holder",
			given: data.Issuance{Account: &data.Account{Login: strptr("ana@example.com")}},
			want:  "ana@example.com",
		},
		{
			name:  "account id as last resort",
			given: data.Issuance{AccountID: "acc-123", Account: &data.Account{}},
			want:  "acc-123",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := accountLabel(tc.given); got != tc.want {
				t.Fatalf("accountLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCentsToReais(t *testing.T) {
	if got := centsToReais(2450); got != 24.5 {
		t.Fatalf("centsToReais(2450) = %v, want 24.5", got)
	}
}

// TestRenderIssuancesXLSX exercises the full render against the real MODELO template. The
// template is provisioned at runtime and gitignored, so the test skips when it is absent
// (e.g. in CI) rather than reaching out to the network.
func TestRenderIssuancesXLSX(t *testing.T) {
	template, err := os.ReadFile("../../" + issuanceTemplatePath)
	if os.IsNotExist(err) {
		t.Skipf("template not provisioned at %q; skipping end-to-end render", issuanceTemplatePath)
	}
	if err != nil {
		t.Fatalf("read template: %v", err)
	}

	issuances := []data.Issuance{
		{
			LoyaltyProgram:      &data.LoyaltyProgram{Name: "Smiles"},
			ProgramLocator:      "ABC123",
			MainPassengerName:   strptr("Ana Souza"),
			OriginIATACode:      strptr("GRU"),
			DestinationIATACode: strptr("SDU"),
			Points:              10000,
			ValueOfTheThousand:  2000,
			Taxes:               intptr(3550),
			AccountNickname:     &data.AccountNickname{Nickname: "Conta VIP"},
			Issuer:              &data.User{Name: "João Emissor"},
			Agency:              &data.Agency{Name: "Agência Central"},
			CreatedAt:           time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
			Flights: []data.IssuanceFlight{
				{Direction: "outbound", LegOrder: 1, DepartureAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)},
			},
		},
	}

	out, err := renderIssuancesXLSX(template, issuances, time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("renderIssuancesXLSX: %v", err)
	}

	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("reopen rendered file: %v", err)
	}
	defer func() { _ = f.Close() }()

	cells := map[string]string{
		"B3": "Smiles",
		"C3": "ABC123",
		"F3": "Ana Souza",
		"G3": "GRU",
		"H3": "SDU",
		"I3": "10.000", // 10000 points rendered as thousands (numFmt 0.000)
		"M3": "Conta VIP",
		"O3": "João Emissor",
		"P3": "Agência Central",
	}
	for cell, want := range cells {
		got, err := f.GetCellValue(issuanceExportSheet, cell)
		if err != nil {
			t.Fatalf("get %s: %v", cell, err)
		}
		if got != want {
			t.Errorf("cell %s = %q, want %q", cell, got, want)
		}
	}
}
