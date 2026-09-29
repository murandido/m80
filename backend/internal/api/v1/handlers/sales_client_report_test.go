package handlers

import (
	"testing"
	"time"

	"github.com/murandido/m-80-backend/internal/api/v1/schemas"
)

func TestBuildClientReport_GroupsByClientAndAddsUp(t *testing.T) {
	joao, maria := "c-joao", "c-maria"
	joaoName, mariaName := "JOÃO", "MARIA"
	day := time.Date(2026, 9, 2, 15, 0, 0, 0, time.UTC)
	cancelled := day

	sales := []schemas.SaleResponse{
		{ClientID: &maria, ClientName: &mariaName, ProgramLocator: "M1", IssuedAt: day, AmountCents: 10000, PaidCents: 10000, BalanceCents: 0},
		{ProgramLocator: "N1", IssuedAt: day, AmountCents: 5000, BalanceCents: 5000},
		{ClientID: &joao, ClientName: &joaoName, ProgramLocator: "J1", IssuedAt: day, AmountCents: 10000, PaidCents: 4000, BalanceCents: 6000},
		// Cancelled after being paid: 2000 has to go back, and its value is no debt.
		{ClientID: &joao, ClientName: &joaoName, ProgramLocator: "J2", IssuedAt: day, AmountCents: 7000, PaidCents: 2000, BalanceCents: -2000, CancelledAt: &cancelled},
	}

	model := buildClientReport(sales, map[string]int{joao: 1500}, "Agência", "setembro", day)

	if len(model.Sections) != 3 {
		t.Fatalf("sections: got %d, want 3", len(model.Sections))
	}
	names := []string{model.Sections[0].ClientName, model.Sections[1].ClientName, model.Sections[2].ClientName}
	if names[0] != "JOÃO" || names[1] != "MARIA" || names[2] != noClientSectionName {
		t.Errorf("order: got %v, want clients by name and the unlinked last", names)
	}

	j := model.Sections[0]
	if j.TotalAmount != "R$ 100,00" || j.TotalPaid != "R$ 60,00" || j.TotalBalance != "R$ 40,00" || j.Settled {
		t.Errorf("joão totals: got amount %s paid %s balance %s settled %v", j.TotalAmount, j.TotalPaid, j.TotalBalance, j.Settled)
	}
	if j.Credit != "R$ 15,00" {
		t.Errorf("joão credit: got %q", j.Credit)
	}
	if j.Lines[0].StatusKind != "partial" || j.Lines[1].StatusKind != "refund" || !j.Lines[1].Cancelled {
		t.Errorf("joão statuses: got %+v", j.Lines)
	}

	if m := model.Sections[1]; !m.Settled || m.Credit != "" || m.Lines[0].Status != "Pago" {
		t.Errorf("maria: got %+v", m)
	}

	if got := clientReportFilename(model, day); got != "controle-pagamentos-2026-09-02.pdf" {
		t.Errorf("several clients filename: got %q", got)
	}
	single := buildClientReport(sales[:1], nil, "", "", day)
	if got := clientReportFilename(single, day); got != "controle-maria-2026-09-02.pdf" {
		t.Errorf("one client filename: got %q", got)
	}
	accented := buildClientReport(sales[2:3], nil, "", "", day)
	if got := clientReportFilename(accented, day); got != "controle-joao-2026-09-02.pdf" {
		t.Errorf("accented client filename: got %q", got)
	}
}
