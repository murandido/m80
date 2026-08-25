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
			// The account is exempt (Azul Diamante Unique), so the fee it did not pay
			// belongs in the TAXA RESGATE column the render adds to the template.
			RedemptionFeeSavedCents: intptr(4200),
			AccountNickname:         &data.AccountNickname{Nickname: "Conta VIP"},
			Issuer:                  &data.User{Name: "João Emissor"},
			Agency:                  &data.Agency{Name: "Agência Central"},
			CreatedAt:               time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC),
			Flights: []data.IssuanceFlight{
				{Direction: "outbound", LegOrder: 1, DepartureAt: time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)},
			},
		},
	}

	out, err := renderIssuancesXLSX(
		template,
		issuances,
		nil,
		nil,
		time.Date(2026, 7, 22, 15, 0, 0, 0, time.UTC),
	)
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
		"Q2": "TAXA RESGATE",
		// The currency format is borrowed from the TAXAS column; excelize renders it with
		// the locale-independent decimal point.
		"Q3": "R$ 42.00",
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

func TestSignedIssuanceAmounts(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 8, 10+d, 12, 0, 0, 0, time.UTC) }
	from := time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 13, 3, 0, 0, 0, time.UTC)
	cancelled := func(i data.Issuance, at time.Time) data.Issuance {
		i.Active = false
		i.CancelledAt = &at
		return i
	}

	base := data.Issuance{Points: 10000, Taxes: intptr(3000)}

	tests := []struct {
		name       string
		given      data.Issuance
		wantPoints int
		wantTaxes  *int
	}{
		{
			name:       "issued inside the period counts positive",
			given:      func() data.Issuance { i := base; i.CreatedAt = day(2); return i }(),
			wantPoints: 10000,
			wantTaxes:  intptr(3000),
		},
		{
			name: "issued and cancelled in the period nets out",
			given: func() data.Issuance {
				i := base
				i.CreatedAt = day(2)
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, day(2))
			}(),
			wantPoints: 0,
			wantTaxes:  intptr(0),
		},
		{
			name: "cancelled in the period after being issued before it is only the refund",
			given: func() data.Issuance {
				i := base
				i.CreatedAt = day(0)
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, day(2))
			}(),
			wantPoints: -10000,
			wantTaxes:  intptr(-3000),
		},
		{
			name: "cancellation-only row carries the refund and no booking",
			given: func() data.Issuance {
				i := base
				i.CreatedAt = day(2)
				i.CancellationOnly = true
				i.RefundedPoints = intptr(10000)
				i.RefundedTaxes = intptr(3000)
				return cancelled(i, day(2))
			}(),
			wantPoints: -10000,
			wantTaxes:  intptr(-3000),
		},
		{
			name: "cancellation with no refund keeps the miles spent",
			given: func() data.Issuance {
				i := base
				i.CreatedAt = day(2)
				return cancelled(i, day(2))
			}(),
			wantPoints: 10000,
			wantTaxes:  intptr(3000),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			points, taxes := signedIssuanceAmounts(tc.given, &from, &to)
			if points != tc.wantPoints {
				t.Errorf("points = %d, want %d", points, tc.wantPoints)
			}
			switch {
			case taxes == nil && tc.wantTaxes != nil:
				t.Errorf("taxes = nil, want %d", *tc.wantTaxes)
			case taxes != nil && tc.wantTaxes == nil:
				t.Errorf("taxes = %d, want nil", *taxes)
			case taxes != nil && *taxes != *tc.wantTaxes:
				t.Errorf("taxes = %d, want %d", *taxes, *tc.wantTaxes)
			}
		})
	}
}

// An export with no date filter has to keep netting a cancelled booking out, or the "all
// dates" sheet would count miles that came back.
func TestSignedIssuanceAmountsUnboundedPeriod(t *testing.T) {
	at := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	issuance := data.Issuance{
		Points:         10000,
		CreatedAt:      at,
		CancelledAt:    &at,
		RefundedPoints: intptr(4000),
	}

	points, taxes := signedIssuanceAmounts(issuance, nil, nil)
	if points != 6000 {
		t.Errorf("points = %d, want 6000", points)
	}
	if taxes != nil {
		t.Errorf("taxes = %d, want nil", *taxes)
	}
}

func TestSignedRedemptionFee(t *testing.T) {
	from := time.Date(2026, 8, 12, 3, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 13, 3, 0, 0, 0, time.UTC)
	inside := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	before := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		given data.Issuance
		want  *int
	}{
		{
			name:  "ordinary account saves nothing",
			given: data.Issuance{CreatedAt: inside},
			want:  nil,
		},
		{
			name: "exempt account issued in the period",
			given: data.Issuance{
				CreatedAt:               inside,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(4200),
		},
		{
			name: "cancelled in the period gives the saving back",
			given: data.Issuance{
				CreatedAt:               before,
				CancelledAt:             &inside,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(-4200),
		},
		{
			name: "issued and cancelled in the period nets out",
			given: data.Issuance{
				CreatedAt:               inside,
				CancelledAt:             &inside,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: intptr(0),
		},
		{
			name: "outside the period on both sides is blank",
			given: data.Issuance{
				CreatedAt:               before,
				RedemptionFeeSavedCents: intptr(4200),
			},
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := signedRedemptionFee(tc.given, &from, &to)
			switch {
			case got == nil && tc.want != nil:
				t.Fatalf("fee = nil, want %d", *tc.want)
			case got != nil && tc.want == nil:
				t.Fatalf("fee = %d, want nil", *got)
			case got != nil && *got != *tc.want:
				t.Fatalf("fee = %d, want %d", *got, *tc.want)
			}
		})
	}
}
