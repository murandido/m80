package bot

import (
	"strings"
	"testing"
)

func TestIsCancelamentoMessage(t *testing.T) {
	cases := map[string]bool{
		"CANCELAMENTO - IWBEQP": true,
		"cancelamento - iwbeqp": true,
		"  CANCELAMENTO - X":    true,
		"CANCELAMENTOS - X":     true, // prefix match is deliberately loose, same as EMISSAO/ALTERACAO
		"ALTERACAO - IWBEQP":    false,
		"oi pessoal, bom dia":   false,
	}
	for text, want := range cases {
		if got := isCancelamentoMessage(text); got != want {
			t.Errorf("isCancelamentoMessage(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestParseCancelamento_HeaderOnly(t *testing.T) {
	msg, err := parseCancelamento("CANCELAMENTO - IWBEQP")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ProgramLocator != "IWBEQP" {
		t.Fatalf("ProgramLocator = %q, want IWBEQP", msg.ProgramLocator)
	}
	if msg.HasRefundLine {
		t.Fatal("HasRefundLine = true, want false for a header-only message")
	}
}

func TestParseCancelamento_FullRefundOnly(t *testing.T) {
	msg, err := parseCancelamento("cancelamento - iwbeqp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.ProgramLocator != "IWBEQP" {
		t.Fatalf("ProgramLocator = %q, want IWBEQP (uppercased)", msg.ProgramLocator)
	}
}

func TestParseCancelamento_RefundLine(t *testing.T) {
	cases := []struct {
		name       string
		line2      string
		wantPoints int
		wantTaxes  *int
		wantFee    *int
	}{
		{name: "points only, zero", line2: "0k", wantPoints: 0},
		{name: "points and tax", line2: "0k + 50 tx", wantPoints: 0, wantTaxes: ptr(5000)},
		{name: "points and fee", line2: "30k + 50 multa", wantPoints: 30000, wantFee: ptr(5000)},
		{name: "tax before fee", line2: "10k + 20 tx + 30 multa", wantPoints: 10000, wantTaxes: ptr(2000), wantFee: ptr(3000)},
		{name: "fee before tax", line2: "10k + 30 multa + 20 tx", wantPoints: 10000, wantTaxes: ptr(2000), wantFee: ptr(3000)},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := parseCancelamento("CANCELAMENTO - IWBEQP\n" + tt.line2)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !msg.HasRefundLine {
				t.Fatal("HasRefundLine = false, want true")
			}
			if msg.RefundedPoints != tt.wantPoints {
				t.Errorf("RefundedPoints = %d, want %d", msg.RefundedPoints, tt.wantPoints)
			}
			if !intPtrEqual(msg.RefundedTaxes, tt.wantTaxes) {
				t.Errorf("RefundedTaxes = %v, want %v", deref(msg.RefundedTaxes), deref(tt.wantTaxes))
			}
			if !intPtrEqual(msg.CancellationFee, tt.wantFee) {
				t.Errorf("CancellationFee = %v, want %v", deref(msg.CancellationFee), deref(tt.wantFee))
			}
		})
	}
}

func TestParseCancelamento_Rejections(t *testing.T) {
	cases := map[string]string{
		"":                                "pelo menos do localizador",
		"CANCELAMENTO":                    "linha 1 precisa ser",
		"CANCELAMENTO - SMILES/IWBEQP":    "dois localizadores",
		"CANCELAMENTO - IWBEQP\nabc\ndef": "no máximo 2 linhas",
		"CANCELAMENTO - IWBEQP\ntx":       "número válido",
		"CANCELAMENTO - IWBEQP\n0k + 50":  `Não entendi "50"`,
		"CANCELAMENTO - IWBEQP\n0k + 50 tx + 60 tx": "duas vezes",
	}
	for text, mentions := range cases {
		t.Run(text, func(t *testing.T) {
			_, err := parseCancelamento(text)
			if err == nil {
				t.Fatalf("expected an error for %q", text)
			}
			if got := err.Error(); !strings.Contains(got, mentions) {
				t.Fatalf("error = %q, want it to mention %q", got, mentions)
			}
		})
	}
}

func ptr(v int) *int { return &v }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func intPtrEqual(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
