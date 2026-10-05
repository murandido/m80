package clientreport

import (
	"strings"
	"testing"
)

func TestRender_WritesTheFiguresItWasGiven(t *testing.T) {
	out, err := Render(ViewModel{
		AgencyName:  "Agência",
		PeriodLabel: "01/09/2026 a 30/09/2026",
		GeneratedAt: "29/09/2026 10:00",
		Sections: []Section{{
			ClientName: "JOÃO SILVA",
			Lines: []Line{
				{Date: "02/09/2026", Locator: "ABC123", Passenger: "JOAO", Amount: "R$ 100,00", Paid: "R$ 40,00", Balance: "R$ 60,00", Status: "Parcial", StatusKind: "partial"},
				{Date: "03/09/2026", Locator: "XYZ789", Amount: "R$ 50,00", Paid: "R$ 0,00", Balance: "R$ 0,00", Status: "Cancelada", StatusKind: "cancelled", Cancelled: true},
			},
			TotalAmount:  "R$ 100,00",
			TotalPaid:    "R$ 40,00",
			TotalBalance: "R$ 60,00",
			Credit:       "R$ 5,00",
		}},
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	for _, want := range []string{"JOÃO SILVA", "ABC123", "tag-partial", "line cancelled", "Total em aberto", "R$ 60,00", "Crédito disponível", "R$ 5,00", "01/09/2026 a 30/09/2026"} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered document is missing %q", want)
		}
	}
}

func TestRender_SettledSectionSaysSo(t *testing.T) {
	out, err := Render(ViewModel{Sections: []Section{{ClientName: "MARIA", TotalBalance: "R$ 0,00", Settled: true}}})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "Nada em aberto") || strings.Contains(out, "Crédito disponível") {
		t.Errorf("settled section rendered wrong:\n%s", out)
	}
	if !strings.Contains(out, "Nenhuma emissão no período.") {
		t.Error("an empty section should say it has no issuances")
	}
}
